# VBD Attachment Reconciliation

The driver detects volumes whose virtual disk was unplugged from the node VM in Xen
Orchestra, and plugs them back in automatically. This page explains how that works,
what it can and cannot repair, and how to troubleshoot it.

## Background

A volume is a Xen Orchestra **VDI** (virtual disk). To attach it to a node, the driver
creates a **VBD** (virtual block device) that links the VDI to the node VM, then
**plugs** it in. Xen Orchestra shows a plugged VBD as `attached`, XAPI as
`currently_attached`. The disk then appears on the node as `/dev/xvdX`.

In Kubernetes, each attachment is a `VolumeAttachment` object. When it says
`status.attached: true`, kubelet formats and mounts the device (`NodeStageVolume`).

## The problem

A VBD can be unplugged while Kubernetes still considers the volume attached, for example
after an action in Xen Orchestra or a failed hotplug. The pod then stays in
`ContainerCreating`, and its events repeat:

```
MountVolume.MountDevice failed for volume "pvc-…" : rpc error: code = Internal
desc = device xvdb does not exist on this host despite SR being connected
```

The node plugin has no Xen Orchestra credentials, so it cannot plug the VBD back in
itself.

## How the driver repairs it

The controller supports `ListVolumes` with the `LIST_VOLUMES_PUBLISHED_NODES`
capability. This enables the **VolumeAttachment reconciler** of the
[external-attacher](https://github.com/kubernetes-csi/external-attacher) sidecar that
runs in the controller pod:

1. Every `--reconcile-sync` interval (default `1m`), the attacher calls `ListVolumes`.
   For each volume, the driver reports the node VMs where its VBD is actually plugged.
2. For each `VolumeAttachment` with `attached: true`, the attacher checks that its node
   is in that list. If not, it logs `VolumeAttachment attached status and actual state do
   not match` and processes the attachment again. The `VolumeAttachment` stays
   `attached: true` meanwhile.
3. The attacher calls `ControllerPublishVolume` again. If the VBD still exists but is
   unplugged, the driver plugs it back in. If the VBD was removed, the driver creates a
   new one. The new device name is stored in the `VolumeAttachment`.
4. kubelet's next retry finds the device and mounts it. The pod starts.

Nothing needs to be configured on the nodes. A repair usually completes within one
reconcile interval: in one test, the VBD was plugged back 6 s after it was unplugged,
and the pod was running 18 s after.

## What `ListVolumes` reports

- **Volumes**: every VDI with a `k8s:volumeId:<id>` tag and, when `--cluster-tag` is set,
  that cluster tag too. The volume ID is the value of the `k8s:volumeId` tag. Statically
  provisioned volumes get both tags the first time they are attached (see
  [VDI Lookup and Identification](vdi-lookup-and-identification.md)).
- **Published nodes**: the UUIDs of the VMs (the CSI node IDs) where a VBD of the VDI is
  plugged. An unplugged VBD does not count.
- **Cost**: one Xen Orchestra API call for the VDIs, plus one call per 50 VDIs for
  their plugged VBDs (`attached? VDI:|(<id> …)`). VBDs of other disks are never fetched.
- **Errors**: if a Xen Orchestra call fails, `ListVolumes` returns an error and the
  attacher skips that cycle. The driver never returns a partial list, because the
  attacher would then process every missing volume again.
- **Pagination**: `starting_token` is an offset into the list sorted by volume ID. An
  invalid token returns `ABORTED`.

## Configuration

- **Reconcile interval**: the external-attacher `--reconcile-sync` flag (default `1m`).
  The Helm chart does not set it. To change it, add `--reconcile-sync=<duration>` to the
  `csi-attacher` container arguments. A shorter interval repairs faster but calls Xen
  Orchestra more often.
- **Several clusters on one Xen Orchestra**: give each cluster its own `--cluster-tag`.
  Otherwise `ListVolumes` also returns the other clusters' volumes. The attacher ignores
  them, but the list is bigger.

## Limitations

### Do not disconnect or remove the VBD of a running pod

The repair is for volumes that are not mounted yet. If a VBD is unplugged while a pod
uses the volume, the driver plugs it back in, but the pod does not recover:

- Linux cannot reattach a mounted filesystem to a disk that comes back. The pod gets
  I/O errors until it is restarted, and recent writes may be lost.
- The node VM can freeze its disk handling. On Linux 6.12 guests (Debian 13),
  unplugging a disk under a mounted ext4 filesystem blocks the kernel's `xenwatch`
  thread forever. After that, the VM processes no more disk plug or unplug events (see
  the next section). Only a hard reboot of the VM recovers it.

The guest cannot refuse the unplug. Xen Orchestra's **Disconnect** and **Remove** actions
on a VBD always force it (`VBD.unplug_force`). Before disconnecting a VBD that belongs
to a Kubernetes volume, stop the pods that use it.

### Node VM that no longer sees new disks

**Symptom**: every new volume on one node fails with `device xvdX does not exist on
this host`, while Xen Orchestra and the `VolumeAttachment` both show the VBD as attached.
The reconciler cannot detect this, because Xen Orchestra and Kubernetes agree.

**Check**, on the node:

```sh
ps -o stat= -p "$(pgrep -x xenwatch)"   # D means xenwatch is stuck
```

**Fix**: drain the node if possible, then hard-reboot the VM from Xen Orchestra. A
normal reboot can hang on the stuck unmount.

### Other cases

- **Device name changes.** After a re-plug, the disk can get another name (`xvdc`
  instead of `xvdb`). The driver reports the new name, and kubelet uses it.
- **Stopped or rebooting node VMs.** A stopped VM has no plugged VBDs, so the attacher
  retries `ControllerPublishVolume` for its volumes, with backoff, until the VM runs
  again. The VBDs are plugged back when the VM boots, and the next retry succeeds without
  creating a new VBD. Expect attacher errors in the logs while the node is down.
- **VDIs with missing tags.** If a volume's VDI lacks the `k8s:volumeId` tag, or the
  cluster tag, the attacher processes its attachment on every cycle. This is harmless,
  and the next attach re-writes the missing tags.

## Verifying the repair

Use a test node and a volume that nothing mounts at the time of the unplug (see
[the limitations above](#do-not-disconnect-or-remove-the-vbd-of-a-running-pod)).
Pausing kubelet gives that window.

1. Create a PVC and a pod on the test node that writes a file to it. Wait until the pod
   is `Running`, then delete the pod and keep the PVC.
2. On the node, stop kubelet. With systemd: `sudo systemctl stop kubelet`. With
   MicroK8s: `sudo snap stop microk8s.daemon-kubelite`. Running containers keep running.
3. Create a pod that reads the file, with `nodeName: <node>`. The volume gets attached
   (`kubectl get volumeattachment` shows `ATTACHED true`), but nothing mounts it. Note
   the VBD in `status.attachmentMetadata.vbd`.
4. In Xen Orchestra, disconnect that VBD. Check that it is no longer attached.
5. Start kubelet again. The pod events may show `NodeStageVolume` failing because the
   device does not exist.
6. Within about one reconcile interval, the attacher logs show the mismatch and then
   `ControllerPublishVolume`:

   ```sh
   kubectl logs deploy/<release>-controller -c csi-attacher | grep -E 'do not match|ControllerPublishVolume'
   ```

   Xen Orchestra shows the VBD attached again.
7. The pod reaches `Running` and reads the file.

When every VBD is plugged, the attacher logs no `do not match` lines.

## See also

- Volume health (`VOLUME_CONDITION`) is not implemented. In Kubernetes it only reports
  problems as events and does not repair them.
- [VDI Lookup and Identification](vdi-lookup-and-identification.md)

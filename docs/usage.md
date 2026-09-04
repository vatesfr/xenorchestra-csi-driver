# Usage and Examples
// TODO: check that file
How to use the driver once it is installed: choose a provisioning mode, create
the matching StorageClass, and attach volumes to your workloads.

> **CSI driver name:** `csi.xenorchestra.vates.tech`

## Contents

- [Dynamic provisioning](#dynamic-provisioning) (recommended)
  - [VAC SR selection (Kubernetes ≥ 1.31)](#vac-sr-selection-kubernetes--131)
  - [Explicit pool](#explicit-pool)
  - [Topology-aware (no poolId)](#topology-aware-no-poolid)
  - [Local SR (host-pinned)](#local-sr-host-pinned)
- [Static provisioning (pre-existing VDI)](#static-provisioning-pre-existing-vdi)
- [Examples reference](#examples-reference)
- [Migrating a volume to another SR with VolumeAttributesClass](#migrating-a-volume-to-another-sr-with-volumeattributesclass)
- [Driver parameters reference](#driver-parameters-reference)

---

## Dynamic provisioning

The driver creates a VDI automatically when a PVC is bound. Three modes are
available, in order of precedence: **VAC SR > explicit poolId > topology-aware**.

### VAC SR selection (Kubernetes ≥ 1.31)

Set `storageRepositoryId` in a `VolumeAttributesClass`. The driver creates the
VDI directly in the specified SR at provision time — no post-creation migration
needed. The SR is validated: it must exist and belong to the pool selected by
`poolId` or topology. If `storageType` is set in the StorageClass, the SR's
shared/local type must match.

The `storageType: local` automatic local-SR override is **not** applied when a
VAC SR is provided — the VDI lands exactly where the VAC points.

```yaml
# VolumeAttributesClass
apiVersion: storage.k8s.io/v1beta1
kind: VolumeAttributesClass
metadata:
  name: csi-xo-specific-sr
driver: csi.xenorchestra.vates.tech
parameters:
  storageRepositoryId: "<sr-uuid>"   # UUID of the target SR
```

```yaml
# PVC referencing the VAC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: xo-csi-pvc-specific-sr
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: csi-xenorchestra-sc-dynamic
  volumeAttributesClassName: csi-xo-specific-sr
```

### Explicit pool

Set `poolId` in the StorageClass parameters. The driver uses that pool's
default SR by default. If `storageType: local` is requested, it later selects
one of the pool's local SRs for the initial placement. The `poolId` is
validated against the pod's topology requirements at provision time — an error
is returned if they are incompatible.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-xenorchestra-sc-dynamic
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: false
parameters:
  poolId: "<xo-pool-uuid>"   # UUID of the target XO pool
```

A complete, ready-to-apply example (StorageClass + PVC + pod) is in
[`examples/csi-app-explicit-pool.yaml`](../examples/csi-app-explicit-pool.yaml).

> **How to find the pool UUID in Xen Orchestra:**
> In the XO web UI, open the pool and copy the UUID from the URL or the pool
> detail page. Alternatively:
> `xo-cli xo.getAllObjects filter='{"type":"pool"}' | jq '.[].id'`

### Topology-aware (no poolId)

Omit `poolId` entirely. The driver selects the pool automatically from the
`accessibility_requirements` passed by the Kubernetes scheduler, following the
CSI spec ordering: **preferred topologies first**, then **requisite topologies**
as fallback. The first pool whose default SR is accessible is used.

This mode requires:
- `volumeBindingMode: WaitForFirstConsumer` — so the scheduler picks a node
  (and therefore a pool topology) before provisioning begins.
- Nodes labelled with `topology.k8s.xenorchestra/pool_id` — set automatically
  by the CCM or the CSI node plugin's `NodeGetInfo`.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-xenorchestra-sc-topology
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: false
# no parameters block required
```

A complete, ready-to-apply example is in
[`examples/csi-app-topology-aware.yaml`](../examples/csi-app-topology-aware.yaml)
(see also [`examples/csi-app-scheduler-driven.yaml`](../examples/csi-app-scheduler-driven.yaml)).

See [Topology and Placement](topology.md) for a detailed explanation of how
pool selection works in each mode.

### Local SR (host-pinned)

Set `storageType: local`. The driver creates the VDI on one of the pool local
SRs at provision time, then migrates it to the target host local SR in
`ControllerPublishVolume` before attaching it to the VM.

This mode requires `volumeBindingMode: WaitForFirstConsumer` (so the target
node is known before provisioning) and every host must have at least one
accessible local user-data SR.

```bash
kubectl apply -f examples/csi-app-local-storage.yaml
```

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-xenorchestra-sc-local
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: false
parameters:
  storageType: local
  # poolId: "<xo-pool-uuid>"   # optional; omit for topology-aware mode
```

See [Local Storage reference](references/local-storage.md) for full details on
SR selection, VDI migration, idempotency, and VM live-migration behaviour.

---

## Static provisioning (pre-existing VDI)

Use a VDI that already exists in Xen Orchestra. No `poolId` is required; the
volume is bound by its raw VDI UUID. This remains supported in v0.4.0: static
volumes use `volumeHandle = <VDI UUID>` and are resolved through a direct VDI
UUID fallback when tag-based lookup does not apply.

1. **Create a VDI in Xen Orchestra.** Use the GUI, CLI, or API to create a
   Virtual Disk Image (VDI). Note its UUID
   (e.g. `b05f63f2-692a-4833-9453-980a73f9f27f`).

2. **Create a StorageClass**:

   ```bash
   kubectl apply -f examples/csi-storageclass.yaml
   ```

   ```yaml
   # examples/csi-storageclass.yaml
   apiVersion: storage.k8s.io/v1
   kind: StorageClass
   metadata:
     name: csi-xenorchestra-sc
   provisioner: csi.xenorchestra.vates.tech
   reclaimPolicy: Delete
   volumeBindingMode: Immediate
   allowVolumeExpansion: false
   ```

3. **Create a PersistentVolume** referencing the VDI UUID:

   ```yaml
   apiVersion: v1
   kind: PersistentVolume
   metadata:
     name: my-xo-pv
   spec:
     storageClassName: csi-xenorchestra-sc
     capacity:
       storage: 10Gi
     accessModes:
       - ReadWriteOnce
     csi:
       driver: csi.xenorchestra.vates.tech
       volumeHandle: "b05f63f2-692a-4833-9453-980a73f9f27f"  # VDI UUID
   ```

4. **Create a PersistentVolumeClaim and use it in a pod.** A complete,
   ready-to-apply example (PV + PVC + pod) is in
   [`examples/csi-app-static.yaml`](../examples/csi-app-static.yaml):

   ```bash
   kubectl apply -f examples/csi-app-static.yaml
   ```

---

## Examples reference

Each file in the [`examples/`](../examples/) directory is a self-contained,
ready-to-apply manifest:

| Example file | Shows |
| ------------ | ----- |
| [`examples/csi-storageclass.yaml`](../examples/csi-storageclass.yaml) | Base StorageClass (static provisioning) |
| [`examples/csi-app-explicit-pool.yaml`](../examples/csi-app-explicit-pool.yaml) | Dynamic, explicit-pool: StorageClass + PVC + pod |
| [`examples/csi-app-topology-aware.yaml`](../examples/csi-app-topology-aware.yaml) | Dynamic, topology-aware: StorageClass + PVC + pod |
| [`examples/csi-app-scheduler-driven.yaml`](../examples/csi-app-scheduler-driven.yaml) | Scheduler-driven placement: StorageClass + PVC + pod |
| [`examples/csi-app-local-storage.yaml`](../examples/csi-app-local-storage.yaml) | Dynamic, host-pinned local SR: StorageClass + PVC + pod |
| [`examples/csi-app-static.yaml`](../examples/csi-app-static.yaml) | Static, pre-existing VDI: PV + PVC + pod |
| [`examples/volumeattributesclass.yaml`](../examples/volumeattributesclass.yaml) | `VolumeAttributesClass` for SR migration |
| [`examples/csi-app-volumeattributesclass.yaml`](../examples/csi-app-volumeattributesclass.yaml) | Full VAC migration example: StorageClass + VAC + PVC + pod |

---

## Migrating a volume to another SR with VolumeAttributesClass

You can move an existing volume to a different Storage Repository (SR) on the
same pool at runtime using a `VolumeAttributesClass`. The `csi-resizer`
sidecar watches for the change and calls the driver, which migrates the VDI.

The target SR must be in the **same pool** as the current VDI, and the VDI
must not be attached to a running VM (the resizer waits for the consuming pod
to stop before migrating).

```bash
# 1. Create the VAC targeting the desired SR (replace the UUID)
kubectl apply -f examples/volumeattributesclass.yaml

# 2. Annotate the PVC to trigger the migration
kubectl annotate pvc example-vac-pvc \
  volumeattributesclass.storage.k8s.io=migrate-to-fast-sr
```

A complete, ready-to-apply example (StorageClass + VAC + PVC + pod) is in
[`examples/csi-app-volumeattributesclass.yaml`](../examples/csi-app-volumeattributesclass.yaml).

---

## Driver parameters reference

### Supported access modes

| Access mode | Supported |
| ----------- | --------- |
| `ReadWriteOnce` | ✅ |
| `ReadWriteMany` | ❌ (planned) |
| `ReadOnlyMany` | ❌ (planned) |

### Static provisioning — volumeHandle fields

| Field | Description | Required | Example |
| ----- | ----------- | -------- | ------- |
| `volumeHandle` | Raw UUID of the existing VDI | Yes | `b05f63f2-692a-4833-9453-980a73f9f27f` |
| `driver` | Must be `csi.xenorchestra.vates.tech` | Yes | — |

### Dynamic provisioning — StorageClass parameters

| Parameter | Description | Required | Example |
| --------- | ----------- | -------- | ------- |
| `poolId` | UUID of the Xen Orchestra pool. The VDI is created on the pool's default SR. If omitted, the pool is selected automatically from `accessibility_requirements` (topology-aware mode). | No | `aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee` |
| `storageType` | Storage placement strategy. `shared` (default): VDI stays on the pool's shared default SR. `local`: VDI is migrated to the target host's local SR in `ControllerPublishVolume`. | No | `local` |

### Dynamic provisioning — VolumeAttributesClass parameters

| Parameter | Description | Required |
| --------- | ----------- | -------- |
| `storageRepositoryId` | UUID of the target Storage Repository. The VDI is created directly in this SR at provision time. The SR must belong to the pool selected by `poolId` or topology. If `storageType` is set in the StorageClass, the SR's shared/local type must match. | Yes |

Any other parameter key in a `VolumeAttributesClass` is rejected with
`InvalidArgument` — the driver only accepts the keys listed above.

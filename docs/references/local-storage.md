# Local Storage: VDI Placement and Migration

## Overview

The `storageType: local` StorageClass parameter instructs the driver to pin each
VDI to a host-local SR. Unlike shared storage (NFS, iSCSI, Ceph…), a local SR is
physically attached to a single XCP-ng host; only VMs running on that host can mount
VDIs stored there.

This document covers:

- [How SR selection works at provision time](#sr-selection-at-createvolume)
- [How VDI migration works at attach time](#vdi-migration-at-controllerpublishvolume)
- [Idempotency guarantees](#idempotency)
- [What happens when a VM is live-migrated](#vm-live-migration)
- [Operational notes and recommendations](#operational-notes)

---

## SR selection at CreateVolume

At `CreateVolume` time the Kubernetes scheduler has picked a node (assuming
`volumeBindingMode: WaitForFirstConsumer`), but the driver does **not** yet know
which host the pod will run on. The target host is only available in
`ControllerPublishVolume`.

To avoid blocking `CreateVolume`, the driver follows a two-phase approach:

1. **Provision phase (`CreateVolume`)**: the pool is selected via the normal
   pool-selection logic (`poolId` parameter or topology-aware mode), then the
   driver calls `FindLocalSRsForPool` and picks one local SR from that pool for
   initial VDI creation.

   If no local SR is available in the selected pool, `CreateVolume` fails
   immediately with `FailedPrecondition`.

2. **Attach phase (`ControllerPublishVolume`)**: once the target node VM is
   resolved, the driver calls `FindLocalSRForHost` to locate the local SR that
   belongs to the target XCP-ng host.

   If the VDI is already on that SR, no migration is needed. Otherwise, the
   driver migrates the VDI to the host's local SR before attaching it.

### `FindLocalSRsForPool`

Uses the XenOrchestra filter:

```
content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $pool:<poolID>
```

- `content_type:user` — only user-data SRs (excludes ISO libraries, XenServer tools, etc.)
- `!shared?` — shared flag is false or missing (host-local SRs)
- `!inMaintenanceMode?` — SR is not in maintenance mode
- `$PBDs:length:>=1` — SR has at least one connected PBD
- `$pool:<poolID>` — scoped to the target pool

### `FindLocalSRForHost`

Uses the XenOrchestra filter:

```
content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $container:<hostID>
```

Same tokens as above, with `$container:<hostID>` scoping the result to a specific
XCP-ng host. Returns the first matching SR (`limit: 1`).

---

## VDI migration at ControllerPublishVolume

When `storageType: local` and the VDI is not already on the correct local SR, the
driver calls `MigrateVDIAndWait`:

1. **`VDI().Migrate(ctx, vdiID, targetSRID)`** — initiates the migration and
   returns a XO task ID.
2. **`Task().Wait(ctx, taskID)`** — blocks until the task completes or fails.
3. On success, `task.Result.ID` (type `uuid.UUID`) is the **new VDI UUID**. The
   original UUID is no longer valid after migration.
4. The driver re-fetches the VDI by its new UUID and continues with
   `IsSRAttachedToHost`, `IsVDIUsedAnywhere`, and `AttachVDIToVM`.

If the task fails or is interrupted, `ControllerPublishVolume` returns
`codes.Internal` with the task status and result details.

---

## Idempotency

The driver checks `vdi.SR == localSR.ID` before initiating migration. If the VDI
is already on the target host's local SR the migration step is skipped entirely.
This makes `ControllerPublishVolume` safe to retry.

---

## Operational notes

### Use `WaitForFirstConsumer`

```yaml
volumeBindingMode: WaitForFirstConsumer
```

This is **strongly recommended** for `storageType: local` StorageClasses. It
ensures the scheduler picks a node before `CreateVolume` runs, so topology hints
(`accessibility_requirements`) are available and the pool can be derived
automatically (topology-aware mode).

Without `WaitForFirstConsumer`, `CreateVolume` may be called before any node is
known. In that case, the driver can still create the VDI on a local SR of the
selected pool, but it may later need an additional migration at
`ControllerPublishVolume` if the workload lands on a different host.

### Every node must have a local SR

`FindLocalSRForHost` returns `FailedPrecondition` if no local SR is found for the
target host. Ensure every XCP-ng host in the pool has at least one accessible
user-data local SR (e.g. `ext`, `lvm`, `zfs`).

### StorageClass example

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

See `examples/csi-app-local-storage.yaml` for a complete PVC + Pod example.

### Performance

Local SRs bypass network storage latency. They are well-suited for
latency-sensitive workloads (databases, caches) that do not require shared access
or cross-node mobility.

### Limitations

- A VDI on a local SR **cannot** be attached to a VM on a different host. If the
  VM is migrated without first migrating the VDI, XAPI will reject the VBD
  attachment. The CSI driver handles this automatically via `ControllerPublishVolume`.
- `ReadWriteMany` is not supported for local storage (not supported in general by
  this driver today).
- Volume expansion is not yet implemented.

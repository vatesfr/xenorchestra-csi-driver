# VDI Lookup and Identification

## Scope

This reference describes how the driver identifies and locates VDIs during
volume lifecycle operations (`DeleteVolume`, `ControllerPublishVolume`,
`ControllerUnpublishVolume`), and documents the known limitations around
`other_config` erasure.

---

## Primary lookup: `other_config`

The driver's stable CSI volume ID is stored at creation time in the VDI
`other_config` map under the key `kubernetes_volume_id`.

Every lookup first queries XO with the filter:

```
other_config:kubernetes_volume_id:<volumeId>
```

This is the authoritative source of truth. Under normal operating conditions
this lookup always succeeds.

---

## Fallback lookup: `name_label`

The VDI `name_label` is set at creation time to:

```
<vdiNamePrefix><volumeId>-<volumeName>
```

For example: `csi-12345678-90ab-cdef-pvc-my-app-data`

Because the CSI `volumeId` UUID is embedded in the name, the driver can
fall back to searching by `name_label` if `other_config` returns no results:

```
name_label:<volumeId>
```

The fallback is transparent to callers — `DeleteVolume`,
`ControllerPublishVolume`, and `ControllerUnpublishVolume` all benefit from it
automatically.

---

## When `other_config` can be erased

The `other_config` map is XenServer/XCP-ng metadata that may be lost in
certain operational scenarios:

- **VDI migration** — migrations do not carry `other_config` to the destination SR.
- **Manual admin edits** — an operator clearing `other_config` via the XAPI shell.

When `other_config:kubernetes_volume_id` is gone the driver automatically
retries using the `name_label` fallback described above.

---

## Limitations

### VDI name_label must not be changed

The `name_label` fallback relies on the VDI name remaining unchanged from
the value set at creation. **Renaming a VDI in Xen Orchestra will break the
fallback** and cause volume operations to fail with `ErrVolumeNotFound` if
`other_config` has also been erased.

> **Do not rename CSI-managed VDIs in Xen Orchestra.**
> CSI-managed VDIs are identifiable by the `csi-` prefix in their name
> and the `kubernetes_created_by` key in their `other_config`.

### Both `other_config` and `name_label` are lost

If both `other_config` and `name_label` are modified or lost, the driver
can no longer locate the VDI automatically. Manual recovery is required:

1. Identify the VDI in XO by its size, SR, and the `name_description`
   field (set to `VDI managed by the Kubernetes CSI; pv-name=<pv-name>`).
2. Restore the `other_config:kubernetes_volume_id` key to the CSI
   volume ID stored in the PersistentVolume's `.spec.volumeHandle`.
3. Or restore the `name_label` to `<vdiNamePrefix><volumeId>-<volumeName>`.

### `DeleteVolume` with no fallback available

If neither `other_config` nor `name_label` allow the VDI to be located,
`DeleteVolume` returns the volume as deleted. Although the PV is deleted
from Kubernetes, the VDI remains in Xen Orchestra.


---

## VDI metadata reference

| Field | Value | Purpose |
|-------|-------|---------|
| `name_label` | `<prefix><volumeId>-<volumeName>` | Human-readable name; used as fallback lookup key |
| `name_description` | `VDI managed by the Kubernetes CSI; pv-name=<pv-name>` | Human-readable only; not used for lookups |
| `other_config:kubernetes_volume_id` | CSI volume ID (UUID) | Primary lookup key |
| `other_config:kubernetes_pv_name` | Kubernetes PV name | Informational |
| `other_config:kubernetes_created_by` | Driver name | Identifies CSI-managed VDIs |

---

## Related documents

- [Volume Handle and Volume ID in v0.3.0](volume-handle-and-volume-id-v0.3.0.md)
- [Migration v0.3.0 to v0.4.0](../migrations/v0.3.0-to-v0.4.0.md)

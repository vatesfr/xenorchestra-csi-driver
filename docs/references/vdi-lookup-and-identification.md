# VDI Lookup and Identification

## Scope

This reference describes how the driver identifies and locates VDIs during
volume lifecycle operations (`DeleteVolume`, `ControllerPublishVolume`,
`ControllerUnpublishVolume`, `ValidateVolumeCapabilities`), and documents the
known limitations around tag erasure and legacy metadata.

---

## Primary lookup: VDI tags

The driver's stable CSI volume ID is stored at creation time in a VDI tag
with the format `k8s:volumeId:<uuid>`.

Every lookup first queries XO with the filter:

```
tags:/^k8s:volumeId:<volumeId>$/
```

This is the authoritative source of truth. Under normal operating conditions
this lookup always succeeds.

Additional tags set at creation time:

| Tag | Value | Purpose |
|-----|-------|---------|
| `k8s:volumeId:<uuid>` | CSI volume ID (UUID) | Primary lookup key |
| `k8s:pvName:<pv-name>` | Kubernetes PV name | Idempotency check in `CreateVolume` |
| `k8s:managedBy:<driver>@<version>` | Driver identifier | Identifies CSI-managed VDIs |

---

## Fallback lookup: `name_label`

The VDI `name_label` is set at creation time to:

```
<vdiNamePrefix><volumeId>-<volumeName>
```

For example: `csi-12345678-90ab-cdef-pvc-my-app-data`

Because the CSI `volumeId` UUID is embedded in the name, the driver can
fall back to searching by `name_label` if the tag lookup returns no results:

```
name_label:<volumeId>
```

When a VDI is found via this fallback, the driver automatically recovers the
`k8s:volumeId` and `k8s:pvName` tags so that future lookups use the primary
path.

The fallback is transparent to callers — `DeleteVolume`,
`ControllerPublishVolume`, `ControllerUnpublishVolume`, and
`ValidateVolumeCapabilities` all benefit from it automatically.

---

## Static volume lookup: direct VDI UUID

Static provisioning uses the raw VDI UUID as the `volumeHandle`. If neither
the tag lookup nor the `name_label` fallback find a match, and the `volumeHandle`
is a valid UUID, the driver attempts a direct VDI lookup by UUID.

This is intentionally the **last** fallback because dynamic volume handles can
also be UUID-shaped, and the driver must avoid returning the wrong VDI.

When a VDI is found via direct UUID lookup, the driver also recovers the
`k8s:volumeId` tag so that future lookups use the primary path.

---

## When tags can be erased

VDI tags may be lost in certain operational scenarios:

- **VDI migration** — migrations do not carry `tags` to the destination SR.
- **Manual admin edits** — an operator removing tags via the XAPI shell or even more easily with the Xen Orchestra web interface.

When the `k8s:volumeId` tag is gone, the driver automatically retries using
the `name_label` fallback described above.

---

## Legacy `other_config` metadata (v0.3.0 and earlier)

v0.3.0 and earlier stored CSI identity in `VDI.other_config` under keys:

- `kubernetes_volume_id`
- `kubernetes_pv_name`
- `kubernetes_created_by`

**v0.4.0 no longer reads `other_config` for runtime lookups.** These keys
remain on migrated VDIs as legacy compatibility data for rollback and manual
recovery, but they are not used by the driver at runtime.

If you are upgrading from v0.3.0, you must apply the
[v0.3.0 to v0.4.0 migration](../migrations/v0.3.0-to-v0.4.0.md) to backfill
the new tag-based metadata. Without it, legacy dynamic volumes will not be
discoverable.

---

## Limitations

### VDI name_label must not be changed

The `name_label` fallback relies on the VDI name remaining unchanged from
the value set at creation. **Renaming a VDI in Xen Orchestra will break the
fallback** and cause volume operations to fail with `ErrVolumeNotFound` if
the `k8s:volumeId` tag has also been erased.

> **Do not rename CSI-managed VDIs in Xen Orchestra.**
> CSI-managed VDIs are identifiable by the `k8s:volumeId` tag, the `csi-`
> prefix in their `name_label`, and the `name_description` field set to
> `VDI managed by the Kubernetes CSI; pv-name=<pv-name>`.

### Both tags and name_label are lost

If both the `k8s:volumeId` tag and the `name_label` are modified or lost,
the driver can no longer locate the VDI automatically. Manual recovery is
required:

1. Identify the VDI in XO by its size, SR, and the `name_description`
   field (set to `VDI managed by the Kubernetes CSI; pv-name=<pv-name>`).
2. Restore the `k8s:volumeId:<volumeHandle>` tag using the CSI volume ID
   stored in the PersistentVolume's `.spec.csi.volumeHandle`.
3. Or restore the `name_label` to `<vdiNamePrefix><volumeHandle>-<pvName>`.

### `DeleteVolume` with no fallback available

If neither tags, `name_label`, nor direct UUID lookup allow the VDI to be
located, `DeleteVolume` returns the volume as deleted. Although the PV is
deleted from Kubernetes, the VDI remains in Xen Orchestra.

---

## VDI metadata reference

| Field | Value | Purpose |
|-------|-------|---------|
| `tags: k8s:volumeId:<uuid>` | CSI volume ID (UUID) | Primary lookup key |
| `tags: k8s:pvName:<pv-name>` | Kubernetes PV name | Idempotency in `CreateVolume` |
| `tags: k8s:managedBy:<driver>@<version>` | Driver identifier | Identifies CSI-managed VDIs |
| `name_label` | `<prefix><volumeId>-<volumeName>` | Human-readable name; fallback lookup key |
| `name_description` | `VDI managed by the Kubernetes CSI; pv-name=<pv-name>` | Human-readable only; not used for lookups |
| `other_config:kubernetes_volume_id` | CSI volume ID (UUID) | **Legacy** — v0.3.0 and earlier only |
| `other_config:kubernetes_pv_name` | Kubernetes PV name | **Legacy** — v0.3.0 and earlier only |
| `other_config:kubernetes_created_by` | Driver name | **Legacy** — v0.3.0 and earlier only |

---

## Related documents

- [Volume Handle and Volume ID in v0.3.0](volume-handle-and-volume-id-v0.3.0.md)
- [Migration v0.3.0 to v0.4.0](../migrations/v0.3.0-to-v0.4.0.md)

## Volume Handle and Volume ID in v0.3.0

## Scope

This reference explains the volume identity model introduced in v0.3.0 for
xenorchestra-csi.

For step-by-step upgrade actions, see the migration guide:
[Migration v0.2.0 to v0.3.0](../migrations/v0.2.0-to-v0.3.0.md).

## Terminology

- VDI UUID: Xen Orchestra disk UUID backing a volume.
- CSI volumeHandle: the stable identity Kubernetes stores in PersistentVolume.
- CSI volume ID: internal value used by the driver for lookup and lifecycle
  operations. In this driver, this is the same value exposed as volumeHandle.

## Behavior in v0.2.0

In v0.2.0, volume identity for existing operational paths was tied to the VDI
UUID.

This design works while the backend UUID remains unchanged, but it couples CSI
identity to a storage-level identifier. The VDI UUID is not guaranteed to be
stable: it changes when a VDI is migrated to another Storage Repository (SR),
for example during a storage live migration or a manual VDI move between SRs.

## Behavior in v0.3.0

In v0.3.0, the driver uses a stable CSI volume ID stored in VDI metadata under:
- other-config key: kubernetes_volume_id

This key is represented in code by:
- VDIOtherConfigKeyVolumeId = kubernetes_volume_id

The objective is to decouple CSI identity from backend VDI UUID lifecycle.

## Why this change

A CSI volume identity should remain stable from Kubernetes point of view.

Backend operations can change VDI UUID in some workflows (for example storage
relocation patterns). If the CSI identity depends directly on that UUID,
reconciliation and lookup become fragile.

Using a dedicated metadata key makes the identity explicit and stable.

## Practical implications

- New volumes created with v0.3.0 get a dedicated CSI volume ID in
  other-config:kubernetes_volume_id.
- Legacy volumes created before v0.3.0 may not have this key.
- Legacy volumes should be backfilled during migration so that operational
  lookup remains consistent after upgrade.

## Metadata key reference

- Key: kubernetes_volume_id
- Location: VDI other-config
- Expected value:
  - v0.3.0 native volumes: generated stable CSI volume ID,
  - migrated legacy volumes: set to current VDI UUID as compatibility backfill.

## Related keys

Other keys commonly present in VDI other-config:
- kubernetes_created_by
- kubernetes_pv_name

These keys are informational and separate from the CSI stable volume identity
key introduced for v0.3.0.

## Documentation Index

Evergreen guides live at the top level of this folder; version-specific
material is grouped into subfolders:
- `migrations/`: version-to-version upgrade procedures,
- `references/`: version-specific technical behavior and concepts.

## Current guides

- [Installation guide](install.md) – entry point: requirements, CCM dependency, credentials, and how to choose an installation method.
- [Installation with Helm](install-helm.md) – Helm chart from the OCI registry or a local checkout, component toggles, MicroK8s, and the Helm test.
- [Installation with static deployment files](install-static.md) – apply the pre-rendered manifests in `deploy/` with `kubectl`.
- [Usage and Examples](usage.md) – provisioning modes, StorageClasses, dynamic and static examples, VAC migration, and the driver parameters reference.
- [Topology and Placement](topology.md)
- [Developer guide](development.md)
- [Release process](release.md)

## Migrations

- [v0.2.0 to v0.3.0](migrations/v0.2.0-to-v0.3.0.md)
- [v0.3.0 to v0.4.0](migrations/v0.3.0-to-v0.4.0.md) *(required for existing v0.3.0 dynamic volumes — migrate metadata from `other_config` to tags)*

## References

- [Volume Handle and Volume ID in v0.3.0](references/volume-handle-and-volume-id-v0.3.0.md)
- [Local Storage: VDI Placement and Migration](references/local-storage.md)
- [VDI Lookup and Identification](references/vdi-lookup-and-identification.md)

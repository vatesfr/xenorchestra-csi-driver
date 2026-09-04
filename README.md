# XenOrchestra CSI Driver for Kubernetes

A Container Storage Interface (CSI) driver that provides persistent storage for Kubernetes workloads using XenServer/XCP-ng infrastructure through Xen Orchestra.

This repository hosts the CSI driver and all of its build and dependent configuration files to deploy the driver.

The Xen Orchestra CCM is **required**. Without it, `spec.providerID` is not set
on Node objects, `NodeGetInfo` fails, and the node-driver-registrar enters
**CrashLoopBackOff** — the CSI node plugin never registers with kubelet and no
volume operations are possible on that node (see [Topology and Placement](docs/topology.md)).

* csi plugin name: `csi.xenorchestra.vates.tech`
* supported accessModes: `ReadWriteOnce`

---
> **⚠️ WARNING**  
> This driver is currently under development. It contains unimplemented methods, shortcuts, and non-standard practices. **DO NOT use in production environments.**
---

## Features

- Static volume provisioning (use an existing VDI by UUID).
- Dynamic volume provisioning (automatically create a VDI from a StorageClass).
- Topology-aware pool selection: automatic pool placement from the pod's topology hints when no `poolId` is set.
- Local storage support: pin VDIs to a host-local SR with automatic migration on reschedule.
- Volume migration via `VolumeAttributesClass`: create a VDI directly in a specific Storage Repository (SR),
  or migrate it after creation within the same pool.

## Prerequisites

See the [installation guide](docs/install.md#requirements) for the full
requirements list (XCP-ng 8.3+, Xen Orchestra 6.4+, Kubernetes 1.26+, network
connectivity to the XO API, and the required CCM).

## Documentation

- [Documentation index](docs/README.md) – entry point to guides, migrations, and references.
- [Installation guide](docs/install.md) – requirements, CCM dependency, credentials, and how to choose an installation method.
- [Installation with Helm](docs/install-helm.md) – Helm chart from the OCI registry or a local checkout, component toggles, MicroK8s, and the Helm test.
- [Installation with static deployment files](docs/install-static.md) – apply the pre-rendered manifests with `kubectl`.
- [Usage and Examples](docs/usage.md) – provisioning modes, StorageClasses, dynamic and static examples, VAC migration, and the driver parameters reference.
- [Topology and Placement](docs/topology.md) – pool boundary, live migration behaviour, CCM dependency.
- [Developer guide](docs/development.md) – build, `kxo` helper, DevSpace, MicroK8s registry, remote debugging.
- [Reference: Volume Handle and Volume ID in v0.3.0](docs/references/volume-handle-and-volume-id-v0.3.0.md) – details about stable CSI identity semantics.
- [Reference: VDI Lookup and Identification](docs/references/vdi-lookup-and-identification.md) – how VDIs are located, tag-based lookup, fallback behaviour, and limitations.
- [Reference: Local Storage: VDI Placement and Migration](docs/references/local-storage.md) – SR selection, VDI migration, idempotency, and VM live-migration behaviour.

## Version migrations

When upgrading between versions, some releases require or recommend metadata
changes to existing VDIs. Each guide is self-contained and includes rollback
instructions.

- [v0.2.0 to v0.3.0](docs/migrations/v0.2.0-to-v0.3.0.md) – **required** — backfill `other-config:kubernetes_volume_id` on legacy VDIs.
- [v0.3.0 to v0.4.0](docs/migrations/v0.3.0-to-v0.4.0.md) – **required** — migrate VDI metadata from `other_config` to tags (mandatory for existing v0.3.0 dynamic volumes).

## Limitations

### Do not rename CSI-managed VDIs in Xen Orchestra

The driver uses the VDI `name_label` as a fallback lookup when the
`k8s:volumeId:<volumeId>` tag is missing (e.g. after tag erasure). The
`name_label` is set at creation time
to `<prefix><volumeId>-<volumeName>`.

**Renaming a VDI in Xen Orchestra breaks this fallback.** If the tag
has also been erased, the driver will no longer be able to locate the VDI and
volume operations (`DeleteVolume`, `ControllerUnpublishVolume`) will fail and
the VDI will be considered deleted.

CSI-managed VDIs are identifiable by:
- the `k8s:volumeId:<volumeId>` tag,
- the `csi-` prefix in their `name_label` (or the prefix set with the flag `--vdi-name-prefix`),
- the `name_description` field set to `VDI managed by the Kubernetes CSI; pv-name=<pv-name>`.

See [VDI Lookup and Identification](docs/references/vdi-lookup-and-identification.md)
for full details and manual recovery steps.

## Install driver on a Kubernetes cluster

Install the driver with Helm, reusing the Xen Orchestra CCM credentials Secret:

```bash
helm upgrade --install xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver
```

See the [installation guide](docs/install.md) for requirements, credentials,
and how to choose between the [Helm chart](docs/install-helm.md) and the
[static deployment files](docs/install-static.md). StorageClasses, component
toggles, and usage examples live in the
[Usage and Examples guide](docs/usage.md).


## Driver parameters

For provisioning modes, StorageClass examples, dynamic and static usage,
`VolumeAttributesClass` migration, and the full driver parameter reference, see
the [Usage and Examples guide](docs/usage.md).


## 🚀 TODO / Roadmap

### Core CSI Operations
- [x] Dynamic Volume Provisioning (Create VDIs from a StorageClass)
- [x] Delete VDIs when a PV is released (`reclaimPolicy: Delete`)
- [ ] Read only full-support
- [ ] Volume Expansion
- [ ] Volume Snapshots

### Storage Management
- [ ] Volume Listing
- [ ] Storage Capacity
- [-] Volume Validation, Information, Modification
- [ ] Access modes - Add ReadWriteMany and ReadOnlyMany support

### Security & Configuration
- [x] Use with Xen Orchestra Cloud Controller Manager
- [x] Alternative for credential management (environment variables supported)
- [ ] Check RBAC policies

### Performance & Monitoring
- [ ] Metrics endpoint
- [x] Switch completely to the Xen Orchestra REST API

### Other
- [ ] Complete the documentation (installation, configuration, examples...)
- [x] Provide improved deployment methods (using kubectl, Helm or other)
- [ ] Test with other Kubernetes clusters (Talos, Rancher, etc.)

### CI & Testing
- [x] Proper CI pipelines
- [ ] Unit test and integration tests

### XO related
- [x] Pool selection via `StorageClass.parameters.poolId`
- [x] Topology-aware pool selection from `accessibility_requirements` (no `poolId` required)
- [x] `poolId` validation against `accessibility_requirements` requisite topologies
- [x] `VOLUME_ACCESSIBILITY_CONSTRAINTS` controller capability — `AccessibleTopology` returned in `CreateVolumeResponse`, topology requirements honoured in `CreateVolumeRequest`
- [x] Cluster tag filtering (`--cluster-tag`; VDIs tagged at creation)
- [x] Cluster Topology support
- [x] Multi-SR support (migration...)
- [x] Local SR support (`storageType: local` — VDI migration to host-local SR in `ControllerPublishVolume`)
- [x] Multi-pool support
- [x] XO CCM

## Contributing

Contributions are what make the open source community such an amazing place to be learn, inspire, and create. Any contributions you make are **greatly appreciated**.

## License

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

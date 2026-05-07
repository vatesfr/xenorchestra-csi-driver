# XenOrchestra CSI Driver for Kubernetes

A Container Storage Interface (CSI) driver that provides persistent storage for Kubernetes workloads using XenServer/XCP-ng infrastructure through Xen Orchestra.

This repository hosts the CSI driver and all of its build and dependent configuration files to deploy the driver.

The Xen Orchestra CCM is **required** when using the default
`--node-metadata-source=kubernetes` mode. Without it, `spec.providerID` is not set
on Node objects, `NodeGetInfo` fails, and the node-driver-registrar enters
**CrashLoopBackOff** — the CSI node plugin never registers with kubelet and no
volume operations are possible on that node. Use `--node-metadata-source=xo-api` if
you cannot run the CCM (see [Topology and Placement](docs/topology.md)).

* csi plugin name: `csi.xenorchestra.vates.tech`
* supported accessModes: `ReadWriteOnce`

---
> **⚠️ WARNING**  
> This driver is currently under development. It contains unimplemented methods, shortcuts, and non-standard practices. **DO NOT use in production environments.**
---

## Features

- Static volume provisioning (use an existing VDI by UUID).
- Dynamic volume provisioning (automatically create a VDI from a StorageClass).

## Prerequisite

* XenOrchestra version 5.110.1+
* XCP-ng version 8.3+
* Network connectivity between Kubernetes nodes and XO API

## Documentation

- [Installation guide](docs/install.md) – requirements, credentials, StorageClass, static and dynamic provisioning.
- [Topology and Placement](docs/topology.md) – pool boundary, live migration behaviour, CCM dependency.
- [Developer guide](docs/development.md) – build, `kxo` helper, DevSpace, MicroK8s registry, remote debugging.

## Install driver on a Kubernetes cluster

An example of an install with kubectl can be found in the './deploy' folder. You may need to adjust this file to fit your actual installation; see below for more information.

### Prerequisites
* The driver depends on the CCM credentials config secret ([See here](https://github.com/vatesfr/xenorchestra-cloud-controller-manager/blob/main/docs/install.md#deploy-ccm)).

### Quick Start with the PoC

```bash
# Create a registry credential secret for ghcr.io
kubectl -n kube-system create secret docker-registry regcred --docker-server=<your-registry-server> --docker-username=<your-name> --docker-password=<your-pword> --docker-email=<your-email>

# Install the driver
./deploy/install-driver.sh

# Alternative: Install the driver from local repo
./deploy/install-driver.sh local

# Create a StorageClass
kubectl apply -f examples/csi-storageclass.yaml

# Install the driver
./deploy/uninstall-driver.sh
```

### MicroK8s

Use `/var/snap/microk8s/common/var/lib/kubelet/`

### Other Kubelet installation

Use `/var/lib/kubelet/`


## Driver parameters

### Static provisioning

Manually attach a disk to the node VM.
> [Get an example](./examples/pv-volume.yaml)

1. Create a VDI using the Xen Orchestra GUI, or any other tools such as CLI, API or Terraform.
2. Create a persistent volume (PV) and enter the UUID of the VDI created in Step 1 into the 'volumeHandle' property.
3. Use the PV with a PVC and then mount the volume inside your pod.

Name | Meaning | Example | Required | Default
--- | --- | --- | --- | ---
`volumeHandle` | Disk identifier, it must be the VDI UUID | `b05f63f2-692a-4833-9453-980a73f9f27f` | Yes | N/A
`driver` | Driver to use for the PV | it must be `csi.xenorchestra.vates.tech` | Yes | N/A

### Dynamic provisioning

The driver creates a new VDI in a pool's **default SR** each time a PVC is bound.
Two modes are supported:
> [Get an example](./examples/csi-sc-dynamic.yaml)

#### Explicit pool (simple)

Set `poolId` in `StorageClass.parameters`. The driver always provisions into that pool.
The `poolId` is validated against the pod's topology requirements at provision time —
an error is returned if they are incompatible.

Name | Meaning | Example | Required | Default
--- | --- | --- | --- | ---
`poolId` | UUID of the Xen Orchestra pool. The VDI is created on the pool's default SR. | `aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee` | No | —

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
  poolId: "<xo-pool-uuid>"
```

#### Topology-aware (no poolId)

Omit `poolId` entirely. The driver selects the pool automatically from the
`accessibility_requirements` passed by the Kubernetes scheduler, following the
CSI spec ordering: **preferred topologies first**, then **requisite topologies**
as fallback. The first pool whose default SR is accessible is used.

This mode requires `volumeBindingMode: WaitForFirstConsumer` and nodes labelled
with `topology.k8s.xenorchestra/pool_id` (set by the CCM or the CSI node plugin).

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
- [ ] Volume Validation, Information, Modification
- [ ] Access modes - Add ReadWriteMany and ReadOnlyMany support

### Security & Configuration
- [x] Use with Xen Orchestra Cloud Controller Manager
- [x] Alternative for credential management (environment variables supported)
- [ ] Check RBAC policies

### Performance & Monitoring
- [ ] Metrics endpoint
- [ ] Switch completely to the Xen Orchestra REST API

### Other
- [ ] Complete the documentation (installation, configuration, examples...)
- [ ] Provide improved deployment methods (using kubectl, Helm or other)
- [ ] Test with other Kubernetes clusters (Talos, Rancher, etc.)

### CI & Testing
- [ ] Proper CI pipelines
- [ ] Unit test and integration tests

### XO related
- [x] Pool selection via `StorageClass.parameters.poolId`
- [x] Topology-aware pool selection from `accessibility_requirements` (no `poolId` required)
- [x] `poolId` validation against `accessibility_requirements` requisite topologies
- [x] `VOLUME_ACCESSIBILITY_CONSTRAINTS` controller capability — `AccessibleTopology` returned in `CreateVolumeResponse`, topology requirements honoured in `CreateVolumeRequest`
- [x] Cluster tag filtering (`--cluster-tag`; VDIs tagged at creation)
- [ ] Cluster Topology support
- [ ] Multi-SR support (migration...)
- [ ] Multi-pool support
- [ ] XO CCM

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

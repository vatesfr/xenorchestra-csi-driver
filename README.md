# XenOrchestra CSI Driver for Kubernetes

A Container Storage Interface (CSI) driver that provides persistent storage for Kubernetes workloads using XenServer/XCP-ng infrastructure through Xen Orchestra.

This repository hosts the CSI driver and all of its build and dependent configuration files to deploy the driver.

It is recommended to install the Xen Orchestra CCM in addition to the CSI driver.

* csi plugin name: `csi.xenorchestra.vates.tech`
* supported accessModes: `ReadWriteOnce`

---
> **⚠️ WARNING**  
> This driver is currently under development. It contains unimplemented methods, shortcuts, and non-standard practices. **DO NOT use in production environments.**
---

## Features

- Static volume provisioning (use an existing VDI by UUID).

## Prerequisite

* XenOrchestra version 5.110.1+
* XCP-ng version 8.3+
* Network connectivity between Kubernetes nodes and XO API

## Documentation

TODO. See below for a short one.

## Install driver on a Kubernetes cluster

An example of an install with kubectl can be found in the './deploy' folder. You may need to adjust this file to fit your actual installation; see below for more information.

### Prerequisites
* The driver depends on the CCM credentials config secret ([See here](https://github.com/vatesfr/xenorchestra-cloud-controller-manager/blob/main/docs/install.md#deploy-ccm)).

### Quick Start with the PoC

```bash
# Install the driver
./deploy/install-driver.sh

# Create a StorageClass
kubectl apply -f csi-storageclass.yaml

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


## 🚀 TODO / Roadmap

### Core CSI Operations
- [ ] Dynamic Volume Provisioning (Create / Delete VDIs)
- [ ] Read only full-support
- [ ] Volume Expansion
- [ ] Volume Snapshots

### Storage Management
- [ ] Volume Listing
- [ ] Storage Capacity
- [ ] Volume Validation, Information, Modification
- [ ] Access modes - Add ReadWriteMany and ReadOnlyMany support

### Security & Configuration
- [ ] Use with Xen Orchestra Cloud Controller Manager
- [ ] Alternative for credential management (env variables?)
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

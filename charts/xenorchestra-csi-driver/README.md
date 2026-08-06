# xenorchestra-csi-driver

![Version: 0.4.0](https://img.shields.io/badge/Version-0.4.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.4.0](https://img.shields.io/badge/AppVersion-v0.4.0-informational?style=flat-square)

CSI driver for Xen Orchestra

CSI driver for Xen Orchestra: it provides persistent storage for Kubernetes workloads on XenServer/XCP-ng infrastructure, with dynamic volume provisioning (one VDI per PVC) and support for pinning volumes to a pool or to a host-local storage repository.

> **Warning**
> This driver is under active development and must not be used in production.

**Homepage:** <https://github.com/vatesfr/xenorchestra-csi-driver>

## Source Code

* <https://github.com/vatesfr/xenorchestra-csi-driver>

## Requirements

| Component | Minimum version |
| --------- | --------------- |
| XCP-ng | 8.3+ |
| Xen Orchestra | 5.110.1+ |
| Kubernetes | 1.26+ |
| Helm | 3+ |

The chart defaults to `driver.nodeMetadataSource=kubernetes`, which requires the
[XenOrchestra Cloud Controller Manager](https://github.com/vatesfr/xenorchestra-cloud-controller-manager):
the CCM sets `spec.providerID` on each Node and the CSI node plugin uses it to
resolve the pool and VM identities. Without the CCM, set
`driver.nodeMetadataSource=xo-api` so the driver queries the Xen Orchestra API directly.

## Create the credentials secret

Create a `config.yaml` file for the driver:

```yaml
# xo-config.yaml
url: https://xo.example.com
insecure: false
token: "<your-xo-api-token>"
```

Store it as a Kubernetes secret:

```shell
kubectl -n kube-system create secret generic xenorchestra-csi-driver \
  --from-file=config.yaml=xo-config.yaml
```

If the CCM is already installed, you can reuse its secret by setting
`existingConfigSecret` instead of creating a new one. Credentials may also be
provided inline under `config` (the chart then creates the secret) or through
`driver.envFrom`.

## Install the chart

```shell
helm upgrade --install xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-csi-driver \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver
```

The controller, node plugin, and `CSIDriver` resource can be toggled independently
with `controller.enabled`, `node.enabled`, and `csidriver.enabled`.

See the [installation guide](https://github.com/vatesfr/xenorchestra-csi-driver/blob/main/docs/install.md)
for StorageClass examples, testing, and the full parameter reference.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"ghcr.io/vatesfr/xenorchestra-csi-driver"` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.tag | string | `""` |  |
| imagePullSecrets | list | `[]` |  |
| nameOverride | string | `""` |  |
| fullnameOverride | string | `""` |  |
| existingConfigSecret | string | `""` |  |
| existingConfigSecretKey | string | `"config.yaml"` |  |
| config | object | `{}` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.controllerName | string | `""` |  |
| serviceAccount.nodeName | string | `""` |  |
| rbac.create | bool | `true` |  |
| csidriver.enabled | bool | `true` |  |
| driver.name | string | `"csi.xenorchestra.vates.tech"` |  |
| driver.logVerbosityLevel | int | `2` |  |
| driver.clusterTag | string | `"k8s-managed"` |  |
| driver.nodeMetadataSource | string | `"kubernetes"` |  |
| driver.extraArgs | list | `[]` |  |
| driver.extraEnv | list | `[]` |  |
| driver.envFrom | list | `[]` |  |
| controller.enabled | bool | `true` |  |
| controller.replicas | int | `1` |  |
| controller.priorityClassName | string | `"system-cluster-critical"` |  |
| controller.hostNetwork | bool | `false` |  |
| controller.dnsPolicy | string | `"ClusterFirstWithHostNet"` |  |
| controller.nodeSelector."kubernetes.io/os" | string | `"linux"` |  |
| controller.tolerations[0].key | string | `"node-role.kubernetes.io/master"` |  |
| controller.tolerations[0].operator | string | `"Exists"` |  |
| controller.tolerations[0].effect | string | `"NoSchedule"` |  |
| controller.tolerations[1].key | string | `"node-role.kubernetes.io/control-plane"` |  |
| controller.tolerations[1].operator | string | `"Exists"` |  |
| controller.tolerations[1].effect | string | `"NoSchedule"` |  |
| controller.tolerations[2].key | string | `"CriticalAddonsOnly"` |  |
| controller.tolerations[2].operator | string | `"Exists"` |  |
| controller.tolerations[2].effect | string | `"NoSchedule"` |  |
| controller.affinity | object | `{}` |  |
| controller.podAnnotations | object | `{}` |  |
| controller.resources.requests.cpu | string | `"10m"` |  |
| controller.resources.requests.memory | string | `"20Mi"` |  |
| controller.resources.limits.memory | string | `"500Mi"` |  |
| node.enabled | bool | `true` |  |
| node.priorityClassName | string | `"system-node-critical"` |  |
| node.hostNetwork | bool | `false` |  |
| node.dnsPolicy | string | `"ClusterFirstWithHostNet"` |  |
| node.kubeletRootDir | string | `"/var/lib/kubelet"` |  |
| node.nodeSelector."kubernetes.io/os" | string | `"linux"` |  |
| node.tolerations[0].operator | string | `"Exists"` |  |
| node.affinity | object | `{}` |  |
| node.podAnnotations | object | `{}` |  |
| node.resources.requests.cpu | string | `"10m"` |  |
| node.resources.requests.memory | string | `"20Mi"` |  |
| node.resources.limits.memory | string | `"1000Mi"` |  |
| tests.enabled | bool | `false` |  |
| tests.storageClassName | string | `""` |  |
| tests.size | string | `"1Gi"` |  |
| tests.image.repository | string | `"busybox"` |  |
| tests.image.tag | string | `"1.37.0"` |  |
| tests.image.pullPolicy | string | `"IfNotPresent"` |  |
| tests.nodeSelector | object | `{}` |  |
| tests.tolerations | list | `[]` |  |
| tests.affinity | object | `{}` |  |
| sidecars.livenessProbe.image.repository | string | `"registry.k8s.io/sig-storage/livenessprobe"` |  |
| sidecars.livenessProbe.image.tag | string | `"v2.18.0"` |  |
| sidecars.livenessProbe.image.pullPolicy | string | `"IfNotPresent"` |  |
| sidecars.livenessProbe.resources.requests.cpu | string | `"10m"` |  |
| sidecars.livenessProbe.resources.requests.memory | string | `"20Mi"` |  |
| sidecars.livenessProbe.resources.limits.memory | string | `"100Mi"` |  |
| sidecars.attacher.image.repository | string | `"registry.k8s.io/sig-storage/csi-attacher"` |  |
| sidecars.attacher.image.tag | string | `"v4.11.0"` |  |
| sidecars.attacher.image.pullPolicy | string | `"IfNotPresent"` |  |
| sidecars.provisioner.image.repository | string | `"registry.k8s.io/sig-storage/csi-provisioner"` |  |
| sidecars.provisioner.image.tag | string | `"v6.2.0"` |  |
| sidecars.provisioner.image.pullPolicy | string | `"IfNotPresent"` |  |
| sidecars.registrar.image.repository | string | `"registry.k8s.io/sig-storage/csi-node-driver-registrar"` |  |
| sidecars.registrar.image.tag | string | `"v2.16.0"` |  |
| sidecars.registrar.image.pullPolicy | string | `"IfNotPresent"` |  |
| sidecars.registrar.resources.requests.cpu | string | `"10m"` |  |
| sidecars.registrar.resources.requests.memory | string | `"20Mi"` |  |
| sidecars.registrar.resources.limits.memory | string | `"100Mi"` |  |

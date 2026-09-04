# Install with Helm

Install the driver from the Helm chart. The chart is published on the GitHub
Container Registry (`ghcr.io`) and is also available locally in
[`./charts/xenorchestra-csi-driver`](../charts/xenorchestra-csi-driver).

Complete the prerequisites and the credentials step in the
[Installation guide](install.md) before installing the chart.

## From the OCI registry (published chart)

```bash
helm upgrade --install xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver
```

## From a local checkout

Install the chart straight from the repository, which is handy for development
or when you want to try an unreleased change:

```bash
helm upgrade --install xenorchestra-csi-driver \
  ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager
```

To install a specific local variant, pass the matching values file:

```bash
# Edge variant (edge image, verbose logging)
helm upgrade --install xenorchestra-csi-driver \
  ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  --values charts/xenorchestra-csi-driver/values.edge.yaml

# MicroK8s variant (correct kubelet path)
helm upgrade --install xenorchestra-csi-driver \
  ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  --values charts/xenorchestra-csi-driver/values.microk8s.yaml
```

## Component toggles

The controller, node plugin, and `CSIDriver` resource can be managed
independently:

| Value | Default | Resource |
| ----- | ------- | -------- |
| `controller.enabled` | `true` | Controller Deployment |
| `node.enabled` | `true` | Node DaemonSet |
| `csidriver.enabled` | `true` | `CSIDriver` resource |

Disabling a component also skips its ServiceAccount and RBAC rules. For
example, to install only the controller:

```bash
helm upgrade --install xenorchestra-csi-driver \
  --namespace kube-system \
  --set controller.enabled=true \
  --set node.enabled=false \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver
```

For a node-only installation, invert `controller.enabled` and `node.enabled`.
If the `CSIDriver` object is managed elsewhere, set `csidriver.enabled=false`.

## MicroK8s kubelet path

MicroK8s stores kubelet data under
`/var/snap/microk8s/common/var/lib/kubelet` instead of the standard
`/var/lib/kubelet`. Set the correct path when installing the chart from OCI:

```bash
helm upgrade --install xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  --set node.kubeletRootDir=/var/snap/microk8s/common/var/lib/kubelet \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver
```

When installing from a local checkout, the equivalent override is already
provided in `charts/xenorchestra-csi-driver/values.microk8s.yaml` (see
[From a local checkout](#from-a-local-checkout)).

## Test the installation

The optional Helm test provisions a real volume through an existing
StorageClass, mounts it in a pod, then writes and reads a file. The test does
not create a StorageClass: `tests.storageClassName` must refer to one that
already exists and uses this CSI driver.

If you created one of the StorageClasses described in the
[Usage guide](usage.md), use its name and skip the creation step below. If no
suitable StorageClass exists, create one for the test. For example, the
following StorageClass is pinned to a Xen Orchestra pool; replace
`<xo-pool-uuid>` with the UUID of a pool accessible from the Kubernetes nodes:

```bash
kubectl apply -f - <<'EOF'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: xo-sc-explicit-pool
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: false
parameters:
  poolId: "<xo-pool-uuid>"
EOF

kubectl get storageclass xo-sc-explicit-pool
```

Enable and run the test. Replace `xo-sc-explicit-pool` with the name of your
existing StorageClass if you did not create the example above:

```bash
# Replace your storageClassName if you already have one
helm upgrade xenorchestra-csi-driver \
  --namespace kube-system \
  --reuse-values \
  --set tests.enabled=true \
  --set tests.storageClassName=xo-sc-explicit-pool \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver

helm test xenorchestra-csi-driver --namespace kube-system --logs
```

The test uses a generic ephemeral PVC and creates a real VDI. On success, Helm
removes the pod and Kubernetes removes the PVC. The StorageClass reclaim policy
should be `Delete` so the test VDI is removed as well.

Failed test pods are retained for diagnosis. Delete one before rerunning the
test:

```bash
kubectl delete pod xenorchestra-csi-driver-test --namespace kube-system
```

## Values

The full list of chart values, with their defaults, is generated by `helm-docs`
and documented in the chart's
[README](../charts/xenorchestra-csi-driver/README.md). Credentials are accepted
in three ways: an existing secret (`existingConfigSecret`), an inline `config`
object (the chart creates the secret), or environment variables via
`driver.envFrom`. See the
[credentials step](install.md#create-the-credentials-secret) for details.

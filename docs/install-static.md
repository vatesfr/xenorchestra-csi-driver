# Install with static deployment files
// TODO: check that file
If you prefer to manage the manifests directly with `kubectl` instead of Helm,
pre-rendered deployment files are committed to
[`docs/deploy/`](deploy/). Each file is a complete, ready-to-apply manifest
set (ServiceAccounts, RBAC, controller Deployment, node DaemonSet, and
`CSIDriver`).

Complete the prerequisites and the credentials step in the
[Installation guide](install.md) before applying the manifests.

> ℹ️ These files are generated from the Helm chart by `make docs`. To regenerate
> them after a chart change, run `make docs` from the repository root. Do not
> edit them by hand — update the chart and regenerate.

## Choose a variant

| File | Variant |
| ---- | ------- |
| [`docs/deploy/csi-driver.yml`](deploy/csi-driver.yml) | Default (latest stable image) |
| [`docs/deploy/csi-driver-edge.yml`](deploy/csi-driver-edge.yml) | Edge image, verbose logging |
| [`docs/deploy/csi-driver-microk8s.yml`](deploy/csi-driver-microk8s.yml) | MicroK8s kubelet path |

## 1. Create the credentials secret

The manifests mount a Secret named **`xenorchestra-csi-driver`** that must
contain a `config.yaml` key. Create it in `kube-system`:

```bash
kubectl -n kube-system create secret generic xenorchestra-csi-driver \
  --from-file=config.yaml=xo-config.yaml
```

where `xo-config.yaml` is:

```yaml
url: https://<xen-orchestra-host>
insecure: false   # set to true only when using a self-signed certificate
token: "<your-xo-api-token>"
```

> ℹ️ If you already run the CCM and want to reuse its credentials, create the
> secret under the name `xenorchestra-csi-driver` anyway, or edit the
> `xo-cloud-config` volume in the controller Deployment to point at your
> existing secret name.

## 2. Apply the manifests

```bash
kubectl apply -f docs/deploy/csi-driver.yml
```

(Use `csi-driver-edge.yml` or `csi-driver-microk8s.yml` for the matching
variant.)

## 3. Verify

Verify that the pods are running:

```bash
kubectl -n kube-system get pods \
  -l app.kubernetes.io/instance=xenorchestra-csi-driver
```

## Uninstall

Delete the rendered resources. The cleanest way is to render the same variant
and delete it, or delete by label:

```bash
kubectl -n kube-system delete -f docs/deploy/csi-driver.yml

# or by label
kubectl -n kube-system delete pod,sa -l app.kubernetes.io/instance=xenorchestra-csi-driver
kubectl delete clusterrole,clusterrolebinding \
  -l app.kubernetes.io/instance=xenorchestra-csi-driver
kubectl delete csidriver csi.xenorchestra.vates.tech
```

To also remove the credentials secret (if not used by the CCM):

```bash
kubectl -n kube-system delete secret xenorchestra-csi-driver
```

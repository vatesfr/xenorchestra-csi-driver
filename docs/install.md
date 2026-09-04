# Installation Guide

This guide walks you through installing the XenOrchestra CSI driver on a
Kubernetes cluster.

> **⚠️ Warning**
> This driver is currently under active development.
> It contains unimplemented features. **Do not use in production.**

## Contents

- [Requirements](#requirements)
- [Choose your installation method](#choose-your-installation-method)
- [Shared steps](#shared-steps)
  - [Create the credentials secret](#create-the-credentials-secret)
  - [Verify the installation](#verify-the-installation)
  - [Uninstall](#uninstall)
- [Next steps](#next-steps)

## Requirements

### Infrastructure

| Component | Minimum version |
| --------- | --------------- |
| XCP-ng | 8.3+ |
| Xen Orchestra | 6.4+ |
| Kubernetes | 1.26+ |

Network connectivity is required between the CSI controller pod and the Xen
Orchestra API endpoint.

### Kubernetes tooling

- `kubectl` configured with access to the target cluster.
- Sufficient RBAC permissions to create resources in the `kube-system` namespace.
- **Helm 3+** if you install with the [Helm chart](install-helm.md).

### XenOrchestra Cloud Controller Manager (CCM)

The CCM is **required** for the CSI node plugin. The CCM sets `spec.providerID`
on each Kubernetes Node object. The CSI node plugin reads this field at startup
to resolve the pool ID and VM UUID, then returns them from `NodeGetInfo` so
that kubelet can register the driver and write the
`topology.k8s.xenorchestra/pool_id` label.

**Without the CCM**, `spec.providerID` is empty. `NodeGetInfo` fails with a
`codes.Internal` error and the node-driver-registrar enters
**CrashLoopBackOff**. The CSI node plugin is never registered with kubelet on
that node: no volume can be staged, published, or unpublished. The
`topology.k8s.xenorchestra/pool_id` label is never written to the Node object.

The CSI controller reuses the same credentials secret as the CCM. If the CCM is
already installed you can skip the
[credentials step](#create-the-credentials-secret) below. The node plugin does
not mount or read this secret.

See the
[CCM install guide](https://github.com/vatesfr/xenorchestra-cloud-controller-manager/blob/main/docs/install.md)
for setup instructions, and [docs/topology.md](topology.md) for a detailed
explanation of how topology works in this driver.

## Choose your installation method

| Method | Document | When to use |
| ------ | -------- | ----------- |
| **Helm chart** | [install-helm.md](install-helm.md) | Recommended. Install from the OCI registry (`ghcr.io`) or a local checkout. Supports component toggles, the Helm test, and easy upgrades. |
| **Static deployment files** | [install-static.md](install-static.md) | Manage the manifests directly with `kubectl`. Pre-rendered files live in [`docs/deploy/`](deploy/). |

Both methods share the same prerequisites and credentials secret described
below, then differ only in how the driver pods are created.

## Shared steps

### Create the credentials secret

The controller authenticates to Xen Orchestra using a YAML config file stored
as a Kubernetes secret.

Create a file named `xo-config.yaml`:

```yaml
url: https://<xen-orchestra-host>
insecure: false   # set to true only when using a self-signed certificate
token: "<your-xo-api-token>"
```

> **How to generate an API token in Xen Orchestra:**
> _User Settings → Authentication tokens → New token_

Then create the secret. The name you choose depends on the installation method:

- **Helm**: the secret name is up to you; you pass it to the chart with
  `--set existingConfigSecret=<name>`. If you reuse the CCM's secret, use
  `xenorchestra-cloud-controller-manager`.

  ```bash
  kubectl -n kube-system create secret generic xenorchestra-cloud-controller-manager \
    --from-file=config.yaml=xo-config.yaml
  ```

- **Static deployment files**: the manifests expect the secret to be named
  `xenorchestra-csi-driver`.

  ```bash
  kubectl -n kube-system create secret generic xenorchestra-csi-driver \
    --from-file=config.yaml=xo-config.yaml
  ```

> [!NOTE]
> The secret name `xenorchestra-cloud-controller-manager` is shared with the
> CCM by convention.

#### Alternative: Environment Variable Configuration

Instead of using a config file secret, you can configure the driver using
environment variables. This can be useful for development or testing scenarios.

The driver supports the following environment variables:

| Variable | Description | Required | Default |
| -------- | ----------- | -------- | ------- |
| `XOA_URL` | Xen Orchestra API URL | Yes | — |
| `XOA_TOKEN` | API token for authentication | Yes (if not using username/password) | — |
| `XOA_INSECURE` | Skip TLS verification | No | `false` |

**Example:**

Create a `.env` file (e.g., `xo-config.env`):

```env
# Xen Orchestra configuration
XOA_URL=https://xo.example.com
XOA_TOKEN="your-api-token-here"
XOA_INSECURE=false
```

Then create a secret from the `.env` file:

```bash
# Create secret from .env file
kubectl -n kube-system create secret generic xenorchestra-config \
  --from-env-file=xo-config.env
```

Reference that Secret from your Helm values:

```yaml
driver:
  envFrom:
    - secretRef:
        name: xenorchestra-config
```

> [!IMPORTANT]
> Environment variables take precedence only when the config file
> is not found. The driver first tries to load configuration from the mounted
> config file, and falls back to environment variables if the file is missing
> or invalid.

### Verify the installation

Verify that the pods are running:

```bash
kubectl -n kube-system get pods \
  -l app.kubernetes.io/instance=xenorchestra-csi-driver
```

### Uninstall

- **Helm:**

  ```bash
  helm uninstall xenorchestra-csi-driver --namespace kube-system
  ```

- **Static deployment files:** see the
  [uninstall section](install-static.md#uninstall) of the static guide.

To also remove the credentials secret (if not used by the CCM):

```bash
kubectl -n kube-system delete secret xenorchestra-cloud-controller-manager
```

## Next steps

- [Usage and Examples](usage.md) — choose a provisioning mode, create a
  StorageClass, and attach volumes to workloads.
- [Helm chart values](../charts/xenorchestra-csi-driver/README.md) — the full
  generated values reference for the Helm chart.
- [Topology and Placement](topology.md) — how pool selection and placement work.
- [Local Storage reference](references/local-storage.md) — SR selection, VDI
  migration, idempotency, and VM live-migration behaviour.

## Driver parameters reference

### CSI driver name

```text
csi.xenorchestra.vates.tech
```

### Driver startup flags

These flags are passed as container arguments in the controller/node deployment
manifests.

The driver supports two deployment modes:
- `--mode=controller`: the controller deployment. It exposes Identity,
  Controller, and Node services.
- `--mode=node`: the node deployment. It exposes Identity and Node services
  only, and does not load XO credentials.

// TODO: Missing parameters
| Flag | Description | Default |
| ---- | ----------- | ------- |
| `--mode` | Driver service mode: `controller` or `node` | `controller` |
| `--driver-name` | CSI driver name registered with Kubernetes | `csi.xenorchestra.vates.tech` |
| `--endpoint` | CSI gRPC endpoint | `unix://tmp/csi.sock` |
| `--config-file` | Path to the XO credentials config file mounted in the controller pod | `/etc/xenorchestra/config.yaml` |
| `--vdi-name-prefix` | Prefix prepended to the Kubernetes volume name when labelling VDIs in XO | `csi-` |
| `--cluster-tag` | Tag added to every VDI at creation; `ListVolumes` only returns VDIs carrying this tag. Set to `""` to disable tagging and filtering. | `k8s-managed` |
| `--xo-client-timeout` | HTTP timeout for XenOrchestra API requests. Defaults to `30s`, increase for large pools or slow connections. | `30s` |

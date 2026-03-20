# Installation Guide

This guide walks you through installing the XenOrchestra CSI driver on a Kubernetes cluster.

> **⚠️ Warning**
> This driver is currently under active development.
> It contains unimplemented features. **Do not use in production.**

## Requirements

### Infrastructure

| Component | Minimum version |
| --------- | --------------- |
| XCP-ng | 8.3+ |
| Xen Orchestra | 5.110.1+ |
| Kubernetes | 1.26+ |

Network connectivity is required between every Kubernetes node and the Xen Orchestra API endpoint.

### Kubernetes tooling

- `kubectl` configured with access to the target cluster.
- Sufficient RBAC permissions to create resources in the `kube-system` namespace.

### Optional: XenOrchestra Cloud Controller Manager (CCM)

The CSI driver reuses the same credentials secret as the CCM.
If the CCM is already installed in your cluster the secret is already present and you can skip the
[credentials step](#2-create-the-credentials-secret) below.

See the [CCM install guide](https://github.com/vatesfr/xenorchestra-cloud-controller-manager/blob/main/docs/install.md)
for details.

---

## Step-by-step installation

### 1. Create a registry pull secret (GHCR)

The driver image is hosted on the GitHub Container Registry (`ghcr.io`).
If your cluster cannot pull public images anonymously you need to create a pull secret first.

```bash
kubectl -n kube-system create secret docker-registry regcred \
  --docker-server=ghcr.io \
  --docker-username=<your-github-username> \
  --docker-password=<your-github-token> \
  --docker-email=<your-email>
```

### 2. Create the credentials secret

The driver authenticates to Xen Orchestra using a YAML config file stored as a Kubernetes secret.

Create a file named `xo-config.yaml`:

```yaml
url: https://<xen-orchestra-host>
insecure: false   # set to true only when using a self-signed certificate
token: "<your-xo-api-token>"
```

> **How to generate an API token in Xen Orchestra:**
> _User Settings → Authentication tokens → New token_

Then create the secret:

```bash
kubectl -n kube-system create secret generic xenorchestra-cloud-controller-manager \
  --from-file=config.yaml=xo-config.yaml
```

> ℹ️ The secret name `xenorchestra-cloud-controller-manager` is shared with the CCM by convention.

### 3. Install the driver

Using the installation script (recommended):

```bash
# Install from the latest published manifests
./deploy/install-driver.sh

# Or install from your local clone
./deploy/install-driver.sh local
```

The script applies the following manifests in order:

| Manifest | Purpose |
| -------- | ------- |
| `csi-xenorchestra-driver.yaml` | `CSIDriver` resource |
| `rbac-csi-xenorchestra-node.yaml` | Node plugin RBAC |
| `csi-xenorchestra-node.yaml` | Node plugin `DaemonSet` |
| `rbac-csi-xenorchestra-controller.yaml` | Controller RBAC |
| `csi-xenorchestra-controller.yaml` | Controller `StatefulSet` |

Verify that the pods are running:

```bash
kubectl -n kube-system get pods -l app=csi-xenorchestra-controller
```

### 4. Create a StorageClass

```bash
kubectl apply -f examples/csi-storageclass.yaml
```

```yaml
# examples/csi-storageclass.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-xenorchestra-sc
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: false
```

---

## Static volume provisioning

Dynamic provisioning is not yet implemented.
Volumes must be created manually in Xen Orchestra first, then referenced by UUID.

### 1. Create a VDI in Xen Orchestra

Use the Xen Orchestra GUI, CLI, API to create a Virtual Disk Image (VDI).
Note its UUID (e.g. `b05f63f2-692a-4833-9453-980a73f9f27f`).

### 2. Create a PersistentVolume

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: my-xo-pv
spec:
  storageClassName: csi-xenorchestra-sc
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteOnce
  csi:
    driver: csi.xenorchestra.vates.tech
    volumeHandle: "b05f63f2-692a-4833-9453-980a73f9f27f"  # VDI UUID
```

### 3. Create a PersistentVolumeClaim and use it in a Pod

```bash
kubectl apply -f examples/csi-pvc.yaml
kubectl apply -f examples/csi-app.yaml
```

---

## MicroK8s – kubelet path

When running on MicroK8s, the kubelet socket path differs from a standard installation.
The node plugin manifest must use:

```text
/var/snap/microk8s/common/var/lib/kubelet/
```

instead of the default:

```text
/var/lib/kubelet/
```

Edit `deploy/csi-xenorchestra-node.yaml` accordingly before applying.

---

## Uninstall

```bash
./deploy/uninstall-driver.sh
```

To also remove the credentials secret (if not used by the CCM):

```bash
kubectl -n kube-system delete secret xenorchestra-cloud-controller-manager
```

---

## Driver parameters reference

### CSI driver name

```text
csi.xenorchestra.vates.tech
```

### Supported access modes

| Access mode | Supported |
| ----------- | --------- |
| `ReadWriteOnce` | ✅ |
| `ReadWriteMany` | ❌ (planned) |
| `ReadOnlyMany` | ❌ (planned) |

### Static provisioning – volumeHandle fields

| Field | Description | Required | Example |
| ----- | ----------- | -------- | ------- |
| `volumeHandle` | UUID of the existing VDI | Yes | `b05f63f2-692a-4833-9453-980a73f9f27f` |
| `driver` | Must be `csi.xenorchestra.vates.tech` | Yes | — |

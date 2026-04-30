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

### XenOrchestra Cloud Controller Manager (CCM)

The CCM is **required** when using the CSI driver with the default
`--node-metadata-source=kubernetes` mode. The CCM sets `spec.providerID` on each
Kubernetes Node object. The CSI node plugin reads this field at startup to resolve
the pool ID and VM UUID, then returns them from `NodeGetInfo` so that kubelet can
register the driver and write the `topology.k8s.xenorchestra/pool_id` label.

**Without the CCM**, `spec.providerID` is empty. `NodeGetInfo` fails with a
`codes.Internal` error and the node-driver-registrar enters **CrashLoopBackOff**.
The CSI node plugin is never registered with kubelet on that node: no volume can be
staged, published, or unpublished. The `topology.k8s.xenorchestra/pool_id` label is
never written to the Node object.

If you cannot run the CCM, use `--node-metadata-source=xo-api` instead — the driver
will resolve the pool ID directly from the XenOrchestra API without depending on
`spec.providerID`.

The CSI driver reuses the same credentials secret as the CCM. If the CCM is already
installed you can skip the [credentials step](#2-create-the-credentials-secret)
below.

See the [CCM install guide](https://github.com/vatesfr/xenorchestra-cloud-controller-manager/blob/main/docs/install.md)
for setup instructions, and [docs/topology.md](topology.md) for a detailed
explanation of how topology works in this driver.

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

### Alternative: Environment Variable Configuration

Instead of using a config file secret, you can configure the driver using environment variables.
This can be useful for development or testing scenarios.

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

```yaml
# In your deployment manifest
envFrom:
  - secretRef:
      name: xenorchestra-config
```

> ⚠️ **Note:** Environment variables take precedence only when the config file is not found.
> The driver first tries to load configuration from the mounted config file, and falls back to environment variables if the file is missing or invalid.

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

**Customizing for Environment Variables:**

If you want to use environment variables instead of the config file, modify the controller deployment:

```yaml
# In deploy/csi-xenorchestra-controller.yaml
containers:
  - name: xenorchestra-csi-driver
    # ... existing args ...
    envFrom:
      - secretRef:
          name: xenorchestra-config  # Secret created from .env file
    # Remove or comment out the config-file volume mount
    # volumeMounts:
    #   - name: xenorchestra-config
    #     mountPath: /etc/xenorchestra
```

Verify that the pods are running:

```bash
kubectl -n kube-system get pods -l app=csi-xenorchestra-controller
```

### 4. Create a StorageClass

Choose the provisioning mode that suits your use case.

#### Dynamic provisioning (recommended)

The driver creates a VDI automatically when a PVC is bound.
Two modes are available depending on whether you want to pin provisioning to a
specific pool or let the scheduler decide.

##### Explicit pool

Set `poolId` in the StorageClass parameters. The driver always provisions into
that pool's default SR. The `poolId` is validated against the pod's topology
requirements at provision time — an error is returned if they are incompatible.

```bash
kubectl apply -f examples/csi-sc-dynamic.yaml
```

```yaml
# examples/csi-sc-dynamic.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-xenorchestra-sc-dynamic
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: false
parameters:
  poolId: "<xo-pool-uuid>"   # UUID of the target XO pool
```

> **How to find the pool UUID in Xen Orchestra:**
> In the XO web UI, open the pool and copy the UUID from the URL or the pool detail page.
> Alternatively: `xo-cli xo.getAllObjects filter='{"type":"pool"}' | jq '.[].id'`

##### Topology-aware (no poolId)

Omit `poolId` entirely. The driver selects the pool automatically from the
`accessibility_requirements` passed by the Kubernetes scheduler, following the
CSI spec ordering: **preferred topologies first**, then **requisite topologies**
as fallback. The first pool whose default SR is accessible is used.

This mode requires:
- `volumeBindingMode: WaitForFirstConsumer` — so the scheduler picks a node
  (and therefore a pool topology) before provisioning begins.
- Nodes labelled with `topology.k8s.xenorchestra/pool_id` — set automatically
  by the CCM or the CSI node plugin's `NodeGetInfo`.

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

See [Topology and Placement](topology.md) for a detailed explanation of how pool
selection works in each mode.

#### Static provisioning (pre-existing VDI)

No `poolId` is required. The volume is identified by its VDI UUID in the PV manifest.

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

## Dynamic volume provisioning

The driver creates a VDI in XenOrchestra when Kubernetes binds a PVC to a pod.
The target SR is always the pool's **default SR**. The pool is either specified
explicitly via `poolId` in the StorageClass, or selected automatically from the
`accessibility_requirements` topology hints passed by the scheduler (see
[Topology and Placement](topology.md) for the full selection logic).

### 1. Create the StorageClass

```bash
kubectl apply -f examples/csi-sc-dynamic.yaml
```

Replace `<xo-pool-uuid>` with your actual pool UUID before applying.

### 2. Create a PersistentVolumeClaim

```bash
kubectl apply -f examples/csi-pvc-dynamic.yaml
```

```yaml
# examples/csi-pvc-dynamic.yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: xo-csi-pvc-dynamic
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-xenorchestra-sc-dynamic
```

> **`volumeBindingMode: WaitForFirstConsumer`** defers PVC binding until a pod is scheduled.
> Use `Immediate` if you want volumes to be provisioned as soon as the PVC is created.

### 3. Deploy a pod that uses the PVC

```bash
kubectl apply -f examples/csi-app.yaml
```

---

## Static volume provisioning

Use a VDI that already exists in XenOrchestra.
No `poolId` is required; the volume is bound by its VDI UUID.

### 1. Create a VDI in Xen Orchestra

Use the Xen Orchestra GUI, CLI, or API to create a Virtual Disk Image (VDI).
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

### Dynamic provisioning – StorageClass parameters

| Parameter | Description | Required | Example |
| --------- | ----------- | -------- | ------- |
| `poolId` | UUID of the Xen Orchestra pool. The VDI is created on the pool's default SR. If omitted, the pool is selected automatically from `accessibility_requirements` (topology-aware mode). | No | `aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee` |

### Driver startup flags

These flags are passed as container arguments in the controller/node deployment manifests.

| Flag | Description | Default |
| ---- | ----------- | ------- |
| `--driver-name` | CSI driver name registered with Kubernetes | `csi.xenorchestra.vates.tech` |
| `--endpoint` | CSI gRPC endpoint | `unix://tmp/csi.sock` |
| `--config-file` | Path to the XO credentials config file mounted in the pod | `/etc/xenorchestra/config.yaml` |
| `--vdi-name-prefix` | Prefix prepended to the Kubernetes volume name when labelling VDIs in XO | `csi-` |
| `--cluster-tag` | Tag added to every VDI at creation; `ListVolumes` only returns VDIs carrying this tag. Set to `""` to disable tagging and filtering. | `k8s-managed` |
| `--node-metadata-source` | How the node plugin resolves the pool ID and VM identity: `kubernetes` (reads `spec.providerID`, requires CCM) or `xo-api` (queries XO directly) | `kubernetes` |

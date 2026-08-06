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

Network connectivity is required between the CSI controller pod and the Xen Orchestra API endpoint.

### Kubernetes tooling

- `kubectl` configured with access to the target cluster.
- Sufficient RBAC permissions to create resources in the `kube-system` namespace.

### XenOrchestra Cloud Controller Manager (CCM)

The CCM is **required** for the CSI node plugin. The CCM sets `spec.providerID` on each
Kubernetes Node object. The CSI node plugin reads this field at startup to resolve
the pool ID and VM UUID, then returns them from `NodeGetInfo` so that kubelet can
register the driver and write the `topology.k8s.xenorchestra/pool_id` label.

**Without the CCM**, `spec.providerID` is empty. `NodeGetInfo` fails with a
`codes.Internal` error and the node-driver-registrar enters **CrashLoopBackOff**.
The CSI node plugin is never registered with kubelet on that node: no volume can be
staged, published, or unpublished. The `topology.k8s.xenorchestra/pool_id` label is
never written to the Node object.

The CSI controller reuses the same credentials secret as the CCM. If the CCM is already
installed you can skip the [credentials step](#2-create-the-credentials-secret)
below. The node plugin does not mount or read this secret.

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

The controller authenticates to Xen Orchestra using a YAML config file stored as a Kubernetes secret.

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

Reference that Secret from your Helm values:

```yaml
driver:
  envFrom:
    - secretRef:
        name: xenorchestra-config
```

> ⚠️ **Note:** Environment variables take precedence only when the config file is not found.
> The driver first tries to load configuration from the mounted config file, and falls back to environment variables if the file is missing or invalid.

### 3. Install the driver

Using Helm:

```bash
helm upgrade --install xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  oci://ghcr.io/vatesfr/charts/xenorchestra-csi-driver
```

#### MicroK8s kubelet path

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
provided in `charts/xenorchestra-csi-driver/values.microk8s.yaml`:

```bash
helm upgrade --install xenorchestra-csi-driver \
  ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-cloud-controller-manager \
  --values charts/xenorchestra-csi-driver/values.microk8s.yaml
```

Credentials may instead be passed inline under `config` (the chart creates a
Secret), or through `driver.envFrom`.

### Verify

Verify that the pods are running:

```bash
kubectl -n kube-system get pods \
  -l app.kubernetes.io/instance=xenorchestra-csi-driver
```

### 4. Create a StorageClass

Choose the provisioning mode that suits your use case.

#### Dynamic provisioning (recommended)

The driver creates a VDI automatically when a PVC is bound.
Three modes are available, in order of precedence:
**VAC SR > explicit poolId > topology-aware**.

##### VAC SR selection (Kubernetes ≥ 1.31)

Set `storageRepositoryId` in a `VolumeAttributesClass`. The driver creates the VDI
directly in the specified SR at provision time — no post-creation migration needed.
The SR is validated: it must exist and belong to the pool selected by `poolId` or
topology. If `storageType` is set in the StorageClass, the SR's shared/local type
must match.

The `storageType: local` automatic local-SR override is **not** applied when a VAC
SR is provided — the VDI lands exactly where the VAC points.

```yaml
# VolumeAttributesClass
apiVersion: storage.k8s.io/v1beta1
kind: VolumeAttributesClass
metadata:
  name: csi-xo-specific-sr
driver: csi.xenorchestra.vates.tech
parameters:
  storageRepositoryId: "<sr-uuid>"   # UUID of the target SR
```

```yaml
# PVC referencing the VAC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: xo-csi-pvc-specific-sr
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: csi-xenorchestra-sc-dynamic
  volumeAttributesClassName: csi-xo-specific-sr
```

##### Explicit pool

Set `poolId` in the StorageClass parameters. The driver uses that pool’s default
SR by default. If `storageType: local` is requested, it later selects one of the
pool’s local SRs for the initial placement. The `poolId` is validated against
the pod's topology requirements at provision time — an error is returned if they
are incompatible.

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

##### Local SR (host-pinned)

Set `storageType: local`. The driver creates the VDI on one of the pool local SRs at
provision time, then migrates it to the target host local SR in
`ControllerPublishVolume` before attaching it to the VM.

This mode requires `volumeBindingMode: WaitForFirstConsumer` (so the target node
is known before provisioning) and every host must have at least one
accessible local user-data SR.

```bash
kubectl apply -f examples/csi-app-local-storage.yaml
```

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-xenorchestra-sc-local
provisioner: csi.xenorchestra.vates.tech
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: false
parameters:
  storageType: local
  # poolId: "<xo-pool-uuid>"   # optional; omit for topology-aware mode
```

See [Local Storage reference](references/local-storage.md) for full details on SR
selection, VDI migration, idempotency, and VM live-migration behaviour.

#### Static provisioning (pre-existing VDI)

No `poolId` is required. The volume is identified by its raw VDI UUID in the PV manifest.
This remains supported in v0.4.0: static volumes use `volumeHandle = <VDI UUID>` and
are resolved through a direct VDI UUID fallback when tag-based lookup does not apply.

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

### 5. Test the installation

The optional Helm test provisions a real volume through an existing
StorageClass, mounts it in a pod, then writes and reads a file. The test does
not create a StorageClass: `tests.storageClassName` must refer to one that
already exists and uses this CSI driver.

If you created one of the StorageClasses above, use its name and skip the
creation step below. If no suitable StorageClass exists, create one for the
test. For example, the following StorageClass is pinned to a Xen Orchestra
pool; replace `<xo-pool-uuid>` with the UUID of a pool accessible from the
Kubernetes nodes:

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

---

## Dynamic volume provisioning

The driver creates a VDI in XenOrchestra when Kubernetes binds a PVC to a pod.
Provisioning follows this precedence order: a **SR from a VolumeAttributesClass** (VAC) if provided, otherwise an explicit **poolId from the StorageClass** if provided, otherwise a **topology-aware pool selection** based on the scheduler’s accessibility requirements.
In the first case, the VDI is created directly in the requested SR. In the second case, the driver uses the pool’s default SR by default; if `storageType: local` is requested, it later switches to one of the pool’s local SRs for the initial placement. In the third case, the pool is selected automatically from the topology hints passed by the scheduler, as described in [Topology and Placement](topology.md).

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
No `poolId` is required; the volume is bound by its raw VDI UUID.

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

## Uninstall

```bash
helm uninstall xenorchestra-csi-driver --namespace kube-system
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
| `volumeHandle` | Raw UUID of the existing VDI | Yes | `b05f63f2-692a-4833-9453-980a73f9f27f` |
| `driver` | Must be `csi.xenorchestra.vates.tech` | Yes | — |

### Dynamic provisioning – StorageClass parameters

| Parameter | Description | Required | Example |
| --------- | ----------- | -------- | ------- |
| `poolId` | UUID of the Xen Orchestra pool. The VDI is created on the pool's default SR. If omitted, the pool is selected automatically from `accessibility_requirements` (topology-aware mode). | No | `aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee` |
| `storageType` | Storage placement strategy. `shared` (default): VDI stays on the pool's shared default SR. `local`: VDI is migrated to the target host's local SR in `ControllerPublishVolume`. | No | `local` |

### Dynamic provisioning – VolumeAttributesClass parameters

| Parameter | Description | Required |
| --------- | ----------- | -------- |
| `storageRepositoryId` | UUID of the target Storage Repository. The VDI is created directly in this SR at provision time. The SR must belong to the pool selected by `poolId` or topology. If `storageType` is set in the StorageClass, the SR's shared/local type must match. | Yes |

### Driver startup flags

These flags are passed as container arguments in the controller/node deployment manifests.

The driver supports two deployment modes:
- `--mode=controller`: the controller deployment. It exposes Identity, Controller, and Node services.
- `--mode=node`: the node deployment. It exposes Identity and Node services only, and does not load XO credentials.

| Flag | Description | Default |
| ---- | ----------- | ------- |
| `--mode` | Driver service mode: `controller` or `node` | `controller` |
| `--driver-name` | CSI driver name registered with Kubernetes | `csi.xenorchestra.vates.tech` |
| `--endpoint` | CSI gRPC endpoint | `unix://tmp/csi.sock` |
| `--config-file` | Path to the XO credentials config file mounted in the controller pod | `/etc/xenorchestra/config.yaml` |
| `--vdi-name-prefix` | Prefix prepended to the Kubernetes volume name when labelling VDIs in XO | `csi-` |
| `--cluster-tag` | Tag added to every VDI at creation; `ListVolumes` only returns VDIs carrying this tag. Set to `""` to disable tagging and filtering. | `k8s-managed` |
| `--xo-client-timeout` | HTTP timeout for XenOrchestra API requests. Defaults to `30s`, increase for large pools or slow connections. | `30s` |

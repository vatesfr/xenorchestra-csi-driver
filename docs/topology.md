# Topology and Placement

This document explains how the XenOrchestra CSI driver models cluster topology,
why specific design decisions were made, and what their operational implications
are.

## How CSI topology works in Kubernetes

When the CSI node plugin starts on a node it calls `NodeGetInfo`, which returns an
`AccessibleTopology` map. Kubernetes stores these key/value pairs as labels on the
Node object and uses them to enforce two placement rules:

1. **Volume scheduling** — When a PVC with `volumeBindingMode: WaitForFirstConsumer`
   is created, the scheduler picks a node first, then passes the node's topology to
   `CreateVolume` so the driver can provision the volume in a location that is
   reachable from that node.
2. **PV node affinity** — Static PVs can carry a `nodeAffinity` rule based on
   topology keys. Kubernetes will only schedule pods that mount that PV onto nodes
   whose labels match.

The labels are written by the **CSI node-driver-registrar (NDR)** sidecar. The NDR
includes a collision-detection mechanism: if the value reported by `NodeGetInfo`
differs from a label already present on the Node object the NDR times out and
crash-loops the pod, preventing silent topology drift.

## Topology segments used by this driver

| Label key | Value | Set by |
| --------- | ----- | ------ |
| `topology.k8s.xenorchestra/pool_id` | XenOrchestra pool UUID | CCM or XoClient |

### Why only pool_id

A VDI (Virtual Disk Image) in XenOrchestra lives inside a Storage Repository (SR).
An SR belongs to a pool. XenOrchestra **enforces at the API level** that a VDI can
only be attached to a VM in the same pool — cross-pool attachment is forbidden.

The pool ID is therefore the only segment that maps to a real access boundary.

### Why host_id is intentionally absent

An earlier version of this driver also included `topology.k8s.xenorchestra/host_id`
in `AccessibleTopology`. This segment was removed for the following reasons:

- **Live migration** — A VM can be migrated between hypervisors within the same pool
  transparently and at any time. After migration the node's host label changes, but
  the label that the NDR wrote before migration remains on the Node object. The NDR
  detects the mismatch and crash-loops the CSI node pod, producing:

  ```
  detected topology value collision: driver reported
  "topology.k8s.xenorchestra/host_id":"<new-uuid>"
  but existing label is
  "topology.k8s.xenorchestra/host_id":"<old-uuid>"
  ```

- **Shared SRs** — Most production SR types (NFS, iSCSI, Fibre Channel, Ceph…) are
  shared across all hosts in the pool. Any VM in the pool can attach the VDI
  regardless of which hypervisor it runs on. Constraining scheduling to a specific
  host is not only unnecessary but actively harmful for workload distribution.

- **Local SRs** — For SRs local to a single host (raw `lvm`, `ext`, `btrfs`…) host
  affinity is meaningful. The driver handles this via the `storageType: local`
  StorageClass parameter: in `ControllerPublishVolume` the VDI is migrated to the
  target host's local SR before being attached to the VM. Because migration changes
  the VDI UUID, the stable CSI volume handle stored in the VDI tag
  `k8s:volumeId:<volumeHandle>` is used to re-locate the VDI after VM live-migration.
  See the
  [Local Storage reference](references/local-storage.md) for full details.

## Cross-pool migration restriction

Moving a VM to a **different XenOrchestra pool** (cross-pool migration) is a
disruptive operation that requires:

1. Detaching all persistent volumes from the node.
2. Migrating the VM and its storage.
3. Re-attaching volumes in the new pool — which requires new PVs because the VDIs
   are now in a different pool.

This is not a transparent live event. Kubernetes treats it as a node replacement,
following the same procedure as decommissioning a node. The `pool_id` topology
segment therefore remains stable for the entire lifetime of a Kubernetes node
under normal operating conditions.

## Dependency on the XenOrchestra Cloud Controller Manager (CCM)

### What the CCM does

The [XenOrchestra Cloud Controller Manager](https://github.com/vatesfr/xenorchestra-cloud-controller-manager)
runs in the cluster and continuously reconciles Kubernetes Node objects with the
state in XenOrchestra. Among other things it sets the topology labels:

```
topology.k8s.xenorchestra/pool_id      = <uuid>
topology.k8s.xenorchestra/host_id      = <uuid>
topology.k8s.xenorchestra/pool_name_label = <name>
topology.k8s.xenorchestra/host_name_label = <name>
```

### Is the CCM mandatory?

Yes. The node plugin resolves pool topology and VM identity from the `ProviderID`
set by the CCM on the Node object. If it is absent, `NodeGetInfo` fails and the
node plugin cannot register with kubelet.

When using **`NodeMetadataFromKubernetes`** without the CCM, `spec.providerID` on the
Node object is empty. `GetNodeMetadata()` calls `ParseProviderID("")`, which returns
an error. `NodeGetInfo` propagates it as a `codes.Internal` gRPC error:

```
failed to fetch node metadata: failed to parse provider ID:
has the Xen Orchestra CCM been installed? (foreign providerID or empty "")
```

The node-driver-registrar (NDR) receives this error via `NotifyRegistrationStatus`
and immediately restarts, entering a **CrashLoopBackOff**. Consequences:

1. **The CSI node plugin is never registered with kubelet** on that node.
2. **No `topology.k8s.xenorchestra/pool_id` label is ever written** to the Node object.
3. **No volume can be staged, published, or unpublished** on that node — the kubelet
   has no registered driver to call.

This is more severe than unenforced topology: the entire node plugin is non-functional
until a CCM populates `spec.providerID`.

**Running the CCM alongside the CSI driver is therefore strongly recommended.**
It ensures that the pool topology is always present on Kubernetes nodes,
that volumes are scheduled only on nodes that can access them, and that
diagnostic labels (host, pool names) are kept up to date automatically.

### CCM and live migration

When a VM is live-migrated to another host, the CCM updates the informational
`host_id` and `host_name_label` labels on the Node object. Because these labels are
**not** reported by `NodeGetInfo` they are outside the NDR's collision detection
scope and can be updated freely without affecting the CSI node pod.

This is the correct separation of concerns:
- **NDR / CSI topology** — immutable pool boundary.
- **CCM labels** — live, informational node metadata that can change at any time.

## Pool selection in CreateVolume

Provisioning follows this precedence order: a VAC SR if provided, otherwise an
explicit `poolId` if provided, otherwise topology-aware selection. The VAC does
not bypass pool selection entirely: the driver still validates that the requested
SR belongs to the resolved pool.

### VAC SR selection (highest precedence)

When a `VolumeAttributesClass` with `storageRepositoryId` is attached to a PVC,
the driver creates the VDI directly in the specified SR at provision time.
Pool selection still applies (via `poolId` or topology-aware mode) to validate
that the SR belongs to the correct pool. See [Usage and Examples](usage.md)
for usage details.

### Explicit `poolId` (simple mode)

```yaml
parameters:
  poolId: "<xo-pool-uuid>"
```

The driver uses the given pool directly. Before provisioning it validates that
`poolId` appears in at least one `requisite` topology entry. If the `poolId` is
absent from all requisite topologies the call fails with `InvalidArgument` —
the StorageClass and the pod's node affinity are incompatible and the volume
would never be schedulable.

After validation the driver checks that the pool's default SR is reachable (not
in maintenance mode) before creating the VDI. When `storageType: local` is
requested, the driver later switches to one of the pool's local SRs for the
initial placement.

### Topology-aware mode (no `poolId`)

```yaml
# no parameters block required
```

The driver derives the target pool entirely from `accessibility_requirements`:

1. Iterates the **preferred** topology list in order.
2. For each candidate pool: fetches the pool, checks that a default SR is
   configured, fetches the SR, and checks it is not in maintenance mode.
3. The first viable pool is used.
4. If no preferred pool is viable, repeats steps 2–3 for the **requisite** list.
5. If no requisite pool is viable either, returns `ResourceExhausted`.

This mode requires `volumeBindingMode: WaitForFirstConsumer` so that the
scheduler picks a node (and therefore a pool topology) before provisioning
begins. It also requires nodes to carry the
`topology.k8s.xenorchestra/pool_id` label, which is set by the CCM or the CSI
node plugin's `NodeGetInfo` call.

> **Future improvement:** the driver will additionally verify that the selected
> SR has sufficient free space for the requested volume size before committing
> to it (currently a TODO in `pkg/xenorchestra-csi/topology/pool_selector.go`).

### Summary

| VAC `storageRepositoryId` | SC `poolId` | `accessibility_requirements` | Behaviour |
| ------------------------- | ----------- | ---------------------------- | --------- |
| Set | Any | Any | Create VDI in VAC SR; validate SR is in the correct pool |
| Absent | Set | No | Provision into `poolId` default SR, verify SR accessible |
| Absent | Set | Yes | Validate `poolId` ∈ requisite topologies, then verify SR accessible |
| Absent | Absent | Yes | Select first viable pool from preferred → requisite order |
| Absent | Absent | No | `InvalidArgument` — not enough information to pick a pool |

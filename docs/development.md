# Developer Guide

This guide covers the local development workflow for the XenOrchestra CSI driver.

## Requirements

| Tool | Purpose |
| ---- | ------- |
| Go 1.25+ | Build the driver |
| `make` | Task runner |
| `golangci-lint` | Linting |
| `govulncheck` | Vulnerability scanning |
| `kubectl` | Interact with Kubernetes |
| `devspace` | Hot-reload dev environment |
| `dlv` | Remote debugging (optional) |
| `helm` | Deploy the driver and the dev overrides |

---

## Building

```bash
# Build for the current platform
make build

# Build all supported architectures
make build-all-archs

# Build with debug symbols (disables optimizations)
make build-debug

# Remove build artefacts
make clean
```

The compiled binary is written to `bin/xenorchestra-csi-<arch>`.

---

## Code quality

```bash
# Run the linter
make lint

# Run unit tests
make unit

# Check for known CVEs
make vuln
```

---

## Quick deployment with Helm

Everything is a chart value — there are no hand-maintained manifests to apply.
Create the credentials secret (see [install.md](./install.md#create-the-credentials-secret)), then:

```bash
helm upgrade --install csi-driver ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set existingConfigSecret=xenorchestra-csi-driver

# Remove it again
helm uninstall csi-driver --namespace kube-system
```

Per-environment overrides (image, tags, credentials, ...) are plain helm values:

```bash
helm upgrade --install csi-driver ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set image.repository=localhost:32000/vatesfr/xenorchestra-csi-driver \
  --set image.tag=dev \
  --set image.pullPolicy=Always \
  --set driver.clusterTag=k8s-managed-$USER \
  --set driver.vdiNamePrefix=$USER-csi-
```

For a full set of overrides, combine values files instead of `--set` flags:

```bash
helm template csi-driver ./charts/xenorchestra-csi-driver \
  -f ./charts/xenorchestra-csi-driver/values.edge.yaml \
  -f my-overrides.yaml   # any extra values
```

The `docs/deploy/csi-driver*.yml` files are the same chart rendered with `make docs`
— they exist for the static (no-helm) install path, see [install-static.md](./install-static.md).

---

## DevSpace – hot-reload development

[DevSpace](https://www.devspace.sh/) replaces the driver container with a Go development image and
syncs your local source files into it. No image rebuild is required between iterations.

### Prerequisites

- A running Kubernetes cluster with the driver already deployed (see [install guide](./install.md)).
- DevSpace CLI installed: `brew install devspace` or see [devspace.sh/docs](https://www.devspace.sh/docs).

### Developing the node plugin

The Helm releases are deployed in the namespace DevSpace is pointed at. Before the
first `devspace dev`, select it once (persisted per project):

```bash
devspace use namespace kube-system
```

```bash
devspace dev
```

This command:

1. Applies `hack/dev/csi-xenorchestra-node-single.yaml`: a single-node node
   `Deployment` (edge image, verbose logging) plus its ServiceAccount and RBAC.
   It is a `Deployment` — not a `DaemonSet` — because DevSpace can only take
   over a `Deployment`, `StatefulSet` or `ReplicaSet`. The base install provides
   the `CSIDriver` object and the controller. To pin it to one machine, set
   `nodeSelector.kubernetes.io/hostname` in the manifest.
2. Replaces the driver container image with `golang:1.26.8-trixie`.
3. Syncs your workspace into `/app` inside the container.
4. Opens a bash terminal inside the running container.
5. Exposes port `2345` for remote debugging.

Inside the devspace terminal:

```bash
# Build and run the driver
make build && ./bin/xenorchestra-csi-amd64 --v 5 \
  --node-name $NODE_NAME \
  --endpoint unix:///csi/csi.sock

# Or use the make target
make run
```

> **Tip – avoid re-downloading modules every session**
> DevSpace syncs the full workspace, including the `vendor/` directory.
> Run `go mod vendor` locally before starting `devspace dev` so the vendor tree is
> synced into the container and Go uses it directly instead of hitting the network:
> ```bash
> go mod vendor
> devspace dev
> ```

### Developing the controller

```bash
devspace dev --pipeline dev-controller
```

This renders the chart with `hack/dev/values-controller.yaml`: a controller `Deployment`
(edge image, verbose logging, the `xenorchestra-csi-driver` credentials secret mounted)
with the node `DaemonSet` and the `CSIDriver` object disabled — the base install
provides them. The whole repository is synced into `/app` so you can build the driver
(and the `xo-sdk-go` dependency, if vendored) in place.

Both dev releases are independent Helm releases (`csi-xenorchestra-dev-node` /
`csi-xenorchestra-dev-controller`), so you can switch between them at any time.
Stop a session and its deployment is removed with `devspace purge`.

### SSH access to the dev container

DevSpace injects an SSH server into the dev container.
You can connect your IDE (VS Code Remote SSH, GoLand, etc.) using the hostname configured in
`devspace.yaml`:

```text
# Node plugin
ssh node.devspace

# Controller
ssh controller.devspace
```

---

## MicroK8s built-in registry

MicroK8s ships with a built-in container registry on port `32000`.
It is reachable at `localhost:32000` from the node itself, or at `<node-ip>:32000` from any
other machine (including your development workstation). This avoids the need for an external
registry or image pull secrets during development.

### Enable the registry addon

```bash
microk8s enable registry
```

### Configure Docker for the insecure registry

Add the registry to Docker's list of insecure registries in `/etc/docker/daemon.json`
on every machine that needs to push or pull from it (your workstation and/or the cluster nodes):

```json
{
  "insecure-registries": ["<node-ip>:32000"]
}
```

Then restart Docker: `sudo systemctl restart docker`.

### Build and push from your workstation

```bash
# Tag using the node IP so Kubernetes can pull it
REGISTRY=<node-ip>:32000 VERSION=dev make images
docker push <node-ip>:32000/xenorchestra-csi-driver:dev
```

### Use the local image in the deployment

Pass the local image as chart values (see [Quick deployment with Helm](#quick-deployment-with-helm)):

```bash
helm upgrade --install csi-driver ./charts/xenorchestra-csi-driver \
  --namespace kube-system \
  --set image.repository=localhost:32000/vatesfr/xenorchestra-csi-driver \
  --set image.tag=dev \
  --set image.pullPolicy=Always
```

For DevSpace, the driver image is overridden automatically by the dev container
image, so no manifest change is needed. To run a node against a specific machine
with DevSpace, set `nodeSelector.kubernetes.io/hostname` in
`hack/dev/csi-xenorchestra-node-single.yaml`.

If you use the plain rendered files with `kubectl` and want a pinned image, edit
the `image:` field in the matching `docs/deploy/*.yml` (regenerate with
`make docs` after any chart change).

### MicroK8s kubelet path

MicroK8s uses a non-standard kubelet path. Make sure the node plugin manifest mounts:

```text
/var/snap/microk8s/common/var/lib/kubelet/
```

instead of the standard `/var/lib/kubelet/`.

---

## Remote debugging with Delve

```bash
# Build the debug binary and start Delve in DAP mode (port 2345)
make remote-debug
```

DevSpace forwards port `2345` to your local machine, so you can attach any DAP-compatible debugger
(VS Code, GoLand) to `localhost:2345`.

Example VS Code `launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
      {
        "name": "Connect and launch",
        "type": "go",
        "debugAdapter": "dlv-dap", // the default
        "request": "launch",
        "port": 2345,
        "host": "localhost", // can skip for localhost
        "mode": "exec",
        "program": "/app/bin/xenorchestra-csi-amd64",
        "args": [
            "--v", "5",
            "--node-name", "worker-1",
            "--endpoint=unix:///csi/csi.sock"
        ],
        "substitutePath": [
            { "from": "${workspaceFolder}", "to": "/app" },
        ],
        "showLog": true,
        "trace": "verbose"
      }
  ]
}
```

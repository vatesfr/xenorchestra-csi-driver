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
| `zsh` + `autoenv` | `kxo` shell helper (optional) |

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

## `kxo` – kubectl shorthand

`kxo` is a zsh helper that wraps `kubectl` with short aliases for the project manifests.
It can be loaded automatically when you `cd` into the repository if you have
[`autoenv`](https://github.com/hyperupcall/autoenv) (or `zsh-autoenv`) installed and sourced in your shell.

### Manual loading

```zsh
source hack/kxo.zsh
```

### Available commands

```text
kxo [apply|a|delete|d|get|describe|edit] <manifest-key> [manifest-key...]
kxo create-secret [config-file]
kxo delete-secret
```

Run `kxo` without arguments to see the list of available manifest keys, which are derived
automatically from the files in `deploy/` and `examples/`.

### Examples

```zsh
# Apply the node DaemonSet
kxo apply node

# Delete the controller StatefulSet
kxo delete controller

# Apply multiple manifests at once
kxo a driver node controller

# Create the XO credentials secret from xo-config.yaml (default)
kxo create-secret

# Use a custom config file
kxo create-secret path/to/my-config.yaml

# Delete the XO credentials secret
kxo delete-secret
```

### Manifest key mapping

| Key | File |
| --- | ---- |
| `driver` | `deploy/csi-xenorchestra-driver.yaml` |
| `node` | `deploy/csi-xenorchestra-node.yaml` |
| `node-single` | `deploy/csi-xenorchestra-node-single.yaml` |
| `controller` | `deploy/csi-xenorchestra-controller.yaml` |
| `controller-dev` | `deploy/csi-xenorchestra-controller-dev.yaml` |
| `rbac-node` | `deploy/rbac-csi-xenorchestra-node.yaml` |
| `rbac-controller` | `deploy/rbac-csi-xenorchestra-controller.yaml` |
| *(examples)* | all files in `examples/` without extension |

---

## DevSpace – hot-reload development

[DevSpace](https://www.devspace.sh/) replaces the driver container with a Go development image and
syncs your local source files into it. No image rebuild is required between iterations.

### Prerequisites

- A running Kubernetes cluster with the driver already deployed (see [install guide](./install.md)).
- DevSpace CLI installed: `brew install devspace` or see [devspace.sh/docs](https://www.devspace.sh/docs).

### Developing the node plugin

```bash
devspace dev
```

This command:

1. Deploys `deploy/csi-xenorchestra-node-single.yaml` (single-node variant, no `DaemonSet`).
2. Replaces the container image with `golang:1.26.1-trixie`.
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

The controller pipeline syncs `../xo-sdk-go/` in addition to the driver source, allowing you to
develop the SDK and the driver in tandem without publishing intermediate SDK versions.

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

### Use the local image in the manifests

Update the image field in the relevant `deploy/*.yaml` manifests:

```yaml
image: localhost:32000/xenorchestra-csi-driver:dev
```

Or set the `IMAGE` environment variable to override the driver container image without
editing the manifest file:

```bash
 IMAGE=localhost:32000/vatesfr/xenorchestra-csi-driver:dev kxo apply node
```

`kxo` will substitute any line referencing `xenorchestra-csi-driver` with the provided image
before piping the manifest to `kubectl apply`.

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

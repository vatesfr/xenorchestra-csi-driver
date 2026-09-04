REGISTRY ?= ghcr.io
USERNAME ?= vatesfr
PROJECT ?= xenorchestra-csi-driver
IMAGE ?= $(REGISTRY)/$(USERNAME)/$(PROJECT)
HELMREPO ?= $(REGISTRY)/$(USERNAME)/charts
PLATFORM ?= linux/amd64
PUSH ?= false

VERSION ?= $(shell git describe --dirty --tag --match='v*')
TAG ?= $(VERSION)
GIT_COMMIT ?= $(shell git rev-parse HEAD)

PLUGIN_NAME = xenorchestra-csi
VDI_NAME_PREFIX ?= csi-
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

GO_LDFLAGS := -extldflags "-static" -w -s
GO_LDFLAGS += -X github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi.driverVersion=$(VERSION)
GO_LDFLAGS += -X github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi.gitCommit=$(GIT_COMMIT)
GO_LDFLAGS += -X github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi.buildDate=$(BUILD_DATE)

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
ARCHS = amd64

TESTARGS ?= "-v"

BUILD_ARGS := --platform=$(PLATFORM)
ifeq ($(PUSH),true)
BUILD_ARGS += --push=$(PUSH)
BUILD_ARGS += --output type=image,annotation-index.org.opencontainers.image.source="https://github.com/$(USERNAME)/$(PROJECT)",annotation-index.org.opencontainers.image.description="Xen Orchestra CSI driver for Kubernetes"
else
BUILD_ARGS += --output type=docker
endif

COSING_ARGS ?=

############

# Help Menu

define HELP_MENU_HEADER
# Getting Started

To build this project, you must have the following installed:

- git
- make
- golang 1.20+
- golangci-lint
- git-cliff
- helm
- helm-docs

endef

export HELP_MENU_HEADER

help: ## This help menu.
	@echo "$$HELP_MENU_HEADER"
	@grep -E '^[a-zA-Z0-9%_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

############
#
# Build Abstractions
#

build-all-archs:
	@for arch in $(ARCHS); do $(MAKE) ARCH=$${arch} build ; done

.PHONY: clean
clean: ## Clean
	rm -rf bin

.PHONY: build
build: ## Build
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -ldflags "$(GO_LDFLAGS)" \
		-o bin/${PLUGIN_NAME}-$(ARCH) ./cmd/${PLUGIN_NAME}

.PHONY: build-debug
build-debug: ## Build with debug symbols
	GOOS=$(OS) GOARCH=$(ARCH) go build -gcflags=all="-N -l" -o bin/${PLUGIN_NAME}-$(ARCH) ./cmd/${PLUGIN_NAME}

.PHONY: remote-debug
remote-debug: build-debug ## Build with debug and start Delve in DAP mode
	dlv dap --listen=:2345

.PHONY: lint
lint: ## Lint Code
	golangci-lint run --config .golangci.yml

.PHONY: vuln
vuln: ## Check for known vulnerabilities
	govulncheck ./...

.PHONY: unit
unit: ## Unit Tests
	go test -tags=unit $(shell go list ./...) $(TESTARGS)

.PHONY: run
run: ## Run the application
	go run ./cmd/${PLUGIN_NAME} --v 5 --node-name $(KUBE_NODE_NAME) --endpoint=unix:///csi/csi.sock --vdi-name-prefix=$(VDI_NAME_PREFIX) --cluster-tag=$(CLUSTER_TAGS) --xo-client-timeout=240s

.PHONY: mock
mock: ## Generate mocks
	go generate ./...

############

.PHONY: helm-unit
helm-unit: ## Lint and render the Helm chart
	# Lint the chart
	helm lint charts/xenorchestra-csi-driver
	# Render the chart with the CI values (inline config)
	@helm template --namespace kube-system \
		-f charts/xenorchestra-csi-driver/ci/values.yaml \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver >/dev/null
	# Render the Helm test pod when tests.enabled=true
	helm template --namespace kube-system \
		--set tests.enabled=true \
		--set tests.storageClassName=xo-test \
		--set tests.image=null \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver | \
		grep -q 'helm.sh/hook: test'
	# Fall back to the default service accounts when serviceAccount.create=false
	test "$$(helm template --namespace kube-system \
		--set serviceAccount.create=false \
		--set rbac.create=false \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver | \
		grep -c 'serviceAccountName: default')" -eq 2
	# Skip the controller Deployment and the CSIDriver when disabled
	test "$$(helm template --namespace kube-system \
		--set controller.enabled=false \
		--set csidriver.enabled=false \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver | \
		grep -cE 'kind: (Deployment|CSIDriver)')" -eq 0
	# Skip the node DaemonSet when disabled
	test "$$(helm template --namespace kube-system \
		--set node.enabled=false \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver | \
		grep -cE 'kind: DaemonSet')" -eq 0
	# Skip the ServiceAccount and RBAC of the disabled component
	test "$$(helm template --namespace kube-system \
		--set node.enabled=false \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver | \
		grep -c 'name: xenorchestra-csi-driver-node')" -eq 0
	test "$$(helm template --namespace kube-system \
		--set controller.enabled=false \
		xenorchestra-csi-driver charts/xenorchestra-csi-driver | \
		grep -c 'name: xenorchestra-csi-driver-controller')" -eq 0

.PHONY: helm-login
helm-login: ## Helm Login
	@echo "${HELM_TOKEN}" | helm registry login $(REGISTRY) --username $(USERNAME) --password-stdin

.PHONY: helm-release
helm-release: ## Helm Release
	@rm -rf dist/
	@helm package charts/xenorchestra-csi-driver -d dist
	@helm push dist/xenorchestra-csi-driver-*.tgz oci://$(HELMREPO) 2>&1 | tee dist/.digest
	@cosign sign --yes $(COSING_ARGS) $(HELMREPO)/xenorchestra-csi-driver@$$(cat dist/.digest | awk -F "[, ]+" '/Digest/{print $$NF}')

############

.PHONY: docs
docs: ## Generate documentation for the Helm chart
	yq -i '.appVersion = "$(TAG)"' charts/xenorchestra-csi-driver/Chart.yaml -y
	helm template --namespace kube-system xenorchestra-csi-driver \
		--set existingConfigSecret=xenorchestra-csi-driver \
		charts/xenorchestra-csi-driver > docs/deploy/csi-driver.yml
	helm template --namespace kube-system xenorchestra-csi-driver \
		-f charts/xenorchestra-csi-driver/values.edge.yaml \
		--set existingConfigSecret=xenorchestra-csi-driver \
		charts/xenorchestra-csi-driver > docs/deploy/csi-driver-edge.yml
	helm template --namespace kube-system xenorchestra-csi-driver \
		-f charts/xenorchestra-csi-driver/values.microk8s.yaml \
		--set existingConfigSecret=xenorchestra-csi-driver \
		charts/xenorchestra-csi-driver > docs/deploy/csi-driver-microk8s.yml
	helm-docs --sort-values-order=file charts/xenorchestra-csi-driver

release-update: ## Update the release notes using git-cliff
ifdef RELEASE_TAG
	git-cliff --config cliff.toml --tag $(RELEASE_TAG) --unreleased --prepend CHANGELOG.md
else
	git-cliff --config cliff.toml --unreleased --prepend CHANGELOG.md
endif

############
#
# Docker Abstractions
#

docker-init: ## Initialize the Docker buildx environment for multiarch builds
	@docker run --rm --privileged multiarch/qemu-user-static -p yes ||:

	@docker context create multiarch ||:
	@docker buildx create --name multiarch --driver docker-container --use ||:
	@docker context use multiarch
	@docker buildx inspect --bootstrap multiarch

.PHONY: images
images: ## Build images
	docker buildx build $(BUILD_ARGS) \
		--build-arg VERSION="$(VERSION)" \
		--build-arg TAG="$(TAG)" \
		--build-arg GIT_COMMIT="$(GIT_COMMIT)" \
		-t $(IMAGE):$(TAG) \
		-f Dockerfile .

.PHONY: images-checks
images-checks: images ## Run security checks on the built images
	trivy image --exit-code 1 --ignore-unfixed --severity HIGH,CRITICAL --no-progress $(IMAGE):$(TAG)

.PHONY: images-cosign
images-cosign: ## Sign the built images using Cosign
	@cosign sign --yes $(COSING_ARGS) --recursive $(IMAGE):$(TAG)

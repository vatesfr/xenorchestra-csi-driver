REGISTRY ?= ghcr.io
USERNAME ?= vatesfr
PROJECT ?= xenorchestra-csi-driver
PKG = github.com/vatesfr/xenorchestra-csi-driver
GIT_COMMIT ?= $(shell git rev-parse HEAD)
IMAGE_NAME ?= $(REGISTRY)/$(USERNAME)/xenorchestra-csi
PLUGIN_NAME = xenorchestra-csi
VERSION ?= $(shell git describe --dirty --tag --match='v*')
TAG ?= $(VERSION)

BUILD_DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

GO_LDFLAGS := -extldflags "-static" -w -s
GO_LDFLAGS += -X $(PKG)/pkg/xenorchestra-csi.driverVersion=$(VERSION)
GO_LDFLAGS += -X $(PKG)/pkg/xenorchestra-csi.gitCommit=$(GIT_COMMIT)
GO_LDFLAGS += -X $(PKG)/pkg/xenorchestra-csi.buildDate=$(BUILD_DATE)

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
ARCHS = amd64

PLATFORM ?= linux/amd64
BUILD_ARGS := --platform=$(PLATFORM)
ifeq ($(PUSH),true)
BUILD_ARGS += --push=$(PUSH)
BUILD_ARGS += --output type=image,annotation-index.org.opencontainers.image.source="https://$(PKG)",annotation-index.org.opencontainers.image.description="Xen Orchestra CSI Driver for Kubernetes"
else
BUILD_ARGS += --output type=docker
endif
############

# Help Menu

define HELP_MENU_HEADER
# Getting Started

To build this project, you must have the following installed:

- git
- make
- golang 1.20+
- golangci-lint

endef

export HELP_MENU_HEADER

.PHONY: help
help: ## This help menu.
	@echo "$$HELP_MENU_HEADER"
	@grep -E '^[a-zA-Z0-9%_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

############
#
# Build Abstractions
#
.PHONY: build-all-archs
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
build-debug:
	GOOS=$(OS) GOARCH=$(ARCH) go build -gcflags=all="-N -l" -o bin/${PLUGIN_NAME}-$(ARCH) ./cmd/${PLUGIN_NAME}

.PHONY: remote-debug
remote-debug: build-debug
	dlv dap --listen=:2345

.PHONY: lint
lint: ## Lint Code
	golangci-lint run --config .golangci.yml

release-update:
	git-chglog --config hack/chglog-config.yml -o CHANGELOG.md

.PHONY: unit
unit: ## Unit Tests
	go test -tags=unit $(shell go list ./...) $(TESTARGS)


.PHONY: run
run: ## Run the application
	go run ./cmd/${PLUGIN_NAME} --v 5 --node-name $(KUBE_NODE_NAME) --endpoint=unix:///csi/csi.sock

############
#
# Docker Abstractions
#

docker-init:
	@docker run --rm --privileged multiarch/qemu-user-static -p yes ||:

	@docker context create multiarch ||:
	@docker buildx create --name multiarch --driver docker-container --use ||:
	@docker context use multiarch
	@docker buildx inspect --bootstrap multiarch

.PHONY: images
images: ## Build images
	docker buildx build $(BUILD_ARGS) \
		--build-arg VERSION="$(VERSION)" \
		--build-arg GIT_COMMIT="$(GIT_COMMIT)" \
		-t $(IMAGE_NAME):$(TAG) \
		-f Dockerfile .

.PHONY: images-checks
images-checks: images
	trivy image --exit-code 1 --ignore-unfixed --severity HIGH,CRITICAL --no-progress $(IMAGE):$(TAG)

.PHONY: images-cosign
images-cosign:
	$(eval IMAGE_DIGEST=$(shell docker buildx imagetools inspect $(IMAGE_NAME):$(TAG) --format '{{json .}}' | jq -r '.manifest.digest'))
	@echo "Image digest: $(IMAGE_DIGEST)"
	@echo "Signing: $(IMAGE_NAME)@$(IMAGE_DIGEST)"
	@cosign sign --yes $(COSING_ARGS) $(IMAGE_NAME)@$(IMAGE_DIGEST)
<a name="v1.0.0-rc.1"></a>
## [v1.0.0-rc.1](https://github.com/vatesfr/xenorchestra-csi-driver/compare/v0.4.0...v1.0.0-rc.1) (2026-08-28)

Welcome to the v1.0.0-rc.1 release of Kubernetes CSI driver for Xen Orchestra!

### Bug Fixes

- **ci:** support prerelease application tags

### Features

- **helm:** align chart with split driver modes
- **helm:** add component toggles
- add helmchart
- restrict Xen Orchestra access to CSI controller
- add SR selection using the VolumeAttributesClass feature in PVC
- move client timeout to driver flag and improve kxo overrides

### Changelog

* bc7d9de chore(release): prepare v1.0.0-rc.1
* 173fc68 fix(ci): support prerelease application tags
* 66017c5 build(deps): bump k8s.io/mount-utils
* 399d286 build(deps): bump google.golang.org/grpc from 1.82.1 to 1.83.0
* a957782 build(deps): bump github/codeql-action from 4.35.5 to 4.37.7
* c17cfc2 build(deps): bump actions/setup-go from 6 to 7
* cdf480d feat(helm): align chart with split driver modes
* d47f3a6 ci: update checkout and docker login actions
* ccbb55e feat(helm): add component toggles
* 440d4b2 test: add Helm hook test support for CSI volume provisioning
* 7ca17e3 feat: add helmchart
* 334fd7c feat: restrict Xen Orchestra access to CSI controller
* db7fd67 doc(example): remove un-accurate comment
* 9d2410a refactor: fix linter error
* 9487d49 tests(sanity): add and fix tests for new feature "VolumeAttributeClass"
* 8846c0b refactor(controller): split CreateVolume into functions
* 4e4a9b7 build(deps): bump dependencies packages versions
* d5e75b3 feat: add SR selection using the VolumeAttributesClass feature in PVC
* 7222c33 feat: move client timeout to driver flag and improve kxo overrides
<a name="v1.0.0-rc.1"></a>
## [v1.0.0-rc.1](https://github.com/vatesfr/xenorchestra-csi-driver/compare/v0.4.0...v1.0.0-rc.1) (2026-08-28)

Welcome to the v1.0.0-rc.1 release of Kubernetes CSI driver for Xen Orchestra!


### Features

- **helm:** align chart with split driver modes
- **helm:** add component toggles
- add helmchart
- restrict Xen Orchestra access to CSI controller
- add SR selection using the VolumeAttributesClass feature in PVC
- move client timeout to driver flag and improve kxo overrides

### Changelog

* 66017c5 build(deps): bump k8s.io/mount-utils
* 399d286 build(deps): bump google.golang.org/grpc from 1.82.1 to 1.83.0
* a957782 build(deps): bump github/codeql-action from 4.35.5 to 4.37.7
* c17cfc2 build(deps): bump actions/setup-go from 6 to 7
* cdf480d feat(helm): align chart with split driver modes
* d47f3a6 ci: update checkout and docker login actions
* ccbb55e feat(helm): add component toggles
* 440d4b2 test: add Helm hook test support for CSI volume provisioning
* 7ca17e3 feat: add helmchart
* 334fd7c feat: restrict Xen Orchestra access to CSI controller
* db7fd67 doc(example): remove un-accurate comment
* 9d2410a refactor: fix linter error
* 9487d49 tests(sanity): add and fix tests for new feature "VolumeAttributeClass"
* 8846c0b refactor(controller): split CreateVolume into functions
* 4e4a9b7 build(deps): bump dependencies packages versions
* d5e75b3 feat: add SR selection using the VolumeAttributesClass feature in PVC
* 7222c33 feat: move client timeout to driver flag and improve kxo overrides
<a name="v0.0.1"></a>
## v0.0.1 (2026-01-06)

Welcome to the v0.0.1 release of Kubernetes CSI driver for Xen Orchestra!


### Changelog

* a948ba9 ci(build): fix cosign
* 5367a9d ci: add workflows
<a name="v0.1.0"></a>
## [v0.1.0](https://github.com/vatesfr/xenorchestra-csi-driver/compare/v0.0.1...v0.1.0) (2026-03-23)

Welcome to the v0.1.0 release of Kubernetes CSI driver for Xen Orchestra!


### Features

- add support for environment variable configuration and improve error handling

### Changelog

* f721d33 feat: add support for environment variable configuration and improve error handling
* 8fe5c4c build: optimize Dockerfile caching and add deployment strategies
* 1b71cac refactor: cleanup code
* b6944e9 docs: add guides and enhances dev tools
* e76ebbe chore: update CSI sidecar images to latest versions
* 0ec83cc build(deps): bump google.golang.org/grpc from 1.70.0 to 1.78.0
* 5dbb5d3 build(deps): bump actions/checkout from 4 to 6
* 4c2e140 build(deps): bump golangci/golangci-lint-action from 8 to 9
* 77a9a17 build(deps): bump github.com/container-storage-interface/spec
* 6b2f20f build(deps): bump helm/chart-testing-action from 2.7.0 to 2.8.0
* 53030a9 refactor: replace v1 XO SDK calls with v2 SDK (#19)
* c5fb0c2 ci: add missing CodeQL workflow
* fdce2de build: upgrade Golang image to 1.25-alpine
* 55b3658 Refactor/extract shared k8s module (#18)
* 6fbd99a Fixes (#14)
<a name="v0.2.0"></a>
## [v0.2.0](https://github.com/vatesfr/xenorchestra-csi-driver/compare/v0.1.0...v0.2.0) (2026-04-21)

Welcome to the v0.2.0 release of Kubernetes CSI driver for Xen Orchestra!

### Bug Fixes

- fix(node server): detection of already mounted volume was wrong. Tests: add fake mounter implementation and fix sanity test configuration.
- DeleteVolume wrong check of volumeID
- clarify CCM requirements and error handling in documentation and code

### Features

- add VDI other_config at creation to store k8s volume name
- add a "cluster Tag" to tag VDI used by the driver
- add VDI name prefix configuration for volume naming
- implement dynamic provisioning functionality
- improve node metadata handling for CCM integration

### Changelog

* 8c72cab refactor: rename StubMounter in FakeMounter. Cleanup sanity test code after review.
* fc1e942 fix(node server): detection of already mounted volume was wrong. Tests: add fake mounter implementation and fix sanity test configuration.
* d8e7eec chore(devspace): add env variable to customize vdi name prefix
* b06f06e test(sanity): fix controller and sanity tests.
* 089aa0b refactor: mock and stub for sanity tests.
* 17704bb feat: add VDI other_config at creation to store k8s volume name
* f01c964 fix: DeleteVolume wrong check of volumeID
* 6fdb9de test(sanity): fix stub and config to work with Create and Delete volumes
* 458bb66 feat: add a "cluster Tag" to tag VDI used by the driver
* 53c200c feat: add VDI name prefix configuration for volume naming
* 832a330 feat: implement dynamic provisioning functionality
* 5d062b9 test: implement csi sanity check
* e00ff17 refactor(clients): move Mounter, XOClient and NodeMetadataGetter clients into clients package
* bc69023 fix: clarify CCM requirements and error handling in documentation and code
* 6c0d583 feat: improve node metadata handling for CCM integration
<a name="v0.3.0"></a>
## [v0.3.0](https://github.com/vatesfr/xenorchestra-csi-driver/compare/v0.2.0...v0.3.0) (2026-05-11)

Welcome to the v0.3.0 release of Kubernetes CSI driver for Xen Orchestra!

### Bug Fixes

- **sanity:** missing SR mock
- **topology:** use correct pool_id key in buildAccessibleTopology
- **deploy:** disable leader election and add liveness probes to controller sidecars
- **deploy:** add missing csi-provisioner sidecar to controller
- issues due to SR not connected to host and VDB attached with no device name.

### Features

- **controller:** expose SR and pool metadata in PV volume attributes
- **topology:** implement topology-aware pool selection and validation
- implement ValidateVolumeCapabilities and improve capability validation error handling

### Changelog

* ccff578 docs: update XenOrchestra version requirement to 6.4+
* c636883 refactor: normalize VDI other_config key names
* b5790c3 docs: add migration guide and volume identity reference for v0.3.0
* 85c7489 refactor: improve logging for volume operations in ControllerPublishVolume and DeleteVolume
* dadafd8 refactor: introduce stable CSI volume ID and encapsulate VDI operations in XoClient
* edf2f35 build(deps): bump github.com/onsi/ginkgo/v2 from 2.27.2 to 2.28.1
* 5c0bf0b build: update Golang image to 1.26-alpine and update workflow to test build images
* 601e876 build(deps): bump sigstore/cosign-installer from 3.9.2 to 4.1.1
* 14077ae build(deps): bump docker/setup-buildx-action from 3 to 4
* dda32b5 build(deps): bump docker/login-action from 3 to 4
* 61142ee build(deps): bump the k8s-io group across 1 directory with 3 updates
* ae602f4 build(deps): bump k8s.io/klog/v2 from 2.130.1 to 2.140.0
* c6671aa build(deps): bump actions/setup-go from 5 to 6
* 19cbae7 build(deps): bump google.golang.org/grpc from 1.79.1 to 1.80.0
* e372190 build(deps): bump github/codeql-action from 4 to 4.35.2
* 6e41291 refactor(topology): cleanup pool selection logic and add unit tests
* 4e4e4d0 chore(deploy): add cluster tag to allow customization with kxo tool
* 62a45fd chore: update examples
* 1b60253 feat(controller): expose SR and pool metadata in PV volume attributes
* b7b5c96 fix(sanity): missing SR mock
* 541e4d7 feat(topology): implement topology-aware pool selection and validation
* 6abd698 fix(topology): use correct pool_id key in buildAccessibleTopology
* fd35da5 fix(deploy): disable leader election and add liveness probes to controller sidecars
* a15a1a9 fix(deploy): add missing csi-provisioner sidecar to controller
* 6f944af fix: issues due to SR not connected to host and VDB attached with no device name.
* 8a097da refactor: change function name to non-boolean name
* 7af07e7 feat: implement ValidateVolumeCapabilities and improve capability validation error handling
<a name="v0.4.0"></a>
## [v0.4.0](https://github.com/vatesfr/xenorchestra-csi-driver/compare/v0.3.0...v0.4.0) (2026-06-10)

Welcome to the v0.4.0 release of Kubernetes CSI driver for Xen Orchestra!

### Bug Fixes

- lookup for static provisioned volume that use raw VDI UUID as volume handle
- fallback to tag-based pool discovery in CreateVolume

### Features

- add tags recovering after VDI migration and fetch using fallback
- add local storage support to sanity tests with new test cases
- enhance sanity tests with Kubernetes pool tag and cluster tag support
- refactor sanity tests to improve configuration
- update local storage documentation and configuration examples
- add VDI name_label fallback and update naming convention
- add local storage support

### Changelog

* 471f2ff build(deps): bump github.com/onsi/gomega from 1.39.0 to 1.41.0
* 7d8ca7e build(deps): bump google.golang.org/grpc from 1.80.0 to 1.81.1
* 8bb8575 build(deps): bump the k8s-io group with 3 updates
* 2768327 build(deps): bump azure/setup-helm from 4 to 5
* fa08471 build(deps): bump sigstore/cosign-installer from 4.1.1 to 4.1.2
* 5263496 build(deps): bump github.com/onsi/ginkgo/v2 from 2.28.1 to 2.29.0
* 32f7378 build(deps): bump github/codeql-action from 4.35.2 to 4.35.5
* ae32593 build(deps): bump actions/checkout from 6 to 6.0.2
* e21503f build(deps): bump docker/login-action from 4 to 4.1.0
* 2d79b6f test: update mocks
* 3d328a0 test: add missing unit tests for tags after VDI migration
* d9ea285 refactor: apply code review
* dd5c1ec doc: update documentation with new tag-based lookup method
* 3b4077f fix: lookup for static provisioned volume that use raw VDI UUID as volume handle
* 00751b4 feat: add tags recovering after VDI migration and fetch using fallback
* e3a262a refactor: replace the deprecated `other_config` field by `tags`
* 3d62ab5 ci: add mock checks in PR workflow
* 06b6aa0 refactor: apply review
* ada707e test: refactor pool selection tests to use t.Run
* c27b355 feat: add local storage support to sanity tests with new test cases
* 3a9d5c4 feat: enhance sanity tests with Kubernetes pool tag and cluster tag support
* c902b21 fix: fallback to tag-based pool discovery in CreateVolume
* 38e990a feat: refactor sanity tests to improve configuration
* 4896855 test: refactor tests to use t.Run
* f091239 feat: update local storage documentation and configuration examples
* 1569913 docs: add VDI lookup reference, v0.3.0→v0.4.0 optional migration guide and version-migrations section to README
* 0c604ee feat: add VDI name_label fallback and update naming convention
* 1153404 feat: add local storage support

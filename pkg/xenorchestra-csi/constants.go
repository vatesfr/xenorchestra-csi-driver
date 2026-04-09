/*
Copyright (c) 2025 Vates

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package xenorchestracsi

const (
	DriverName = "csi.xenorchestra.vates.tech"

	// ParameterPoolID is the mandatory StorageClass parameter that identifies
	// the Xen Orchestra pool to provision the VDI in. The driver uses the
	// pool's DefaultSR as the target storage repository.
	ParameterPoolID = "poolId"

	// DefaultVDINamePrefix is prepended to the Kubernetes volume name when
	// constructing the VDI name label in Xen Orchestra. Override it at
	// driver startup with the --vdi-name-prefix flag.
	DefaultVDINamePrefix = "csi-"

	// DefaultClusterTag is the default tag added to all VDIs created by this driver.
	// It is also used in ListVolumes to filter out VDIs that were not created by the
	// driver (e.g. non-Kubernetes VDIs). Override with --cluster-tag at driver startup.
	// Use a unique value per cluster when running multiple Kubernetes clusters against
	// the same Xen Orchestra instance (e.g. "k8s-prod", "k8s-staging").
	DefaultClusterTag = "k8s-managed"
)

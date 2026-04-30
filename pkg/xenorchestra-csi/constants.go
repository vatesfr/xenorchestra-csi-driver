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
	// Override with --cluster-tag at driver startup.
	// Use a unique value per cluster when running multiple Kubernetes clusters against
	// the same Xen Orchestra instance (e.g. "k8s-prod", "k8s-staging").
	DefaultClusterTag = "k8s-managed"

	// VDIOtherConfigKeyPVName is the key in VDI's other_config map that stores
	// the Kubernetes PersistentVolume name associated with this VDI.
	VDIOtherConfigKeyPVName = "kubernetesPVName"

	// VDIOtherConfigKeyCreatedBy is the key in VDI's other_config map that identifies
	// the component that created the VDI (always set to the driver name).
	VDIOtherConfigKeyCreatedBy = "createdBy"

	// VolumeContextKeySRID is the key in the PV's volumeAttributes (CSI VolumeContext)
	// that stores the UUID of the Xen Orchestra Storage Repository backing the VDI.
	VolumeContextKeySRID = "srId"

	// VolumeContextKeySRName is the key in the PV's volumeAttributes (CSI VolumeContext)
	// that stores the human-readable name of the Xen Orchestra Storage Repository.
	VolumeContextKeySRName = "srName"

	// VolumeContextKeyPoolID is the key in the PV's volumeAttributes (CSI VolumeContext)
	// that stores the UUID of the Xen Orchestra pool the VDI belongs to.
	VolumeContextKeyPoolID = "poolId"

	// VolumeContextKeyPoolName is the key in the PV's volumeAttributes (CSI VolumeContext)
	// that stores the human-readable name of the Xen Orchestra pool.
	VolumeContextKeyPoolName = "poolName"
)

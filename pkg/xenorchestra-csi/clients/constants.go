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
package clients

// tagPrefix is the common prefix for all VDI tags used by this driver.
const tagPrefix = "k8s"

// VDITagKeyVolumeId is the key segment used in the VDI tag that stores the
// CSI volume ID (UUID) generated at CreateVolume time.
// Full tag format: "k8s:volumeId:<uuid>"
const VDITagKeyVolumeId = "volumeId"

// VDITagKeyPVName is the key segment used in the VDI tag that stores the
// Kubernetes PersistentVolume name associated with this VDI.
// Full tag format: "k8s:pvName:<pv-name>"
const VDITagKeyPVName = "pvName"

// VDITagKeyManagedBy is the key segment used in the VDI tag that identifies
// the driver that created and manages this VDI.
// Full tag format: "k8s:managedBy:<driver-name>@<version>"
const VDITagKeyManagedBy = "managedBy"

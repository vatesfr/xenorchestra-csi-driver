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

// VDIOtherConfigKeyVolumeId is the XO VDI other_config key that stores the
// CSI volume ID (UUID) generated at CreateVolume time.
// This UUID is stable across SR migrations (VDI UUID changes, volume ID does not).
const VDIOtherConfigKeyVolumeId = "csi-volume-handle"

// VDIOtherConfigKeyCreatedBy is the key in VDI's other_config map that identifies
// the component that created the VDI (always set to the driver name).
const VDIOtherConfigKeyCreatedBy = "createdBy"

// VDIOtherConfigKeyPVName is the key in VDI's other_config map that stores
// the Kubernetes PersistentVolume name associated with this VDI.
const VDIOtherConfigKeyPVName = "kubernetesPVName"

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

import "fmt"

// BuildVDINameLabel constructs the VDI.name_label for a new volume.
// The format is "<prefix><volumeId>-<volumeName>" (e.g. "csi-12345678-90ab-cdef-pvc-xyz").
// The volumeId is embedded so that GetVDIByVolumeId can fall back to searching
// by name_label when other_config has been erased.
func BuildVDINameLabel(prefix, volumeId, volumeName string) string {
	return fmt.Sprintf("%s%s-%s", prefix, volumeId, volumeName)
}

// BuildVDINameDescription constructs the VDI.name_description for a new volume.
// It appends "; pv-name=<volumeName>" to the standard description so operators
// can identify the backing Kubernetes PV in the Xen Orchestra UI.
// This field is not used for lookups.
func BuildVDINameDescription(volumeName string) string {
	return "VDI managed by the Kubernetes CSI; pv-name=" + volumeName
}

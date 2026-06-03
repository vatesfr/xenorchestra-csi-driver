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

import (
	"errors"
	"strings"
)

// ErrVBDNotFound is returned when no VBD matches the given VDI and VM combination.
var ErrVBDNotFound = errors.New("VBD not found")

// ErrVolumeNotFound is returned when no VDI matches the given volume handle.
var ErrVolumeNotFound = errors.New("volume not found")

// ErrVolumeIdAmbiguous is returned when multiple VDIs match the same CSI volume ID.
var ErrVolumeIdAmbiguous = errors.New("multiple VDIs match volume ID")

// ErrVolumeNameAmbiguous is returned when multiple VDIs match the same Kubernetes PV name.
var ErrVolumeNameAmbiguous = errors.New("multiple VDIs match volume name")

// IsNotFoundError reports whether err is an HTTP 404 from the Xen Orchestra REST
func IsNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "API error: 404 Not Found")
}

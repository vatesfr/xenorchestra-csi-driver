// Copyright 2025 Marc Siegenthaler
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xenorchestracsi

import (
	"github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/klog/v2"
)

const (
	DefaultFsType = "ext4"
)

func isValidVolumeCapabilities(v []*csi.VolumeCapability) bool {
	if len(v) == 0 {
		return false
	}

	for _, c := range v {
		if !isValidCapability(c) {
			return false
		}
	}
	return true
}

func isValidCapability(c *csi.VolumeCapability) bool {
	if c == nil {
		return false
	}

	switch c.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		klog.V(2).InfoS("isValidCapability: block access type is not supported")
		return false
	case *csi.VolumeCapability_Mount:
		// Continue
	default:
		klog.V(2).InfoS("isValidCapability: unknown access type", "accessType", c.GetAccessType())
		return false
	}

	accessMode := c.GetAccessMode().GetMode()
	switch accessMode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
		return true
	default:
		klog.V(2).InfoS("isValidCapability: access mode is not supported", "accessMode", accessMode)
		return false
	}
}

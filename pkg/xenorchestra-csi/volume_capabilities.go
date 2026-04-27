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
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	DefaultFsType = "ext4"
)

func validateVolumeCapabilities(v []*csi.VolumeCapability) error {
	if len(v) == 0 {
		return fmt.Errorf("at least one volume capability is required")
	}

	for _, c := range v {
		if err := validateVolumeCapability(c); err != nil {
			return err
		}
	}
	return nil
}

func validateVolumeCapability(c *csi.VolumeCapability) error {
	if c == nil {
		return fmt.Errorf("nil volume capability")
	}

	switch c.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		return fmt.Errorf("block access type is not supported")
	case *csi.VolumeCapability_Mount:
		// Continue
	default:
		return fmt.Errorf("unknown access type %T", c.GetAccessType())
	}

	accessMode := c.GetAccessMode().GetMode()
	switch accessMode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
		return nil
	default:
		return fmt.Errorf("access mode %s is not supported", accessMode)
	}
}

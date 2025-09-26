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

import (
	"bytes"
	"fmt"
	"os"

	xoccm "github.com/vatesfr/xenorchestra-cloud-controller-manager/pkg/xenorchestra"
)

// LoadXOConfigFromFile loads the XO configuration from the mounted secret file
// using the same format as the CCM's readCloudConfig function
func LoadXOConfigFromFile(configFile string) (xoccm.XoConfig, error) {
	// Check if file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return xoccm.XoConfig{}, fmt.Errorf("config file %s does not exist", configFile)
	}

	// Read file content
	content, err := os.ReadFile(configFile)
	if err != nil {
		return xoccm.XoConfig{}, fmt.Errorf("failed to read config file %s: %v", configFile, err)
	}
	return xoccm.ReadCloudConfig(bytes.NewReader(content))
}

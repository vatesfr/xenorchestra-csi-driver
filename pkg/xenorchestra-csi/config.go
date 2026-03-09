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
	"fmt"
	"os"

	xok8s "github.com/vatesfr/xenorchestra-k8s-common"
)

// LoadXOConfigFromFile loads the XO configuration from the mounted secret file.
func LoadXOConfigFromFile(configFile string) (xok8s.XoConfig, error) {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return xok8s.XoConfig{}, fmt.Errorf("config file %s does not exist", configFile)
	}
	return xok8s.ReadCloudConfigFromFile(configFile)
}

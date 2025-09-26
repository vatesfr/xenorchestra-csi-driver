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

import "fmt"

var (
	driverVersion = "dev"
	gitCommit     = "none"
	buildDate     = "unknown"
)

// GetVersion returns the driver version
func GetVersion() string {
	return driverVersion
}

// GetGitCommit returns the git commit hash
func GetGitCommit() string {
	return gitCommit
}

// GetBuildDate returns the build date
func GetBuildDate() string {
	return buildDate
}

// GetVersionInfo returns complete version information
func GetVersionInfo() string {
	return fmt.Sprintf("Version: %s, GitCommit: %s, BuildDate: %s", driverVersion, gitCommit, buildDate)
}

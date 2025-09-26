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
	"flag"
)

// DriverOptions defines driver parameters specified in driver deployment
type DriverOptions struct {
	// Common options
	NodeName   string
	DriverName string
	Endpoint   string
	// XO Configuration
	ConfigFile string
}

func (o *DriverOptions) AddFlags() *flag.FlagSet {
	if o == nil {
		return nil
	}
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.StringVar(&o.NodeName, "node-name", "", "Node name")
	fs.StringVar(&o.DriverName, "driver-name", DriverName, "Driver name")
	fs.StringVar(&o.Endpoint, "endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	fs.StringVar(&o.ConfigFile, "config-file", "/etc/xenorchestra/config.yaml", "Path to XO configuration file")
	return fs
}

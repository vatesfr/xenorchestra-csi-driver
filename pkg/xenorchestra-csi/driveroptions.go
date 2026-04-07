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
	"fmt"
)

// NodeMetadataSource controls how the CSI node plugin resolves pool ID and VM
// identity at startup. See docs/topology.md for details.
type NodeMetadataSource string

const (
	// NodeMetadataSourceKubernetes reads the pool ID from the node label set by the
	// XenOrchestra CCM (topology.k8s.xenorchestra/pool_id) and the VM UUID from the
	// node's ProviderID. This is the recommended mode when the CCM is installed.
	NodeMetadataSourceKubernetes NodeMetadataSource = "kubernetes"

	// NodeMetadataSourceXoAPI queries the XenOrchestra API directly at startup to
	// resolve the pool ID and VM UUID. Use this mode when the CCM is not installed.
	NodeMetadataSourceXoAPI NodeMetadataSource = "xo-api"
)

// DriverOptions defines driver parameters specified in driver deployment
type DriverOptions struct {
	// Common options
	NodeName   string
	DriverName string
	Endpoint   string
	// XO Configuration
	ConfigFile string
	// NodeMetadataSource selects how the node plugin resolves pool ID and VM identity.
	NodeMetadataSource NodeMetadataSource
}

func (o *DriverOptions) AddFlags() *flag.FlagSet {
	if o == nil {
		return nil
	}
	fs := flag.NewFlagSet("", flag.ExitOnError)
	// Set default before registering the flag so the field is populated even if
	// the flag is never passed on the command line.
	o.NodeMetadataSource = NodeMetadataSourceKubernetes
	fs.StringVar(&o.NodeName, "node-name", "", "Node name")
	fs.StringVar(&o.DriverName, "driver-name", DriverName, "Driver name")
	fs.StringVar(&o.Endpoint, "endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	fs.StringVar(&o.ConfigFile, "config-file", "/etc/xenorchestra/config.yaml", "Path to XO configuration file")
	fs.Func("node-metadata-source",
		`Source used by the node plugin to resolve pool ID and VM identity.
Allowed values:
  kubernetes  (default) Read pool ID from the node label set by the XenOrchestra CCM.
              Requires the CCM to be installed and running in the cluster.
  xo-api      Query the XenOrchestra API directly at startup.
              Use this mode when the CCM is not installed.`,
		func(v string) error {
			src := NodeMetadataSource(v)
			switch src {
			case NodeMetadataSourceKubernetes, NodeMetadataSourceXoAPI:
				o.NodeMetadataSource = src
				return nil
			default:
				return fmt.Errorf("invalid node-metadata-source %q: must be %q or %q",
					v, NodeMetadataSourceKubernetes, NodeMetadataSourceXoAPI)
			}
		},
	)
	return fs
}

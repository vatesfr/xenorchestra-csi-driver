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
	"time"
)

// DriverMode controls which CSI services the process exposes.
type DriverMode string

const (
	DriverModeController DriverMode = "controller"
	DriverModeNode       DriverMode = "node"
)

// DriverOptions defines driver parameters specified in driver deployment
type DriverOptions struct {
	// Common options
	Mode       DriverMode
	NodeName   string
	DriverName string
	Endpoint   string
	// XO Configuration
	ConfigFile string
	// VDINamePrefix is prepended to the VDI name label in Xen Orchestra.
	// See xenorchestracsi.BuildVDINameLabel for the full format.
	VDINamePrefix string
	// ClusterTag is added to every VDI created by this driver and used to filter
	// VDIs. Use a unique value per cluster when running multiple clusters against the same
	// XO instance. Defaults to DefaultClusterTag ("k8s-managed"). Set to "" to disable
	// tagging and filtering entirely.
	ClusterTag string
	// KubernetesPoolTag identifies Xen Orchestra pools that should be considered for
	// automatic VDI placement when no poolId or topology constraints are provided.
	// Defaults to DefaultKubernetesPoolTag ("k8s-pool").
	KubernetesPoolTag string
	// XoClientTimeout is the HTTP client timeout for XenOrchestra API requests.
	// Defaults to 30s.
	XoClientTimeout time.Duration
}

func (o *DriverOptions) AddFlags() *flag.FlagSet {
	if o == nil {
		return nil
	}
	fs := flag.NewFlagSet("", flag.ExitOnError)
	// Set default before registering the flag so the field is populated even if
	// the flag is never passed on the command line.
	o.Mode = DriverModeController
	o.VDINamePrefix = DefaultVDINamePrefix
	o.ClusterTag = DefaultClusterTag
	o.KubernetesPoolTag = DefaultKubernetesPoolTag
	fs.Func("mode",
		`CSI service mode exposed by this process.
Allowed values:
	  controller   Expose Identity, Controller, and Node services.
	  node         Expose the Identity and Node services.`,
		func(v string) error {
			mode := DriverMode(v)
			switch mode {
			case DriverModeController, DriverModeNode:
				o.Mode = mode
				return nil
			default:
				return fmt.Errorf("invalid mode %q: must be %q or %q",
					v, DriverModeController, DriverModeNode)
			}
		},
	)
	fs.StringVar(&o.NodeName, "node-name", "", "Node name")
	fs.StringVar(&o.DriverName, "driver-name", DriverName, "Driver name")
	fs.StringVar(&o.Endpoint, "endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	fs.StringVar(&o.ConfigFile, "config-file", "/etc/xenorchestra/config.yaml", "Path to XO configuration file")
	fs.StringVar(&o.VDINamePrefix, "vdi-name-prefix", DefaultVDINamePrefix,
		"Prefix prepended to the Kubernetes volume name when naming VDIs in Xen Orchestra (default: \"csi-\")")
	fs.StringVar(&o.ClusterTag, "cluster-tag", DefaultClusterTag,
		"Tag added to all VDIs created by this driver. "+
			"Use a unique value per cluster when running multiple clusters against the same XO instance. "+
			"Set to \"\" to disable tagging and filtering.")
	fs.StringVar(&o.KubernetesPoolTag, "kubernetes-pool-tag", DefaultKubernetesPoolTag,
		"Tag added to Xen Orchestra pools eligible for automatic volume placement. "+
			"Used when no poolId or topology constraints are provided.")
	fs.DurationVar(&o.XoClientTimeout, "xo-client-timeout", 30*time.Second,
		"HTTP timeout for XenOrchestra API requests (e.g. \"30s\").")
	return fs
}

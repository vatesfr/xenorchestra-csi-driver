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
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
	xok8s "github.com/vatesfr/xenorchestra-k8s-common"

	kube "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type Driver interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer

	Run(ctx context.Context) error
}

type xenorchestraCSIDriver struct {
	Name              string
	NodeID            string
	Version           string
	endpoint          string
	vdiNamePrefix     string
	clusterTag        string
	kubernetesPoolTag string
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
	csi.UnimplementedIdentityServer
	nodeMetadata clients.NodeMetadataGetter
	xoClient     clients.XoClient
	mounter      clients.Mounter
	mode         DriverMode
}

// NewDriverWithDependencies is the internal constructor shared by NewDriver and tests.
func NewDriverWithDependencies(options *DriverOptions, nodeMetadata clients.NodeMetadataGetter, xoClient clients.XoClient, mounter clients.Mounter) Driver {
	if options.Mode == "" {
		options.Mode = DriverModeController
	}
	if options.DriverName == "" {
		klog.Fatal("no driver name provided")
	}
	if options.NodeName == "" {
		klog.Fatal("no node name provided")
	}
	if options.Endpoint == "" {
		klog.Fatal("no driver endpoint provided")
	}
	klog.Infof("Driver: %v ", options.DriverName)
	klog.Infof("Version: %s", driverVersion)
	klog.Infof("VDI name prefix: %q", options.VDINamePrefix)
	klog.Infof("Cluster tag: %q", options.ClusterTag)
	klog.Infof("Kubernetes pool tag: %q", options.KubernetesPoolTag)

	return &xenorchestraCSIDriver{
		Name:              options.DriverName,
		Version:           driverVersion,
		endpoint:          options.Endpoint,
		vdiNamePrefix:     options.VDINamePrefix,
		clusterTag:        options.ClusterTag,
		kubernetesPoolTag: options.KubernetesPoolTag,
		nodeMetadata:      nodeMetadata,
		xoClient:          xoClient,
		mounter:           mounter,
		mode:              options.Mode,
	}
}

func newKubernetesClient() kube.Interface {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("failed to get in-cluster config: %v", err)
	}
	kclient, err := kube.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("failed to create kubernetes client: %v", err)
	}

	return kclient
}

func newXoSDKClient(options *DriverOptions) *xok8s.XoClient {
	// Try to load XO config from mounted file first, then fallback to env
	xoConfig, err := LoadXOConfigFromFile(options.ConfigFile)
	if err != nil {
		klog.Warningf("Failed to load config from file %s: %v, falling back to environment variables", options.ConfigFile, err)
		// Load XO config from environment variables if file loading fails
		xoConfig, err = xok8s.LoadXOConfigFromEnv()
		if err != nil {
			klog.Fatalf("Failed to load config from environment variables: %v. Please ensure either a valid config file is mounted or the required environment variables (XOA_URL and XOA_TOKEN) are set", err)
		}
	}

	// Override the client timeout with the driver flag (always set a default).
	xoConfig.ClientTimeout = options.XoClientTimeout

	xoSDKClient, err := xok8s.NewXOClient(&xoConfig)
	if err != nil {
		klog.Fatalf("failed to create Xen Orchestra client: %v", err)
	}

	return xoSDKClient
}

func NewDriver(options *DriverOptions) Driver {
	kclient := newKubernetesClient()

	var xoClient clients.XoClient
	if options.Mode == DriverModeController {
		xoSDKClient := newXoSDKClient(options)
		xoClient = clients.NewXoClient(xoSDKClient.Client)
	}

	klog.Info("Node metadata source: kubernetes (requires the XenOrchestra CCM)")
	nodeMetadataGetter := clients.NewNodeMetadataFromKubernetes(kclient, options.NodeName)

	return NewDriverWithDependencies(options, nodeMetadataGetter, xoClient, clients.NewSafeMounter())
}

// Run implements Driver.
func (driver *xenorchestraCSIDriver) Run(ctx context.Context) error {
	_ = ctx

	grpc := NewNonBlockingGRPCServer()
	switch driver.mode {
	case DriverModeNode:
		grpc.Start(driver.endpoint, driver, nil, driver)
	default:
		grpc.Start(driver.endpoint, driver, driver, driver)
	}

	return nil
}

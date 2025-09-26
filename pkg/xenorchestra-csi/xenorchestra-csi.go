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

	xoccm "github.com/vatesfr/xenorchestra-cloud-controller-manager/pkg/xenorchestra"

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
	Name     string
	NodeID   string
	Version  string
	endpoint string
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
	csi.UnimplementedIdentityServer
	nodeMetadata NodeMetadataGetter
	xoClient     XoClient
	mounter      Mounter
}

func NewDriver(options *DriverOptions) Driver {
	if options.DriverName == "" {
		klog.Fatal("no driver name provided")
	}

	if options.NodeName == "" {
		klog.Fatal("no node name provided")
	}

	if options.Endpoint == "" {
		klog.Fatal("no driver endpoint provided")
	}

	// Configure Kubernetes client
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	kclient, err := kube.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("failed to create kubernetes client: %v", err)
	}

	// Try to load XO config from mounted file first, then fallback to env
	xoConfig, err := LoadXOConfigFromFile(options.ConfigFile)
	if err != nil {
		klog.Warningf("Failed to load config from file %s: %v, falling back to environment variables", options.ConfigFile, err)
		// TODO: Add fallback to env variables
	}
	xoClient, err := xoccm.NewXOClient(&xoConfig)
	if err != nil {
		klog.Fatalf("failed to create Xen Orchestra client: %v", err)
	}

	// TODO: make it configurable to use the CCM labels or the NodeMetadata (no CCM dependency)
	nodeMetadataGetter := NewNodeMetadataFromXoClient(kclient, xoClient, options.NodeName)

	klog.Infof("Driver: %v ", options.DriverName)
	klog.Infof("Version: %s", driverVersion)

	return &xenorchestraCSIDriver{
		Name:         options.DriverName,
		Version:      driverVersion,
		endpoint:     options.Endpoint,
		nodeMetadata: nodeMetadataGetter,
		xoClient:     NewXoClient(xoClient.Client),
		mounter:      NewSafeMounter(),
	}
}

// Run implements Driver.
func (driver *xenorchestraCSIDriver) Run(ctx context.Context) error {
	// controllerServer := driver.GetController()

	// Start the nonblocking GRPC
	grpc := NewNonBlockingGRPCServer()
	grpc.Start(driver.endpoint, driver, driver, driver)

	return nil
}

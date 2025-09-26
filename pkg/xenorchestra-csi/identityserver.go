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
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/klog/v2"
)

func (driver *xenorchestraCSIDriver) GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	klog.V(5).Infof("Using default capabilities")
	caps := []*csi.PluginCapability{
		{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
				},
			},
		},
	}

	return &csi.GetPluginCapabilitiesResponse{Capabilities: caps}, nil
}

func (driver *xenorchestraCSIDriver) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	klog.V(5).Infof("Using default GetPluginInfo")

	if driver.Name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	if driver.Version == "" {
		return nil, status.Error(codes.Unavailable, "Driver is missing version")
	}

	return &csi.GetPluginInfoResponse{
		Name:          driver.Name,
		VendorVersion: driver.Version,
	}, nil
}

func (driver *xenorchestraCSIDriver) Probe(context.Context, *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: true}}, nil
}

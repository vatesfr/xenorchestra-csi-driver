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
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vatesfr/xenorchestra-cloud-controller-manager/pkg/xenorchestra"

	"k8s.io/klog/v2"
)

// NodeGetCapabilities implements Driver.
func (driver *xenorchestraCSIDriver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(5).InfoS("NodeGetCapabilities called", "request", req)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				// Required capabilities in order to get device path after VDI is mounted on the node
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

// NodeGetInfo implements Driver.
func (driver *xenorchestraCSIDriver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(5).InfoS("NodeGetInfo called", "request", req)
	metadata, err := driver.nodeMetadata.GetNodeMetadata()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch node metadata: %v", err)
	}
	return &csi.NodeGetInfoResponse{
		NodeId: metadata.NodeId,
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				xenorchestra.XOLabelTopologyPoolID: metadata.PoolId,
				xenorchestra.XOLabelTopologyHostID: metadata.HostId,
			},
		},
	}, nil
}

// NodePublishVolume implements Driver.
func (driver *xenorchestraCSIDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(5).InfoS("NodePublishVolume called", "request", req)
	// Check arguments
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}
	if !isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capability")
	}

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "target path missing in request")
	}

	attrib := req.GetVolumeContext()
	sourcePath := req.GetStagingTargetPath()
	if sourcePath == "" {
		if src, exists := attrib["diskMount"]; exists {
			sourcePath = src
		}
	}
	if sourcePath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "source path is required in volume context or staging target path")
	}

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "target path is required")
	}

	mount := volCap.GetMount()
	if mount == nil {
		return nil, status.Error(codes.InvalidArgument, "only mount volume capability is supported")
	}

	// Check if target path exists and create if needed (returns nil if already exists)
	if err := os.MkdirAll(targetPath, 0o777); err != nil {
		klog.ErrorS(err, "failed to create target path", "targetPath", targetPath)
		return nil, status.Errorf(codes.Internal, "failed to create target path: %v", err)
	}

	notMnt, err := driver.mounter.IsMountPoint(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check target path: %v", err)
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	fsType := mount.GetFsType()
	if fsType == "" {
		fsType = DefaultFsType
	}

	deviceId := ""
	if req.GetPublishContext() != nil {
		deviceId = req.GetPublishContext()["deviceID"]
	}

	readOnly := req.GetReadonly()
	volumeId := req.GetVolumeId()
	mountFlags := mount.GetMountFlags()
	options := []string{}
	if readOnly {
		options = append(options, "ro")
	}
	options = append(options, "bind")

	klog.V(5).InfoS("Try to mount device", "source", sourcePath, "target", targetPath, "deviceId", deviceId,
		"fsType", fsType, "options", options, "volumeId", volumeId, "attributes", attrib, "mountFlags", mountFlags)

	if err := driver.mounter.Mount(sourcePath, targetPath, fsType, options); err != nil {
		var errList strings.Builder
		errList.WriteString(err.Error())
		return nil, status.Errorf(codes.Internal, "failed to mount device: %s at %s: %s", sourcePath, targetPath, errList.String())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume implements Driver.
func (driver *xenorchestraCSIDriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(5).Info("NodeUnpublishVolume called", "request", req)

	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()

	err := driver.mounter.Unmount(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target path: %v", err)
	}

	klog.V(2).Infof("volume %s has been unpublished.", targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetVolumeStats implements Driver.
func (driver *xenorchestraCSIDriver) NodeGetVolumeStats(context.Context, *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	klog.Error("NodeGetVolumeStats is not implemented")
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats is not implemented")
}

// NodeExpandVolume implements Driver.
func (driver *xenorchestraCSIDriver) NodeExpandVolume(context.Context, *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	klog.Error("NodeExpandVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not implemented")
}

// NodeStageVolume implements Driver.
func (driver *xenorchestraCSIDriver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	klog.V(5).Info("NodeStageVolume called", "request", req)

	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	stagingTarget := req.GetStagingTargetPath()
	if len(stagingTarget) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target path missing in request")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	mount := volCap.GetMount()
	if mount == nil {
		return nil, status.Error(codes.InvalidArgument, "only mount volume capability is supported")
	}

	fsType := mount.GetFsType()
	if fsType == "" {
		fsType = DefaultFsType
	}

	device := req.GetPublishContext()["device"]
	if device == "" {
		return nil, status.Errorf(codes.InvalidArgument, "device is not set")
	}
	devicePath := "/dev/" + device

	currentDevice, _, err := driver.mounter.GetDeviceNameFromMount(stagingTarget)
	if err != nil {
		klog.ErrorS(err, "failed to check if device is already mounted")
		return nil, status.Errorf(codes.Internal, "failed to check if device is already mounted: %v", err)
	}

	klog.V(4).Info("NodeStageVolume: checking if volume is already staged", "device", device, "currentDevice", currentDevice, "target", stagingTarget)
	if currentDevice == device {
		klog.V(2).Info("NodeStageVolume: volume already staged", "device", device, "target", stagingTarget)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Format device if needed
	klog.V(2).Info("Formatting and mounting device", "devicePath", devicePath, "target", stagingTarget, "fsType", fsType)
	if err := driver.mounter.FormatAndMount(devicePath, stagingTarget, fsType, []string{}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to ensure filesystem: %v", err)
	}

	klog.V(2).Info("NodeStageVolume: successfully staged volume", "devicePath", devicePath, "target", stagingTarget, "fstype", fsType)
	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume implements Driver.
func (driver *xenorchestraCSIDriver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(5).Info("NodeUnstageVolume called", "request", req)
	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	stagingTarget := req.GetStagingTargetPath()
	if len(stagingTarget) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target path missing in request")
	}

	currentDevice, refCount, err := driver.mounter.GetDeviceNameFromMount(stagingTarget)
	if err != nil {
		klog.ErrorS(err, "failed to check if device is already mounted")
		return nil, status.Errorf(codes.Internal, "failed to check if device is already mounted: %v", err)
	}

	if refCount < 1 {
		klog.V(2).Info("NodeUnstageVolume: target is not mounted, nothing to do", "stagingTarget", stagingTarget)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	if refCount > 1 {
		klog.V(2).Info("NodeUnstageVolume: target is still in use, skipping unmount", "stagingTarget", stagingTarget, "refCount", refCount, "currentDevice", currentDevice)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	err = driver.mounter.Unmount(stagingTarget)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unstage device at %s: %v", stagingTarget, err)
	}

	klog.V(4).Info("NodeUnstageVolume: successfully unstaged device", "stagingTarget", stagingTarget)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

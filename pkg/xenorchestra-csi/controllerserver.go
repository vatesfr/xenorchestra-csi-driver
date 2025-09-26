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
	"github.com/gofrs/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	xoV1 "github.com/vatesfr/xenorchestra-go-sdk/client"

	"k8s.io/klog/v2"
)

// ControllerExpandVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	klog.Error("ControllerExpandVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not implemented")
}

// ControllerGetCapabilities implements Driver.
func (driver *xenorchestraCSIDriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).InfoS("ControllerGetCapabilities called", "request", req)

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			// {
			// 	Type: &csi.ControllerServiceCapability_Rpc{
			// 		Rpc: &csi.ControllerServiceCapability_RPC{
			// 			Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			// 		},
			// 	},
			// },
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			// {
			// 	Type: &csi.ControllerServiceCapability_Rpc{
			// 		Rpc: &csi.ControllerServiceCapability_RPC{
			// 			Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
			// 		},
			// 	},
			// },
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_GET_VOLUME,
					},
				},
			},
		},
	}, nil
}

// ControllerGetVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.Error("ControllerGetVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "ControllerGetVolume is not implemented")
}

// ControllerModifyVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerModifyVolume(context.Context, *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	klog.Error("ControllerModifyVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "ControllerModifyVolume is not implemented")
}

// ControllerPublishVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.V(5).InfoS("ControllerPublishVolume called", "request", req)

	vmUUID := req.GetNodeId()
	if vmUUID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "node ID is required")
	}

	if !isValidCapability(req.GetVolumeCapability()) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capability")
	}

	// Volume ID is the VDI UUID
	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	vdi, err := driver.xoClient.GetVDI(ctx, volumeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get VDI: %v", err)
	}

	// Get Node/VM
	nodeVM, err := driver.xoClient.VM().GetByID(ctx, uuid.FromStringOrNil(vmUUID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get VM by ID %s: %v", vmUUID, err)
	}
	if nodeVM.PoolID != uuid.FromStringOrNil(vdi.PoolId) {
		klog.ErrorS(err, "Cannot attach VDI from different pool than the VM", "vdiPool", vdi.PoolId, "vmPool", nodeVM.PoolID)
		return nil, status.Errorf(codes.FailedPrecondition, "cannot attach VDI from pool %s to VM in pool %s", vdi.PoolId, nodeVM.PoolID)
	}

	// Check the VDI is not already attached to another VM
	vbds, err := driver.xoClient.IsVDIUsedAnywhere(ctx, vdi)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check if VDI is already attached: %v", err)
	}

	if len(vbds) > 0 {
		var vbdToAttach *xoV1.VBD
		for _, vbd := range vbds {
			if vbd.Attached && vbd.VmId != vmUUID {
				klog.ErrorS(err, "VDI is already attached to another VM", "vdi", vdi, "vmID", vbd.VmId)
				return nil, status.Errorf(codes.FailedPrecondition, "VDI %s is already attached to another VM %s", vdi.VDIId, vbd.VmId)
			} else if vbd.VmId == vmUUID {
				vbdToAttach = &vbd
				// Continue to check all VDB to be sure the VDI ins't connected to any VM
				continue
			}
		}
		if vbdToAttach != nil {
			// If we found a VBD for this VM, connect it if needed
			// The VDI is already added to the VM
			if !vbdToAttach.Attached {
				klog.V(5).InfoS("Connecting existing VBD to VM", "vbd", *vbdToAttach, "vmUUID", vmUUID)
				vbdConnected, err := driver.xoClient.ConnectVBDToVM(ctx, *vbdToAttach)
				if err != nil {
					klog.ErrorS(err, "Failed to connect VBD to VM", "vbd", *vbdToAttach, "vmUUID", vmUUID)
					return nil, status.Errorf(codes.Internal, "Failed to connect VBD to VM: %v", err)
				}
				// Should be fixed by the addition of Device field in VBD
				return &csi.ControllerPublishVolumeResponse{
					PublishContext: publishContextFromVBD(vbdConnected),
				}, nil
			} else {
				klog.V(2).InfoS("VDI already attached to the node", "vbd", vbdToAttach)
				return &csi.ControllerPublishVolumeResponse{
					PublishContext: publishContextFromVBD(*vbdToAttach),
				}, nil
			}
		} else {
			// Else, it means the VDI is added to a VM (= has VBD) but is not attached (connected) to it
			// We can continue to attach it to the node
			klog.V(5).InfoS("VDI is already added to another VM but not attached to it. Continue to attach it to the node", "vdi", vdi)
		}
	}

	klog.V(5).InfoS("Attaching VDI to VM", "vdi", vdi, "vmUUID", vmUUID)
	vbd, err := driver.xoClient.AttachVDIToVM(ctx, vdi, vmUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to attach VDI to VM", "vdi", vdi, "vmUUID", vmUUID)
		return nil, status.Errorf(codes.Internal, "Failed to attach VDI to VM: %v", err)
	}
	klog.V(5).InfoS("VDI attached to VM", "vmUUID", vmUUID, "vbd", vbd)

	// Return the publish context with the VBD ID and device name
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishContextFromVBD(vbd),
	}, nil
}

// ControllerUnpublishVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(5).InfoS("ControllerUnpublishVolume called", "request", req)

	vmUUID := req.GetNodeId()
	if vmUUID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "node ID is required")
	}

	// Volume ID is the VDI UUID
	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	err := driver.xoClient.DisconnectVBDFromVM(ctx, xoV1.VDI{VDIId: volumeId}, vmUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to detach VDI from VM", "vdiID", volumeId, "vmUUID", vmUUID)
		return nil, status.Errorf(codes.Internal, "Failed to detach VDI from VM: %v", err)
	}
	klog.V(5).InfoS("VBD disconnected from VM", "vdiID", volumeId, "vmUUID", vmUUID)

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// CreateSnapshot implements Driver.
func (driver *xenorchestraCSIDriver) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	klog.Error("CreateSnapshot is not implemented")
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot is not implemented")
}

// CreateVolume implements Driver.
func (driver *xenorchestraCSIDriver) CreateVolume(context.Context, *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.Error("CreateVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "CreateVolume is not implemented")
}

// DeleteSnapshot implements Driver.
func (driver *xenorchestraCSIDriver) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	klog.Error("DeleteSnapshot is not implemented")
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot is not implemented")
}

// DeleteVolume implements Driver.
func (driver *xenorchestraCSIDriver) DeleteVolume(context.Context, *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.Error("DeleteVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "DeleteVolume is not implemented")
}

// GetCapacity implements Driver.
func (driver *xenorchestraCSIDriver) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	klog.Error("GetCapacity is not implemented")
	return nil, status.Error(codes.Unimplemented, "GetCapacity is not implemented")
}

// ListSnapshots implements Driver.
func (driver *xenorchestraCSIDriver) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	klog.Error("ListSnapshots is not implemented")
	return nil, status.Error(codes.Unimplemented, "ListSnapshots is not implemented")
}

// ListVolumes implements Driver.
func (driver *xenorchestraCSIDriver) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.Error("ListVolumes is not implemented")
	return nil, status.Error(codes.Unimplemented, "ListVolumes is not implemented")
}

// ValidateVolumeCapabilities implements Driver.
func (driver *xenorchestraCSIDriver) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.Error("ValidateVolumeCapabilities is not implemented")
	return nil, status.Error(codes.Unimplemented, "ValidateVolumeCapabilities is not implemented")
}

func publishContextFromVBD(vbd xoV1.VBD) map[string]string {
	return map[string]string{
		"device": vbd.Device,
		"vbd":    vbd.Id,
	}
}

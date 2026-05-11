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
	"errors"
	"slices"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gofrs/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/topology"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	xok8s "github.com/vatesfr/xenorchestra-k8s-common"

	"k8s.io/klog/v2"
)

// ControllerExpandVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	klog.Error("ControllerExpandVolume is not implemented")
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not implemented")
}

// ControllerGetCapabilities implements Driver.
func (driver *xenorchestraCSIDriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).Infof("ControllerGetCapabilities called, request: %v", req)

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
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
	klog.V(5).Infof("ControllerPublishVolume called, request: %v", req)

	vmUUID, err := uuid.FromString(req.GetNodeId())
	if err != nil || vmUUID == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "node ID is required")
	}

	if err := validateVolumeCapability(req.GetVolumeCapability()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capability: %v", err)
	}

	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	vdi, err := driver.xoClient.GetVDIByVolumeId(ctx, volumeId)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeId, err)
		}
		return nil, status.Errorf(codes.Internal, "failed to look up volume %s: %v", volumeId, err)
	}

	// Adopt the VDI into this cluster's tag set if the tag is not already present.
	// This ensures static (pre-existing) VDIs are visible without requiring manual
	// re-tagging.
	if driver.clusterTag != "" && !slices.Contains(vdi.Tags, driver.clusterTag) {
		if err := driver.xoClient.VDI().AddTag(ctx, vdi.ID, driver.clusterTag); err != nil {
			klog.ErrorS(err, "Failed to add cluster tag to VDI", "vdiID", vdi.ID, "tag", driver.clusterTag)
			return nil, status.Errorf(codes.Internal, "failed to add cluster tag to VDI %s: %v", vdi.ID, err)
		}
		klog.V(4).InfoS("Added cluster tag to VDI", "vdiID", vdi.ID, "tag", driver.clusterTag)
	}

	// Get Node/VM
	nodeVM, err := driver.xoClient.VM().GetByID(ctx, vmUUID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to get VM by ID %s: %v", vmUUID, err)
	}
	if nodeVM.PoolID != vdi.PoolID {
		klog.ErrorS(err, "Cannot attach a VDI to a VM that belongs to a different pool", "vdiPool", vdi.PoolID, "vmPool", nodeVM.PoolID)
		return nil, status.Errorf(codes.FailedPrecondition, "cannot attach VDI from pool %s to VM in pool %s", vdi.PoolID, nodeVM.PoolID)
	}

	// Verify the SR is reachable from the host where the VM is running before attempting
	// to attach or connect any VBD.
	if err := driver.xoClient.IsSRAttachedToHost(ctx, vdi.SR, nodeVM.Container); err != nil {
		klog.ErrorS(err, "SR is not attached to VM host", "srID", vdi.SR, "hostID", nodeVM.Container, "vmUUID", vmUUID)
		return nil, status.Errorf(codes.FailedPrecondition, "SR is not attached to the VM host: %v", err)
	}

	// Check the VDI is not already attached to another VM
	vbds, err := driver.xoClient.IsVDIUsedAnywhere(ctx, vdi)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check if VDI is already attached: %v", err)
	}

	if len(vbds) > 0 {
		var vbdToAttach *payloads.VBD
		for _, vbd := range vbds {
			if vbd.Attached && vbd.VM != vmUUID {
				klog.ErrorS(err, "VDI is already attached to another VM", "vdi", vdi.ID, "vmID", vbd.VM)
				return nil, status.Errorf(codes.FailedPrecondition, "VDI %s is already attached to another VM %s", vdi.ID, vbd.VM)
			} else if vbd.VM == vmUUID {
				vbdToAttach = vbd
				// Continue to check all VDB to be sure the VDI ins't connected to any VM
				continue
			}
		}
		if vbdToAttach != nil {
			// The VDI is already added to this VM; connect it if not yet hot-plugged.
			if !vbdToAttach.Attached {
				klog.V(5).InfoS("Connecting existing VBD to VM", "vbd", *vbdToAttach, "vmUUID", vmUUID)
				vbdConnected, err := driver.xoClient.ConnectVBDToVM(ctx, *vbdToAttach)
				if err != nil {
					klog.ErrorS(err, "Failed to connect VBD to VM", "vbd", *vbdToAttach, "vmUUID", vmUUID)
					return nil, status.Errorf(codes.Internal, "Failed to connect VBD to VM: %v", err)
				}
				return &csi.ControllerPublishVolumeResponse{
					PublishContext: publishContextFromVBD(*vbdConnected),
				}, nil
			}
			klog.V(2).InfoS("VDI already attached to the node", "vbd", vbdToAttach)
			if vbdToAttach.Device == nil {
				klog.ErrorS(nil, "Device name is not yet assigned to the VBD, waiting...", "vbd", vbdToAttach)
				vbdToAttach, err = driver.xoClient.WaitForVDIToBeFullyAttached(ctx, vbdToAttach.ID)
				if err != nil {
					klog.ErrorS(err, "Failed to wait for VBD to be fully attached", "vbd", vbdToAttach)
					return nil, status.Errorf(codes.Internal, "Failed to wait for VBD to be fully attached: %v", err)
				}
				klog.V(5).InfoS("VBD is now fully attached with device name assigned", "vbd", vbdToAttach)
			}
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: publishContextFromVBD(*vbdToAttach),
			}, nil
		} else {
			// Else, it means the VDI is added to a VM (= has VBD) but is not attached (connected) to it
			// We can continue to attach it to the node
			klog.V(5).InfoS("VDI is already added to another VM but not attached to it. Continue to attach it to the node", "vdi", vdi)
		}
	}

	klog.V(5).InfoS("Attaching VDI to VM", "vdi", vdi, "vmUUID", vmUUID)
	vbd, err := driver.xoClient.AttachVDIToVM(ctx, *vdi, vmUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to attach VDI to VM", "vdi", vdi, "vmUUID", vmUUID)
		return nil, status.Errorf(codes.Internal, "Failed to attach VDI to VM: %v", err)
	}
	klog.V(5).InfoS("VDI attached to VM", "vmUUID", vmUUID, "vbd", vbd)

	// Return the publish context with the VBD ID and device name
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishContextFromVBD(*vbd),
	}, nil
}

// ControllerUnpublishVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(5).Infof("ControllerUnpublishVolume called, request: %v", req)

	vmUUID, err := uuid.FromString(req.GetNodeId())
	if err != nil || vmUUID == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "node ID is required")
	}

	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	vdi, err := driver.xoClient.GetVDIByVolumeId(ctx, volumeId)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			// VDI is already gone; idempotent success.
			klog.V(5).InfoS("VDI not found, treating as already detached", "volumeID", volumeId)
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "failed to look up volume %s: %v", volumeId, err)
	}

	err = driver.xoClient.DisconnectVBDFromVM(ctx, *vdi, vmUUID)
	if err != nil {
		// Ignore not found errors as the VBD may have already been detached
		if !errors.Is(err, clients.ErrVBDNotFound) {
			klog.ErrorS(err, "Failed to detach VDI from VM", "vdiID", vdi.ID, "vmUUID", vmUUID)
			return nil, status.Errorf(codes.Internal, "Failed to detach VDI from VM: %v", err)
		}
		klog.V(5).InfoS("VBD not found, already detached", "vdiID", vdi.ID, "vmUUID", vmUUID)
	}
	klog.V(5).InfoS("VBD disconnected from VM", "vdiID", vdi.ID, "vmUUID", vmUUID)

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// CreateSnapshot implements Driver.
func (driver *xenorchestraCSIDriver) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	klog.Error("CreateSnapshot is not implemented")
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot is not implemented")
}

// CreateVolume implements Driver.
func (driver *xenorchestraCSIDriver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(5).Infof("CreateVolume called, request: %v", req)

	volumeName := req.GetName()
	if volumeName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "disk name is required")
	}

	if req.VolumeContentSource != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume content source is not supported")
	}

	capabilities := req.GetVolumeCapabilities()
	if len(capabilities) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "volume capabilities are required")
	}

	if err := validateVolumeCapabilities(capabilities); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capabilities: %v", err)
	}

	var capacityBytes int64
	if req.GetCapacityRange() != nil {
		capacityBytes = req.GetCapacityRange().GetRequiredBytes()
		if capacityBytes <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "capacity must be greater than 0")
		}
	}

	diskName := driver.vdiNamePrefix + volumeName
	klog.V(5).InfoS("Creating volume", "diskName", diskName, "capacityBytes", capacityBytes)

	// Resolve which pool to provision into.
	//
	// Two cases:
	//   1. StorageClass has an explicit poolId parameter → validate it against
	//      accessibility_requirements requisite topologies if present (a VDI is
	//      only accessible within its pool), then verify its SR is accessible.
	//   2. No poolId parameter → derive the pool from accessibility_requirements
	//      by trying preferred topologies first (in order), then requisite as
	//      fallback, picking the first pool whose SR is accessible.
	//      If neither poolId nor accessibility_requirements are present, error.
	params := req.GetParameters()
	poolIDStr, hasPoolParam := params[ParameterPoolID]
	ar := req.GetAccessibilityRequirements()

	var pool *payloads.Pool
	var sr *payloads.StorageRepository

	if hasPoolParam && poolIDStr != "" {
		// Case 1: explicit poolId in StorageClass.
		poolUUID, err := uuid.FromString(poolIDStr)
		if err != nil || poolUUID == uuid.Nil {
			return nil, status.Errorf(codes.InvalidArgument, "parameter %q must be a valid UUID, got %q", ParameterPoolID, poolIDStr)
		}
		if err := topology.ValidatePoolIDAgainstRequisite(ar, poolUUID); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		pool, sr, err = topology.SelectPoolAndStorage(ctx, driver.xoClient.SR(), driver.xoClient.Pool(), []uuid.UUID{poolUUID})
		if err != nil {
			klog.ErrorS(err, "Pool or SR not viable", "poolID", poolUUID)
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
	} else {
		// Case 2: no poolId — derive from accessibility_requirements.
		// Per the CSI spec, preferred topologies are tried first (in order),
		// then requisite topologies as fallback.
		orderedPoolIDs, err := topology.OrderedPoolIDs(ar)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument,
				"parameter %q is required when accessibility_requirements carry no pool topology: %v",
				ParameterPoolID, err)
		}
		pool, sr, err = topology.SelectPoolAndStorage(ctx, driver.xoClient.SR(), driver.xoClient.Pool(), orderedPoolIDs)
		if err != nil {
			klog.ErrorS(err, "No viable pool found in accessibility_requirements")
			return nil, status.Errorf(codes.ResourceExhausted, "%v", err)
		}
		klog.V(5).InfoS("No poolId parameter, selected pool from accessibility_requirements", "poolID", pool.ID)
	}

	klog.V(5).InfoS("Using pool and SR", "poolID", pool.ID, "srID", sr.ID)

	// Idempotency check: return the existing VDI if one was already created for this PV name.
	existingVDI, existingId, err := driver.xoClient.FindVDIByVolumeName(ctx, volumeName)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			existingVDI = nil
		} else {
			klog.ErrorS(err, "Failed to check for existing VDI", "volumeName", volumeName)
			return nil, status.Errorf(codes.Internal, "failed to check for existing VDI: %v", err)
		}
	}
	if existingVDI != nil {
		if existingVDI.Size != capacityBytes {
			return nil, status.Errorf(codes.AlreadyExists, "volume with name %q already exists with different capacity: existing %d, requested %d", volumeName, existingVDI.Size, capacityBytes)
		}
		// Recover the stable volume ID stored at creation time.
		if existingId == "" {
			return nil, status.Errorf(codes.Internal, "existing VDI %s is missing volume ID in other_config", existingVDI.ID)
		}
		klog.V(5).InfoS("Volume already exists, returning existing VDI", "vdiID", existingVDI.ID, "volumeId", existingId)
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:           existingId,
				CapacityBytes:      capacityBytes,
				AccessibleTopology: driver.buildAccessibleTopology(pool),
				VolumeContext:      buildVolumeContext(pool, sr),
			},
		}, nil
	}

	vdiID, volumeID, err := driver.xoClient.CreateNewVolume(ctx, sr.ID, diskName, capacityBytes, volumeName, DriverName, driver.clusterTag)
	if err != nil {
		klog.ErrorS(err, "Failed to create VDI", "diskName", diskName, "capacityBytes", capacityBytes)
		return nil, status.Errorf(codes.Internal, "Failed to create VDI: %v", err)
	}
	klog.V(5).InfoS("VDI created", "vdiID", vdiID, "volumeID", volumeID, "diskName", diskName)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           volumeID.String(),
			CapacityBytes:      capacityBytes,
			AccessibleTopology: driver.buildAccessibleTopology(pool),
			VolumeContext:      buildVolumeContext(pool, sr),
		},
	}, nil
}

// DeleteSnapshot implements Driver.
func (driver *xenorchestraCSIDriver) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	klog.Error("DeleteSnapshot is not implemented")
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot is not implemented")
}

// DeleteVolume implements Driver.
func (driver *xenorchestraCSIDriver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(5).Infof("DeleteVolume called, request: %v", req)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	vdi, err := driver.xoClient.GetVDIByVolumeId(ctx, volumeID)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			klog.V(5).InfoS("VDI not found, treating as already deleted", "volumeID", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		if errors.Is(err, clients.ErrVolumeIdAmbiguous) {
			klog.ErrorS(err, "Multiple VDIs match volume ID, refusing deletion", "volumeID", volumeID)
			return nil, status.Errorf(codes.Internal, "multiple VDIs match volume ID %s", volumeID)
		}
		klog.ErrorS(err, "Failed to look up volume", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "failed to look up volume %s: %v", volumeID, err)
	}

	// Refuse to delete a VDI that is still attached to a VM.
	vbds, err := driver.xoClient.IsVDIUsedAnywhere(ctx, vdi)
	if err != nil {
		klog.ErrorS(err, "Failed to check VDI attachments", "vdiID", vdi.ID)
		return nil, status.Errorf(codes.Internal, "failed to check VDI attachments for %s: %v", vdi.ID, err)
	}
	for _, vbd := range vbds {
		if vbd.Attached {
			klog.ErrorS(nil, "VDI still attached to a VM, refusing deletion", "vdiID", vdi.ID, "vmID", vbd.VM)
			return nil, status.Errorf(codes.FailedPrecondition, "VDI %s is still attached to VM %s", vdi.ID, vbd.VM)
		}
	}

	if err := driver.xoClient.VDI().Delete(ctx, vdi.ID); err != nil {
		if isNotFoundError(err) {
			// Deleted by a concurrent call between our lookup and Delete
			klog.V(5).InfoS("VDI already deleted by concurrent call", "vdiID", vdi.ID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		klog.ErrorS(err, "Failed to delete VDI", "vdiID", vdi.ID)
		return nil, status.Errorf(codes.Internal, "failed to delete VDI %s: %v", vdi.ID, err)
	}

	klog.V(5).InfoS("VDI deleted successfully", "vdiID", vdi.ID, "volumeID", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
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
func (driver *xenorchestraCSIDriver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.V(5).Info("ValidateVolumeCapabilities called", "request", req)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Volume ID is required")
	}

	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "At least one volume capability is required")
	}

	_, err := driver.xoClient.GetVDIByVolumeId(ctx, volumeID)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			return nil, status.Errorf(codes.NotFound, "Volume %s not found", volumeID)
		}
		klog.ErrorS(err, "Failed to get VDI", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "Failed to get VDI for volume %s: %v", volumeID, err)
	}

	if err := validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{
			Message: err.Error(),
		}, nil
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: req.GetVolumeCapabilities(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

func (driver *xenorchestraCSIDriver) buildAccessibleTopology(pool *payloads.Pool) []*csi.Topology {
	return []*csi.Topology{
		{
			Segments: map[string]string{
				xok8s.XOLabelTopologyPoolID: pool.ID.String(),
			},
		},
	}
}

// buildVolumeContext constructs the CSI VolumeContext map that is stored in the PV's
// volumeAttributes and passed back to ControllerPublishVolume / NodeStageVolume.
func buildVolumeContext(pool *payloads.Pool, sr *payloads.StorageRepository) map[string]string {
	return map[string]string{
		VolumeContextKeySRID:     sr.ID.String(),
		VolumeContextKeySRName:   sr.NameLabel,
		VolumeContextKeyPoolID:   pool.ID.String(),
		VolumeContextKeyPoolName: pool.NameLabel,
	}
}

func publishContextFromVBD(vbd payloads.VBD) map[string]string {
	return map[string]string{
		"device": *vbd.Device,
		"vbd":    vbd.ID.String(),
	}
}

// isNotFoundError reports whether err is an HTTP 404 from the Xen Orchestra REST
// API. The SDK does not expose a dedicated sentinel; errors follow the pattern
// "API error: 404 Not Found - <body>".
func isNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "API error: 404 Not Found")
}

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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gofrs/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_VOLUME_CONDITION,
					},
				},
			},
		},
	}, nil
}

// ControllerGetVolume implements Driver.
func (driver *xenorchestraCSIDriver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.V(5).Infof("ControllerGetVolume called, request: %v", req)

	volumeID, err := uuid.FromString(req.GetVolumeId())
	if err != nil || volumeID == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required and must be a valid UUID")
	}

	vdi, err := driver.xoClient.VDI().Get(ctx, volumeID)
	if err != nil {
		if isNotFoundError(err) {
			// Per KEP-1432: return a condition instead of a gRPC NOT_FOUND error so
			// that the health monitor can surface the event on the PVC.
			return &csi.ControllerGetVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      volumeID.String(),
					CapacityBytes: 0,
				},
				Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
					VolumeCondition: &csi.VolumeCondition{
						Abnormal: true,
						Message:  "VolumeNotFound - the volume is deleted from backend",
					},
				},
			}, nil
		}
		klog.ErrorS(err, "Failed to get VDI", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "failed to get VDI %s: %v", volumeID, err)
	}

	// A VDI with Missing=true has been removed from the storage backend outside
	// of Kubernetes. Per KEP-1432 "VolumeNotFound" use case, report as abnormal.
	if vdi.Missing {
		return &csi.ControllerGetVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vdi.ID.String(),
				CapacityBytes: vdi.Size,
			},
			Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
				VolumeCondition: &csi.VolumeCondition{
					Abnormal: true,
					Message:  "VolumeNotFound - the volume is deleted from backend",
				},
			},
		}, nil
	}

	// Determine volume condition from SR health.
	var condition *csi.VolumeCondition
	sr, srErr := driver.xoClient.SR().Get(ctx, vdi.SR)
	if srErr != nil {
		if isNotFoundError(srErr) {
			condition = volumeConditionFromSR(nil, vdi.SR)
		} else {
			klog.ErrorS(srErr, "Failed to get SR for volume condition", "srID", vdi.SR, "volumeID", volumeID)
			return nil, status.Errorf(codes.Internal, "failed to get SR %s: %v", vdi.SR, srErr)
		}
	} else {
		condition = volumeConditionFromSR(sr, vdi.SR)
	}

	// Fetch published node IDs — "attached?" server-side filter returns only hot-plugged VBDs.
	vbds, err := driver.xoClient.VBD().GetAll(ctx, 0, fmt.Sprintf("VDI:%s attached?", vdi.ID))
	if err != nil {
		klog.ErrorS(err, "Failed to get VBDs for volume", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "failed to get VBDs for VDI %s: %v", volumeID, err)
	}

	publishedNodeIDs := make([]string, 0, len(vbds))
	for _, vbd := range vbds {
		publishedNodeIDs = append(publishedNodeIDs, vbd.VM.String())
	}

	// Check PBD connectivity only when the SR-level condition is healthy and there are published nodes.
	// For each published VM, resolve the host it runs on, then verify the SR has an attached PBD on that host.
	if condition != nil && !condition.Abnormal && len(vbds) > 0 {
		publishedHosts := make([]uuid.UUID, 0, len(vbds))
		hostMap := make(map[uuid.UUID]*payloads.Host, len(vbds))
		for _, vbd := range vbds {
			vm, vmErr := driver.xoClient.VM().GetByID(ctx, vbd.VM)
			if vmErr != nil {
				klog.ErrorS(vmErr, "Failed to get VM for PBD check", "vmID", vbd.VM, "volumeID", volumeID)
				return nil, status.Errorf(codes.Internal, "failed to get VM %s for PBD check: %v", vbd.VM, vmErr)
			}
			publishedHosts = append(publishedHosts, vm.Container)
			if vm.Container != uuid.Nil {
				if _, exists := hostMap[vm.Container]; !exists {
					host, hostErr := driver.xoClient.Host().Get(ctx, vm.Container)
					if hostErr != nil {
						klog.ErrorS(hostErr, "Failed to get host for PBD check", "hostID", vm.Container, "volumeID", volumeID)
						return nil, status.Errorf(codes.Internal, "failed to get host %s for PBD check: %v", vm.Container, hostErr)
					}
					hostMap[vm.Container] = host
				}
			}
		}

		pbds, pbdErr := driver.xoClient.PBD().GetAll(ctx, 0, fmt.Sprintf("SR:%s", vdi.SR))
		if pbdErr != nil {
			klog.ErrorS(pbdErr, "Failed to get PBDs for SR", "srID", vdi.SR, "volumeID", volumeID)
			return nil, status.Errorf(codes.Internal, "failed to get PBDs for SR %s: %v", vdi.SR, pbdErr)
		}

		if pbdCond := volumeConditionFromPBDs(sr, publishedHosts, pbds, hostMap); pbdCond != nil {
			condition = pbdCond
		}
	}

	return &csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vdi.ID.String(),
			CapacityBytes: vdi.Size,
			// TODO: add accessible topology segment for the pool or SR to allow topology-aware scheduling
		},
		Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
			PublishedNodeIds: publishedNodeIDs,
			VolumeCondition:  condition,
		},
	}, nil
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

	if !isValidCapability(req.GetVolumeCapability()) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capability")
	}

	// Volume ID is the VDI UUID
	volumeId, err := uuid.FromString(req.GetVolumeId())
	if err != nil || volumeId == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	vdi, err := driver.xoClient.VDI().Get(ctx, volumeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get VDI: %v", err)
	}

	// Adopt the VDI into this cluster's tag set if the tag is not already present.
	// This ensures static (pre-existing) VDIs are visible in ListVolumes and the
	// health monitor after their first publish, without requiring manual re-tagging.
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
		return nil, status.Errorf(codes.Internal, "failed to get VM by ID %s: %v", vmUUID, err)
	}
	if nodeVM.PoolID != vdi.PoolID {
		klog.ErrorS(err, "Cannot attach a VDI to a VM that belongs to a different pool", "vdiPool", vdi.PoolID, "vmPool", nodeVM.PoolID)
		return nil, status.Errorf(codes.FailedPrecondition, "cannot attach VDI from pool %s to VM in pool %s", vdi.PoolID, nodeVM.PoolID)
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
				// Should be fixed by the addition of Device field in VBD
				return &csi.ControllerPublishVolumeResponse{
					PublishContext: publishContextFromVBD(*vbdConnected),
				}, nil
			}
			klog.V(2).InfoS("VDI already attached to the node", "vbd", vbdToAttach)
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

	// Volume ID is the VDI UUID
	volumeId, err := uuid.FromString(req.GetVolumeId())
	if err != nil || volumeId == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required")
	}

	err = driver.xoClient.DisconnectVBDFromVM(ctx, payloads.VDI{ID: volumeId}, vmUUID)
	if err != nil {
		// Ignore not found errors as the VBD may have already been detached
		if !errors.Is(err, ErrVBDNotFound) {
			klog.ErrorS(err, "Failed to detach VDI from VM", "vdiID", volumeId, "vmUUID", vmUUID)
			return nil, status.Errorf(codes.Internal, "Failed to detach VDI from VM: %v", err)
		}
		klog.V(5).InfoS("VBD not found, already detached", "vdiID", volumeId, "vmUUID", vmUUID)
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
func (driver *xenorchestraCSIDriver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(5).Infof("CreateVolume called, request: %v", req)
	// (
	// 	req=name:"pvc-b6ffb29f-633d-448e-b444-1a1a3faf6f4b"
	// 	capacity_range:{required_bytes:1073741824}
	// 	volume_capabilities:{mount:{fs_type:"ext4"}
	// 	access_mode:{mode:SINGLE_NODE_WRITER}}
	// )

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

	if !isValidVolumeCapabilities(capabilities) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume capabilities")
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

	// Resolve pool from StorageClass parameters.
	params := req.GetParameters()
	poolIDStr, ok := params[ParameterPoolID]
	if !ok || poolIDStr == "" {
		return nil, status.Errorf(codes.InvalidArgument, "parameter %q is required", ParameterPoolID)
	}
	poolUUID, err := uuid.FromString(poolIDStr)
	if err != nil || poolUUID == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "parameter %q must be a valid UUID, got %q", ParameterPoolID, poolIDStr)
	}

	pool, err := driver.xoClient.Pool().Get(ctx, poolUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to get pool", "poolID", poolIDStr)
		return nil, status.Errorf(codes.NotFound, "pool %q not found or inaccessible: %v", poolIDStr, err)
	}
	if pool.DefaultSR == uuid.Nil {
		klog.ErrorS(nil, "Pool has no default SR configured", "poolID", poolIDStr)
		return nil, status.Errorf(codes.FailedPrecondition, "pool %q has no default SR configured", poolIDStr)
	}
	klog.V(5).InfoS("Using pool and SR", "poolID", pool.ID, "srID", pool.DefaultSR)

	vdiParams := payloads.VDICreateParams{
		SRId:            pool.DefaultSR,
		NameLabel:       diskName,
		VirtualSize:     capacityBytes,
		NameDescription: "VDI managed by the Kubernetes CSI",
	}
	if driver.clusterTag != "" {
		vdiParams.Tags = []string{driver.clusterTag}
	}
	vdiID, err := driver.xoClient.VDI().Create(ctx, vdiParams)
	if err != nil {
		klog.ErrorS(err, "Failed to create VDI", "diskName", diskName, "capacityBytes", capacityBytes)
		return nil, status.Errorf(codes.Internal, "Failed to create VDI: %v", err)
	}
	klog.V(5).InfoS("VDI created", "vdiID", vdiID, "diskName", diskName, "capacityBytes", capacityBytes)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vdiID.String(),
			CapacityBytes: capacityBytes,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						xok8s.XOLabelTopologyPoolID: pool.ID.String(),
					},
				},
			},
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

	volumeID, err := uuid.FromString(req.GetVolumeId())
	if err != nil || volumeID == uuid.Nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume ID is required and must be a valid UUID")
	}

	// Check whether the VDI still exists. Return success immediately if it is already gone
	_, err = driver.xoClient.VDI().Get(ctx, volumeID)
	if err != nil {
		if isNotFoundError(err) {
			klog.V(5).InfoS("VDI not found, treating as already deleted", "volumeID", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		klog.ErrorS(err, "Failed to get VDI", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "failed to get VDI %s: %v", volumeID, err)
	}

	// Refuse to delete a VDI that is still attached to a VM.
	vbds, err := driver.xoClient.IsVDIUsedAnywhere(ctx, &payloads.VDI{ID: volumeID})
	if err != nil {
		klog.ErrorS(err, "Failed to check VDI attachments", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "failed to check VDI attachments for %s: %v", volumeID, err)
	}
	for _, vbd := range vbds {
		if vbd.Attached {
			klog.ErrorS(nil, "VDI still attached to a VM, refusing deletion", "volumeID", volumeID, "vmID", vbd.VM)
			return nil, status.Errorf(codes.FailedPrecondition, "volume %s is still attached to VM %s", volumeID, vbd.VM)
		}
	}

	if err := driver.xoClient.VDI().Delete(ctx, volumeID); err != nil {
		if isNotFoundError(err) {
			// Deleted by a concurrent call between our Get and Delete
			klog.V(5).InfoS("VDI already deleted by concurrent call", "volumeID", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		klog.ErrorS(err, "Failed to delete VDI", "volumeID", volumeID)
		return nil, status.Errorf(codes.Internal, "failed to delete VDI %s: %v", volumeID, err)
	}

	klog.V(5).InfoS("VDI deleted successfully", "volumeID", volumeID)
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
func (driver *xenorchestraCSIDriver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.V(5).Infof("ListVolumes called, request: %v", req)

	// Parse and validate the starting token (integer offset into the full VDI list).
	startIndex := 0
	if tok := req.GetStartingToken(); tok != "" {
		idx, err := strconv.Atoi(tok)
		if err != nil || idx < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "invalid starting_token %q: must be a non-negative integer", tok)
		}
		startIndex = idx
	}

	// Fetch all VDIs.
	allVDIs, err := driver.xoClient.VDI().GetAll(ctx, 0, "")
	if err != nil {
		klog.ErrorS(err, "Failed to list VDIs")
		return nil, status.Errorf(codes.Internal, "failed to list VDIs: %v", err)
	}

	// Filter client-side: only include VDIs that carry the cluster tag so that
	// non-Kubernetes VDIs are invisible to the health monitor and volume listings.
	// When clusterTag is "" the filter is disabled (backward-compatible behavior).
	if driver.clusterTag != "" {
		managed := make([]*payloads.VDI, 0, len(allVDIs))
		for _, v := range allVDIs {
			if slices.Contains(v.Tags, driver.clusterTag) {
				managed = append(managed, v)
			}
		}
		allVDIs = managed
	}

	total := len(allVDIs)
	if startIndex > total {
		startIndex = total
	}

	// Apply MaxEntries pagination.
	maxEntries := int(req.GetMaxEntries())
	end := total
	if maxEntries > 0 && startIndex+maxEntries < total {
		end = startIndex + maxEntries
	}

	page := allVDIs[startIndex:end]

	// Fetch all SRs, PBDs and Hosts for volume condition reporting (only when there is at least one VDI to report).
	var srMap map[uuid.UUID]*payloads.StorageRepository
	var pbdsBySR map[uuid.UUID][]*payloads.PBD
	var hostMap map[uuid.UUID]*payloads.Host
	if len(page) > 0 {
		allSRs, err := driver.xoClient.SR().GetAll(ctx, 0, "")
		if err != nil {
			klog.ErrorS(err, "Failed to list SRs for volume condition reporting")
			return nil, status.Errorf(codes.Internal, "failed to list SRs: %v", err)
		}
		srMap = make(map[uuid.UUID]*payloads.StorageRepository, len(allSRs))
		for _, sr := range allSRs {
			srMap[sr.ID] = sr
		}

		allPBDs, err := driver.xoClient.PBD().GetAll(ctx, 0, "")
		if err != nil {
			klog.ErrorS(err, "Failed to list PBDs for volume condition reporting")
			return nil, status.Errorf(codes.Internal, "failed to list PBDs: %v", err)
		}
		pbdsBySR = make(map[uuid.UUID][]*payloads.PBD, len(allSRs))
		for _, pbd := range allPBDs {
			pbdsBySR[pbd.SR] = append(pbdsBySR[pbd.SR], pbd)
		}

		allHosts, err := driver.xoClient.Host().GetAll(ctx, 0, "")
		if err != nil {
			klog.ErrorS(err, "Failed to list hosts for volume condition reporting")
			return nil, status.Errorf(codes.Internal, "failed to list hosts: %v", err)
		}
		hostMap = make(map[uuid.UUID]*payloads.Host, len(allHosts))
		for _, host := range allHosts {
			hostMap[host.ID] = host
		}
	}

	// Build entries, populating published node IDs for each VDI.
	entries := make([]*csi.ListVolumesResponse_Entry, 0, len(page))
	for _, vdi := range page {
		// Fetch attached VBDs for this VDI — "attached?" server-side filter returns only hot-plugged VBDs.
		vbds, err := driver.xoClient.VBD().GetAll(ctx, 0, fmt.Sprintf("VDI:%s attached?", vdi.ID))
		if err != nil {
			klog.ErrorS(err, "Failed to get VBDs for VDI", "vdiID", vdi.ID)
			return nil, status.Errorf(codes.Internal, "failed to get VBDs for VDI %s: %v", vdi.ID, err)
		}

		publishedNodeIDs := make([]string, 0, len(vbds))
		for _, vbd := range vbds {
			publishedNodeIDs = append(publishedNodeIDs, vbd.VM.String())
		}

		// Determine volume condition: start with VDI missing check, then SR health,
		// then layer PBD connectivity.
		var condition *csi.VolumeCondition
		if vdi.Missing {
			// VDI has been removed from the storage backend outside of Kubernetes.
			// Per KEP-1432 "VolumeNotFound" use case, report as abnormal immediately.
			condition = &csi.VolumeCondition{
				Abnormal: true,
				Message:  "VolumeNotFound - the volume is deleted from backend",
			}
		} else {
			condition = volumeConditionFromSR(srMap[vdi.SR], vdi.SR)
			if condition != nil && !condition.Abnormal && len(vbds) > 0 {
				publishedHosts := make([]uuid.UUID, 0, len(vbds))
				for _, vbd := range vbds {
					vm, vmErr := driver.xoClient.VM().GetByID(ctx, vbd.VM)
					if vmErr != nil {
						klog.ErrorS(vmErr, "Failed to get VM for PBD check in ListVolumes", "vmID", vbd.VM, "vdiID", vdi.ID)
						return nil, status.Errorf(codes.Internal, "failed to get VM %s for PBD check: %v", vbd.VM, vmErr)
					}
					publishedHosts = append(publishedHosts, vm.Container)
				}
				if pbdCond := volumeConditionFromPBDs(srMap[vdi.SR], publishedHosts, pbdsBySR[vdi.SR], hostMap); pbdCond != nil {
					condition = pbdCond
				}
			}
		}

		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vdi.ID.String(),
				CapacityBytes: vdi.Size,
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: publishedNodeIDs,
				VolumeCondition:  condition,
			},
		})
	}

	// Compute next token.
	nextToken := ""
	if end < total {
		nextToken = strconv.Itoa(end)
	}

	klog.V(5).InfoS("ListVolumes completed", "total", total, "returned", len(entries), "nextToken", nextToken)
	return &csi.ListVolumesResponse{
		Entries:   entries,
		NextToken: nextToken,
	}, nil
}

// ValidateVolumeCapabilities implements Driver.
func (driver *xenorchestraCSIDriver) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.Error("ValidateVolumeCapabilities is not implemented")
	return nil, status.Error(codes.Unimplemented, "ValidateVolumeCapabilities is not implemented")
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
	return strings.Contains(err.Error(), "404")
}

// srCapacityThreshold is the fraction of SR physical usage at which a volume
// is considered out of capacity. Matches KEP-1432 "OutOfCapacity" use case.
const srCapacityThreshold = 0.95

// volumeConditionFromSR returns a VolumeCondition based on SR health.
// sr is nil when the SR was not found.
// Messages follow the KEP-1432 "Type - detail" format and include both the
// human-readable NameLabel and the UUID for unambiguous identification.
func volumeConditionFromSR(sr *payloads.StorageRepository, srID uuid.UUID) *csi.VolumeCondition {
	if sr == nil {
		return &csi.VolumeCondition{
			Abnormal: true,
			Message:  fmt.Sprintf("VolumeNotFound - SR %s not found", srID),
		}
	}
	if sr.InMaintenanceMode {
		return &csi.VolumeCondition{
			Abnormal: true,
			Message:  fmt.Sprintf("Abnormal - SR '%s' (%s) is in maintenance mode", sr.NameLabel, sr.ID),
		}
	}
	if sr.Size > 0 && sr.PhysicalUsage/sr.Size >= srCapacityThreshold {
		return &csi.VolumeCondition{
			Abnormal: true,
			Message:  fmt.Sprintf("OutOfCapacity - SR '%s' (%s) is out of capacity (%.0f%% used)", sr.NameLabel, sr.ID, sr.PhysicalUsage/sr.Size*100),
		}
	}
	return &csi.VolumeCondition{Abnormal: false}
}

// volumeConditionFromPBDs checks PBD connectivity for all published nodes of a volume.
// pbds is the list of PBDs for the SR (fetched with SR:<srID> filter).
// publishedHosts is the list of host UUIDs for each published VM node (uuid.Nil entries,
// which occur for halted VMs, are skipped).
// hostMap is a map of host UUID to Host object used to include the human-readable host name
// in the condition message; if a host UUID is absent from the map the UUID is used as fallback.
// Returns an abnormal VolumeCondition if any live host is missing an attached PBD for the SR,
// or nil if there are no hosts to check (all halted or no published nodes).
func volumeConditionFromPBDs(sr *payloads.StorageRepository, publishedHosts []uuid.UUID, pbds []*payloads.PBD, hostMap map[uuid.UUID]*payloads.Host) *csi.VolumeCondition {
	// Build a set of hosts that have an attached PBD for this SR.
	attachedHosts := make(map[uuid.UUID]struct{}, len(pbds))
	for _, pbd := range pbds {
		if pbd.Attached {
			attachedHosts[pbd.Host] = struct{}{}
		}
	}

	for _, hostID := range publishedHosts {
		// Skip halted VMs (Container == uuid.Nil means no host).
		if hostID == uuid.Nil {
			continue
		}
		if _, ok := attachedHosts[hostID]; !ok {
			hostLabel := hostID.String()
			if h, found := hostMap[hostID]; found {
				hostLabel = fmt.Sprintf("'%s' (%s)", h.NameLabel, hostID)
			}
			return &csi.VolumeCondition{
				Abnormal: true,
				Message:  fmt.Sprintf("Abnormal - SR '%s' (%s) has no attached PBD on host %s", sr.NameLabel, sr.ID, hostLabel),
			}
		}
	}
	return nil
}

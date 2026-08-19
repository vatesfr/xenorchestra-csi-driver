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
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_MODIFY_VOLUME,
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
// It allows migrating a VDI to a different Storage Repository (SR) within the
// same pool, triggered by the external-resizer when a VolumeAttributesClass is
// applied to a PVC.
func (driver *xenorchestraCSIDriver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	klog.V(2).InfoS("ControllerModifyVolume called", "request", req)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Volume ID is required")
	}

	targetSRStr := req.GetMutableParameters()[ParameterStorageRepository]
	if targetSRStr == "" {
		return nil, status.Errorf(codes.InvalidArgument,
			"mutable parameter %q is required for migration", ParameterStorageRepository)
	}

	targetSRUUID, err := uuid.FromString(targetSRStr)
	if err != nil || targetSRUUID == uuid.Nil {
		klog.ErrorS(err, "invalid SR UUID", "srID", targetSRStr)
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid SR UUID %q: must be a valid UUID", targetSRStr)
	}

	// Look up the existing VDI.
	vdi, err := driver.xoClient.GetVDIByVolumeId(ctx, volumeID)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			klog.ErrorS(err, "Volume handle not found during ControllerModifyVolume", "volumeID", volumeID)
			return nil, status.Errorf(codes.NotFound, "volume %s not found: %v", volumeID, err)
		}
		return nil, status.Errorf(codes.Internal, "failed to look up volume %s: %v", volumeID, err)
	}

	// No-op if the VDI is already on the target SR.
	if vdi.SR == targetSRUUID {
		klog.V(5).InfoS("VDI is already on the target SR, nothing to do",
			"vdiID", vdi.ID, "srID", vdi.SR)
		return &csi.ControllerModifyVolumeResponse{}, nil
	}

	// Look up the target SR to verify it exists and is in the same pool.
	targetSR, err := driver.xoClient.SR().Get(ctx, targetSRUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to look up target SR", "srID", targetSRUUID)
		return nil, status.Errorf(codes.NotFound,
			"failed to look up SR %s: %v", targetSRUUID, err)
	}

	if vdi.PoolID != targetSR.Pool {
		klog.ErrorS(nil, "Target SR is not in the same pool as the VDI",
			"vdiPool", vdi.PoolID, "srPool", targetSR.Pool)
		return nil, status.Errorf(codes.InvalidArgument,
			"SR %s is not in the same pool as VDI %s (vdiPool=%s, srPool=%s)",
			targetSRUUID, vdi.ID, vdi.PoolID, targetSR.Pool)
	}

	// Perform the migration.
	klog.V(5).InfoS("Migrating VDI to target SR",
		"vdiID", vdi.ID, "fromSR", vdi.SR, "toSR", targetSRUUID)
	newVDIUUID, err := driver.xoClient.MigrateVDIAndWait(ctx, *vdi, targetSRUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to migrate VDI", "vdiID", vdi.ID, "targetSR", targetSRUUID)
		return nil, status.Errorf(codes.Internal,
			"failed to migrate VDI %s to SR %s: %v", vdi.ID, targetSRUUID, err)
	}
	klog.V(5).InfoS("VDI migrated successfully",
		"oldVDIID", vdi.ID, "newVDIID", newVDIUUID, "srID", targetSRUUID)

	return &csi.ControllerModifyVolumeResponse{}, nil
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
			klog.V(2).InfoS("Volume handle not found during ControllerPublishVolume", "volumeID", volumeId)
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

	// For local storage, migrate the VDI to the host's local SR if needed.
	if req.GetVolumeContext()[VolumeContextKeyStorageType] == StorageTypeLocal {
		localSR, err := driver.xoClient.FindLocalSRForHost(ctx, nodeVM.Container)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition,
				"no local SR found for host %s: %v", nodeVM.Container, err)
		}
		if vdi.SR != localSR.ID {
			klog.V(2).InfoS("Migrating VDI to local SR",
				"vdiID", vdi.ID, "fromSR", vdi.SR, "toSR", localSR.ID)
			newVDIUUID, err := driver.xoClient.MigrateVDIAndWait(ctx, *vdi, localSR.ID)
			if err != nil {
				klog.ErrorS(err, "Failed to migrate VDI to local SR", "vdiID", vdi.ID, "localSRID", localSR.ID)
				return nil, status.Errorf(codes.Internal,
					"failed to migrate VDI %s to local SR %s: %v", vdi.ID, localSR.ID, err)
			}
			vdi, err = driver.xoClient.VDI().Get(ctx, newVDIUUID)
			if err != nil {
				klog.ErrorS(err, "Failed to fetch VDI after migration", "newVDIID", newVDIUUID)
				return nil, status.Errorf(codes.Internal,
					"failed to fetch VDI after migration (newUUID=%s): %v", newVDIUUID, err)
			}
			klog.V(2).InfoS("VDI migrated successfully", "newVDIID", vdi.ID, "srID", localSR.ID)
		} else {
			klog.V(4).InfoS("VDI already on correct local SR, skipping migration",
				"vdiID", vdi.ID, "srID", localSR.ID)
		}
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

	klog.V(5).InfoS("Creating volume", "namePrefix", driver.vdiNamePrefix, "volumeName", volumeName, "capacityBytes", capacityBytes)

	// Requested storage type: "shared" (default) or "local".
	storageType, hasStorageTypeParam := req.GetParameters()[ParameterStorageType]
	if storageType == "" {
		storageType = StorageTypeShared
	}
	if storageType != StorageTypeShared && storageType != StorageTypeLocal {
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid storageType %q: must be %q or %q", storageType, StorageTypeShared, StorageTypeLocal)
	}

	// Resolve the pool and SR to provision into. Strategies, in priority order:
	//  1. explicit target SR in VolumeAttributesClass mutable parameters,
	//  2. explicit poolId in the StorageClass parameters,
	//  3. pool derived from accessibility_requirements (preferred, then
	//     requisite), with a tag-based fallback when no topology is present.
	pool, sr, err := driver.selectVolumeTarget(ctx, req, storageType, hasStorageTypeParam)
	if err != nil {
		return nil, err
	}

	// Idempotency check: return the existing VDI if one was already created for this PV name.
	if resp, ok, err := driver.existingVolumeResponse(ctx, volumeName, capacityBytes, pool, sr, storageType); ok {
		return resp, nil
	} else if err != nil {
		return nil, err
	}

	vdiID, volumeID, err := driver.xoClient.CreateNewVolume(ctx, sr.ID, driver.vdiNamePrefix, capacityBytes, volumeName, driver.Name+"@"+driver.Version, driver.clusterTag)
	if err != nil {
		klog.ErrorS(err, "Failed to create VDI", "volumeName", volumeName, "capacityBytes", capacityBytes)
		return nil, status.Errorf(codes.Internal, "Failed to create VDI: %v", err)
	}
	klog.V(5).InfoS("VDI created", "vdiID", vdiID, "volumeID", volumeID, "volumeName", volumeName)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           volumeID.String(),
			CapacityBytes:      capacityBytes,
			AccessibleTopology: driver.buildAccessibleTopology(pool),
			VolumeContext:      buildVolumeContext(pool, sr, storageType),
		},
	}, nil
}

// selectVolumeTarget resolves the pool and SR that the volume should be
// provisioned into. Three strategies are tried in priority order:
//
//  1. An explicit target SR from the VolumeAttributesClass mutable parameters
//     (CSI 1.10+, Kubernetes 1.31+): the SR must exist, be in the pool named by
//     the poolId parameter when one is set, and match the requested
//     storageType when one is explicitly set.
//  2. An explicit poolId StorageClass parameter: validated against the
//     accessibility_requirements requisite topologies, then the pool and its
//     SR are selected.
//  3. A pool derived from accessibility_requirements: preferred topologies are
//     tried first (in order), then requisite as fallback. When no topology is
//     provided, pools tagged with the Kubernetes pool tag are used instead.
//
// For local storage the SR is overridden with one of the pool's local SRs (the
// DefaultSR is shared), so the VDI lands on local storage from the start. The
// override is skipped when an explicit target SR was provided via
// VolumeAttributesClass.
func (driver *xenorchestraCSIDriver) selectVolumeTarget(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
	storageType string,
	hasStorageTypeParam bool,
) (*payloads.Pool, *payloads.StorageRepository, error) {
	poolIDStr := req.GetParameters()[ParameterPoolID]
	ar := req.GetAccessibilityRequirements()

	// Explicit target SR in VolumeAttributesClass (VAC) parameters.
	// VAC parameters are passed in mutable_parameters (CSI 1.10+, Kubernetes 1.31+).
	targetSRStr, hasTargetSR := req.GetMutableParameters()[ParameterStorageRepository]
	useTargetSR := hasTargetSR && targetSRStr != ""

	var pool *payloads.Pool
	var sr *payloads.StorageRepository
	var err error

	if useTargetSR {
		pool, sr, err = driver.selectSRFromVolumeAttributesClass(ctx, targetSRStr, poolIDStr, storageType, hasStorageTypeParam)
	} else if poolIDStr != "" {
		pool, sr, err = driver.selectPoolFromParameter(ctx, poolIDStr, ar)
	} else {
		pool, sr, err = driver.selectPoolFromAccessibilityRequirements(ctx, ar)
	}
	if err != nil {
		return nil, nil, err
	}

	klog.V(5).InfoS("Using pool and SR", "poolID", pool.ID, "srID", sr.ID)

	// For local storage, override the SR with one of the pool's local SRs so
	// the VDI lands on local storage from the start rather than on the shared
	// DefaultSR. That will help avoid an extra migration step in the common case
	// where the volume is created and attached to the same node.
	// Skip this override when a specific SR was provided via VolumeAttributesClass.
	if !useTargetSR && storageType == StorageTypeLocal {
		localSRs, err := driver.xoClient.FindLocalSRsForPool(ctx, pool.ID)
		if err != nil {
			return nil, nil, status.Errorf(codes.FailedPrecondition, "no local SR available in pool %s: %v", pool.ID, err)
		}
		sr = localSRs[0]
		klog.V(4).InfoS("Local storageType: using local SR for initial VDI creation", "poolID", pool.ID, "srID", sr.ID)
	}

	return pool, sr, nil
}

// selectSRFromVolumeAttributesClass resolves the target SR specified in the
// VolumeAttributesClass (VAC) mutable parameters (CSI 1.10+, Kubernetes
// 1.31+). The SR must exist, be in the pool named by the poolId parameter when
// one is set, and match the requested storageType when one is explicitly set.
func (driver *xenorchestraCSIDriver) selectSRFromVolumeAttributesClass(
	ctx context.Context,
	targetSRStr,
	poolIDStr,
	storageType string,
	hasStorageTypeParam bool,
) (*payloads.Pool, *payloads.StorageRepository, error) {
	targetSRUUID, err := uuid.FromString(targetSRStr)
	if err != nil || targetSRUUID == uuid.Nil {
		klog.ErrorS(err, "Invalid SR UUID in VolumeAttributesClass", "srID", targetSRStr)
		return nil, nil, status.Errorf(codes.InvalidArgument,
			"invalid SR UUID %q in VolumeAttributesClass: must be a valid UUID", targetSRStr)
	}

	targetSR, err := driver.xoClient.SR().Get(ctx, targetSRUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to look up SR from VolumeAttributesClass", "srID", targetSRUUID)
		return nil, nil, status.Errorf(codes.InvalidArgument,
			"SR %s specified in VolumeAttributesClass not found: %v", targetSRUUID, err)
	}

	// Verify the SR is in the selected pool.
	if poolIDStr != "" && targetSR.Pool.String() != poolIDStr {
		klog.ErrorS(nil, "Target SR is not in the selected pool",
			"srPool", targetSR.Pool, "selectedPool", poolIDStr)
		return nil, nil, status.Errorf(codes.InvalidArgument,
			"SR %s is not in the selected pool %s (srPool=%s)",
			targetSRUUID, poolIDStr, targetSR.Pool)
	}

	// If storageType is explicitly specified, validate it against the SR type.
	if hasStorageTypeParam {
		// Verify the SR type matches the requested storageType.
		wantShared := storageType == StorageTypeShared
		if targetSR.Shared != wantShared {
			srType := "local"
			if targetSR.Shared {
				srType = "shared"
			}
			klog.ErrorS(nil, "Target SR type does not match storageType",
				"targetSRType", srType, "storageType", storageType)
			return nil, nil, status.Errorf(codes.InvalidArgument,
				"SR %s is %s but requested storageType is %q", targetSRUUID, srType, storageType)
		}
	}

	// Fetch the pool object for topology and volume context.
	pool, err := driver.xoClient.Pool().Get(ctx, targetSR.Pool)
	if err != nil {
		klog.ErrorS(err, "Failed to get pool from target SR", "poolID", targetSR.Pool)
		return nil, nil, status.Errorf(codes.InvalidArgument,
			"pool %s of SR %s not found: %v", targetSR.Pool, targetSRUUID, err)
	}
	klog.V(4).InfoS("Using SR from VolumeAttributesClass", "srID", targetSR.ID, "poolID", pool.ID)
	return pool, targetSR, nil
}

// selectPoolFromParameter resolves the pool named by the poolId StorageClass
// parameter, validating it against the requisite topologies before selecting
// the pool and its SR.
func (driver *xenorchestraCSIDriver) selectPoolFromParameter(ctx context.Context, poolIDStr string, ar *csi.TopologyRequirement) (*payloads.Pool, *payloads.StorageRepository, error) {
	poolUUID, err := uuid.FromString(poolIDStr)
	if err != nil || poolUUID == uuid.Nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "parameter %q must be a valid UUID, got %q", ParameterPoolID, poolIDStr)
	}
	if err := topology.ValidatePoolIDAgainstRequisite(ar, poolUUID); err != nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	pool, sr, err := topology.SelectPoolAndStorage(ctx, driver.xoClient.SR(), driver.xoClient.Pool(), []uuid.UUID{poolUUID})
	if err != nil {
		klog.ErrorS(err, "Pool or SR not viable", "poolID", poolUUID)
		return nil, nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	return pool, sr, nil
}

// selectPoolFromAccessibilityRequirements derives the pool from the
// accessibility_requirements: preferred topologies are tried first (in order),
// then requisite topologies as fallback. When no topology is provided, pools
// tagged with the Kubernetes pool tag are used instead.
func (driver *xenorchestraCSIDriver) selectPoolFromAccessibilityRequirements(ctx context.Context, ar *csi.TopologyRequirement) (*payloads.Pool, *payloads.StorageRepository, error) {
	orderedPoolIDs, err := topology.OrderedPoolIDs(ar)
	if err != nil {
		if !errors.Is(err, topology.ErrNoPoolInTopology) {
			return nil, nil, status.Errorf(codes.Internal,
				"failed to derive pool from accessibility_requirements: %v", err)
		}
		// Fallback: discover pools by tag when no topology constraints are provided.
		klog.V(2).InfoS("No pool topology found in accessibility_requirements, falling back to tag-based pool discovery",
			"kubernetesPoolTag", driver.kubernetesPoolTag)

		orderedPoolIDs, err = topology.TaggedPoolIDs(ctx, driver.xoClient.Pool(), driver.kubernetesPoolTag)
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal,
				"failed to list pools for tag-based fallback: %v", err)
		}
		if len(orderedPoolIDs) == 0 {
			return nil, nil, status.Errorf(codes.FailedPrecondition,
				"no pool found with tag %q and no topology requirements provided",
				driver.kubernetesPoolTag)
		}
	}
	pool, sr, err := topology.SelectPoolAndStorage(ctx, driver.xoClient.SR(), driver.xoClient.Pool(), orderedPoolIDs)
	if err != nil {
		klog.ErrorS(err, "No viable pool found in accessibility_requirements")
		return nil, nil, status.Errorf(codes.ResourceExhausted, "%v", err)
	}
	klog.V(5).InfoS("No poolId parameter, selected pool from accessibility_requirements", "poolID", pool.ID)
	return pool, sr, nil
}

// existingVolumeResponse implements the idempotency check of CreateVolume: if
// a VDI already exists for volumeName, it returns the CreateVolumeResponse
// that should be served for it. ok is true when an existing VDI was found and
// the response can be returned as-is; otherwise it is false and err (if any)
// describes why the request failed.
func (driver *xenorchestraCSIDriver) existingVolumeResponse(
	ctx context.Context,
	volumeName string,
	capacityBytes int64,
	pool *payloads.Pool,
	sr *payloads.StorageRepository,
	storageType string,
) (*csi.CreateVolumeResponse, bool, error) {
	existingVDI, existingId, err := driver.xoClient.FindVDIByVolumeName(ctx, volumeName)
	if err != nil {
		if errors.Is(err, clients.ErrVolumeNotFound) {
			existingVDI = nil
		} else {
			klog.ErrorS(err, "Failed to check for existing VDI", "volumeName", volumeName)
			return nil, false, status.Errorf(codes.Internal, "failed to check for existing VDI: %v", err)
		}
	}
	if existingVDI == nil {
		return nil, false, nil
	}
	if existingVDI.Size != capacityBytes {
		return nil, false, status.Errorf(codes.AlreadyExists, "volume with name %q already exists with different capacity: existing %d, requested %d", volumeName, existingVDI.Size, capacityBytes)
	}
	// Recover the stable volume ID stored at creation time.
	if existingId == "" {
		return nil, false, status.Errorf(codes.Internal, "existing VDI %s is missing volume ID in tags", existingVDI.ID)
	}
	klog.V(5).InfoS("Volume already exists, returning existing VDI", "vdiID", existingVDI.ID, "volumeId", existingId)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           existingId,
			CapacityBytes:      capacityBytes,
			AccessibleTopology: driver.buildAccessibleTopology(pool),
			VolumeContext:      buildVolumeContext(pool, sr, storageType),
		},
	}, true, nil
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
		if clients.IsNotFoundError(err) {
			// Deleted by a concurrent call between our lookup and Delete
			klog.V(4).InfoS("VDI not found during delete call, already deleted by concurrent call", "volumeID", volumeID, "vdiID", vdi.ID)
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
			klog.V(2).InfoS("VDI not found during ValidateVolumeCapabilities", "volumeID", volumeID)
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
func buildVolumeContext(pool *payloads.Pool, sr *payloads.StorageRepository, storageType string) map[string]string {
	return map[string]string{
		VolumeContextKeySRID:        sr.ID.String(),
		VolumeContextKeySRName:      sr.NameLabel,
		VolumeContextKeyPoolID:      pool.ID.String(),
		VolumeContextKeyPoolName:    pool.NameLabel,
		VolumeContextKeyStorageType: storageType,
	}
}

func publishContextFromVBD(vbd payloads.VBD) map[string]string {
	return map[string]string{
		"device": *vbd.Device,
		"vbd":    vbd.ID.String(),
	}
}

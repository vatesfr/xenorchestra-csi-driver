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
package clients

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"

	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"

	"k8s.io/klog/v2"
)

// This interface extends the library.Library interface to add methods specific to Xen Orchestra operations needed by the CSI driver.
// It's done to encapsulate call to the legacy v1 client waiting for the v2 client to support all required operations.
//
//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=xoclient.go -destination=mock/xoclient.go -package=mock
type XoClient interface {
	library.Library
	GetVBDFromVDIAndVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) (*payloads.VBD, error)
	ConnectVBDToVM(ctx context.Context, vbd payloads.VBD) (*payloads.VBD, error)
	DisconnectVBDFromVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) error
	AttachVDIToVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) (*payloads.VBD, error)
	CreateNewVolume(ctx context.Context, srID uuid.UUID, namePrefix string, capacityBytes int64, volumeName string, managedBy string, clusterTag string) (uuid.UUID, uuid.UUID, error)
	WaitForVDIToBeFullyAttached(ctx context.Context, vbdID uuid.UUID) (*payloads.VBD, error)
	IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error)
	// FindVDIByVolumeName looks up a VDI by the Kubernetes PV name stored in
	// the VDI tag "k8s:pvName:<volumeName>". It returns nil, nil when not found.
	FindVDIByVolumeName(ctx context.Context, volumeName string) (*payloads.VDI, string, error)
	// IsSRAttachedToHost checks that the given SR is connected (via a plugged PBD) to the given host.
	// Returns nil when the SR is reachable, or a descriptive error otherwise.
	IsSRAttachedToHost(ctx context.Context, srID uuid.UUID, hostID uuid.UUID) error
	// IsSRAttachedToVMHost checks that the SR backing the given VBD is connected (via a plugged PBD)
	// to the XCP-ng host where the VBD's VM is currently running.
	// Returns nil when the SR is reachable, or a descriptive error otherwise.
	IsSRAttachedToVMHost(ctx context.Context, vbdID uuid.UUID) error

	// GetVDIByVolumeId looks up a VDI by its CSI volume ID (UUID).
	// Lookup order:
	//  1. VDI tag "k8s:volumeId:<volumeId>" (primary, v0.4.0+ and migrated volumes)
	//  2. name_label containing the volumeId (migrated v0.3.0 volumes, tag recovery)
	//  3. Direct VDI UUID lookup (static volumes using raw VDI UUID as volumeHandle)
	// Returns ErrVolumeNotFound if no VDI matches, ErrVolumeIdAmbiguous if multiple match.
	GetVDIByVolumeId(ctx context.Context, volumeId string) (*payloads.VDI, error)

	// FindLocalSRForHost returns the first local (non-shared) user SR whose
	// container is the given host. Returns an error if none is found.
	FindLocalSRForHost(ctx context.Context, hostID uuid.UUID) (*payloads.StorageRepository, error)

	// FindLocalSRsForPool returns all local (non-shared) user SRs belonging to
	// the given pool. Returns an error if the API call fails or none are found.
	FindLocalSRsForPool(ctx context.Context, poolID uuid.UUID) ([]*payloads.StorageRepository, error)

	// MigrateVDIAndWait migrates vdi to targetSRID and blocks until the task
	// completes. Returns the new VDI UUID assigned by XAPI after migration.
	MigrateVDIAndWait(ctx context.Context, vdi payloads.VDI, targetSRID uuid.UUID) (uuid.UUID, error)
}

type xoClient struct {
	library.Library
}

func NewXoClient(libraryService library.Library) XoClient {
	return xoClient{
		Library: libraryService,
	}
}

func (c xoClient) GetVBDFromVDIAndVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) (*payloads.VBD, error) {
	vbs, err := c.VBD().GetAll(ctx, 0, fmt.Sprintf("VDI:%s VM:%s", vdi.ID, vmUUID))
	if err != nil {
		klog.ErrorS(err, "Failed to get VBDs for VDI and VM", "vdi", vdi, "vmUUID", vmUUID)
		return nil, err
	}

	if len(vbs) > 1 {
		klog.InfoS("The VDI is attached more than once to the VM. Return the first result.")
	}

	if len(vbs) == 0 {
		return nil, fmt.Errorf("vdi=%s vm=%s: %w", vdi.ID, vmUUID, ErrVBDNotFound)
	}
	return vbs[0], nil
}

func (c xoClient) ConnectVBDToVM(ctx context.Context, vbd payloads.VBD) (*payloads.VBD, error) {
	taskID, err := c.VBD().Connect(ctx, vbd.ID)
	if err != nil {
		klog.ErrorS(err, "Failed to connect existing VBD to the node", "vbd", vbd)
		return nil, err
	}

	task, err := c.Task().Wait(ctx, taskID)
	if err != nil {
		klog.ErrorS(err, "Failed to wait for task to complete", "taskID", taskID, "taskResult", task.Result)
		return nil, err
	}

	updatedVBD, err := c.VBD().Get(ctx, vbd.ID)
	if err != nil {
		klog.ErrorS(err, "Failed to get updated VBD after connecting", "vbd", vbd)
		return nil, err
	}

	return updatedVBD, nil
}

func (c xoClient) AttachVDIToVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) (*payloads.VBD, error) {
	vbdID, err := c.VBD().Create(ctx, &payloads.CreateVBDParams{
		VM:   vmUUID,
		VDI:  vdi.ID,
		Mode: payloads.VBDModeRW,
	})
	if err != nil {
		klog.ErrorS(err, "Failed to create VBD to attach VDI to the node", "vdi", vdi, "vmUUID", vmUUID)
		return nil, err
	}

	// Wait for the attach operation to complete
	vbd, err := c.WaitForVDIToBeFullyAttached(ctx, vbdID)
	if err != nil {
		klog.ErrorS(err, "Failed to wait for attach disk task", "vdi", vdi.ID, "vm", vmUUID)
		return nil, err
	}

	klog.V(4).InfoS("attachDiskToVM: disk attached", "vdi", vdi.ID, "vm", vmUUID, "vbd", vbd)

	return vbd, nil
}

func (c xoClient) CreateNewVolume(ctx context.Context, srID uuid.UUID, namePrefix string, capacityBytes int64, volumeName string, managedBy string, clusterTag string) (uuid.UUID, uuid.UUID, error) {
	volumeId, err := uuid.NewV4()
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("failed to generate volume ID UUID: %w", err)
	}

	tags := []string{
		BuildTag(VDITagKeyVolumeId, volumeId.String()),
		BuildTag(VDITagKeyPVName, volumeName),
		BuildTag(VDITagKeyManagedBy, managedBy),
	}
	if clusterTag != "" {
		tags = append(tags, clusterTag)
	}

	vdiParams := payloads.VDICreateParams{
		SRId:            srID,
		NameLabel:       BuildVDINameLabel(namePrefix, volumeId.String(), volumeName),
		VirtualSize:     capacityBytes,
		NameDescription: BuildVDINameDescription(volumeName),
		Tags:            tags,
	}

	vdiID, err := c.VDI().Create(ctx, vdiParams)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("failed to create VDI: %w", err)
	}

	return vdiID, volumeId, nil
}

// waitForDiskToBeAttached waits for the disk to be attached.
// This pools every 500ms to check if the disk is attached and if a device name is assigned.
// Hardcoded timeout of 2 minutes.
// NOTE: This is required because the VBD can be attached but returned without a device name when `vm.attach` command succeeded.
// See: https://github.com/vatesfr/xen-orchestra/pull/9192
func (c xoClient) WaitForVDIToBeFullyAttached(ctx context.Context, vbdID uuid.UUID) (*payloads.VBD, error) {
	timeout := time.After(2 * time.Minute)
	tick := time.Tick(1 * time.Second)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timed out waiting for VBD %s to be attached", vbdID)
		case <-tick:
			vbd, err := c.VBD().Get(ctx, vbdID)
			if err != nil {
				klog.ErrorS(err, "Failed to get VBD while waiting for disk to be attached", "vbd", vbdID)
				continue
			}
			if vbd.Attached && vbd.Device != nil && *vbd.Device != "" {
				klog.V(4).InfoS("Disk is now attached", "vbd", vbd.ID, "vm", vbd.VM, "device", vbd.Device)
				return vbd, nil
			}
			klog.V(5).InfoS("Disk not yet attached, waiting...", "vbd", vbd.ID, "vm", vbd.VM)
		}
	}
}

// IsVDIUsedAnywhere checks if a VDI is used by any VM in the Xen Orchestra instance.
// If it is used, it returns the list of VBDs it is added to.
// If it is not used, it returns an empty slice.
func (c xoClient) IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error) {
	vbds, err := c.VBD().GetAll(ctx, 0, fmt.Sprintf("VDI:%s", vdi.ID))
	if err != nil {
		return nil, err
	}

	return vbds, nil
}

// IsSRAttachedToHost checks that the given SR is connected (via a plugged PBD) to the given host.
func (c xoClient) IsSRAttachedToHost(ctx context.Context, srID uuid.UUID, hostID uuid.UUID) error {
	pbds, err := c.PBD().GetAll(ctx, 1, fmt.Sprintf("SR:%s host:%s attached?", srID, hostID))
	if err != nil {
		return fmt.Errorf("failed to list PBDs for SR %s on host %s: %w", srID, hostID, err)
	}
	if len(pbds) == 0 {
		return fmt.Errorf("SR %s is not connected to host %s (no plugged PBD found)", srID, hostID)
	}
	return nil
}

// IsSRAttachedToVMHost checks that the SR backing the given VBD is connected (via a plugged PBD)
// to the XCP-ng host where the VBD's VM is currently running.
func (c xoClient) IsSRAttachedToVMHost(ctx context.Context, vbdID uuid.UUID) error {
	vbd, err := c.VBD().Get(ctx, vbdID)
	if err != nil {
		return fmt.Errorf("failed to get VBD %s: %w", vbdID, err)
	}
	if vbd.VDI == nil {
		return fmt.Errorf("VBD %s has no VDI", vbdID)
	}

	vm, err := c.VM().GetByID(ctx, vbd.VM)
	if err != nil {
		return fmt.Errorf("failed to get VM %s for VBD %s: %w", vbd.VM, vbdID, err)
	}

	vdi, err := c.VDI().Get(ctx, *vbd.VDI)
	if err != nil {
		return fmt.Errorf("failed to get VDI %s: %w", vbd.VDI, err)
	}

	return c.IsSRAttachedToHost(ctx, vdi.SR, vm.Container)
}

func (c xoClient) DisconnectVBDFromVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) error {
	vbd, err := c.GetVBDFromVDIAndVM(ctx, vdi, vmUUID)
	if err != nil {
		return err
	}
	taskID, err := c.VBD().Disconnect(ctx, vbd.ID)
	if err != nil {
		klog.ErrorS(err, "Failed to disconnect VBD from the node", "vbdID", vbd.ID)
		return err
	}
	task, err := c.Task().Wait(ctx, taskID)
	if err != nil {
		klog.ErrorS(err, "Failed to wait for task to complete", "taskID", taskID, "taskResult", task.Result)
	}
	return err
}

func (c xoClient) GetVDIByVolumeId(ctx context.Context, volumeId string) (*payloads.VDI, error) {
	// 1. Primary: look up by VDI tag "k8s:volumeId:<volumeId>".
	filter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
	vdis, err := c.VDI().GetAll(ctx, 2, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list VDIs by volume ID %s: %w", volumeId, err)
	}
	if len(vdis) == 1 {
		return vdis[0], nil
	}
	if len(vdis) > 1 {
		return nil, fmt.Errorf("%w: volumeId=%s matched %d VDIs via tag", ErrVolumeIdAmbiguous, volumeId, len(vdis))
	}

	// 2. Fallback: search by name_label containing the volumeId.
	// This handles the case where tags were erased (e.g. after a migration).
	klog.V(2).InfoS("No VDI found with tag volume ID, falling back to name_label search", "volumeId", volumeId)
	fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
	vdis, err = c.VDI().GetAll(ctx, 2, fallbackFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list VDIs by name_label for volume ID %s: %w", volumeId, err)
	}
	if len(vdis) == 1 {
		// Recover VDI tags for the found VDI to ensure it can be found by volume ID in the future.
		klog.V(2).InfoS("Found VDI by name_label fallback, recovering tags", "volumeId", volumeId, "vdiID", vdis[0].ID)
		c.recoverVolumeLookupTags(ctx, vdis[0], volumeId)
		return vdis[0], nil
	}
	if len(vdis) > 1 {
		return nil, fmt.Errorf("%w: volumeId=%s matched %d VDIs via name_label fallback", ErrVolumeIdAmbiguous, volumeId, len(vdis))
	}

	// 3. Final fallback: direct VDI UUID lookup for static volumes.
	// Static provisioning uses the raw VDI UUID as volumeHandle. This lookup
	// is intentionally last because dynamic volume handles can also be UUID-shaped,
	// and we must avoid returning the wrong VDI.
	if parsedUUID, err := uuid.FromString(volumeId); err == nil && parsedUUID != uuid.Nil {
		klog.V(2).InfoS("No VDI found by name_label, falling back to direct VDI UUID lookup", "volumeId", volumeId)
		vdi, err := c.VDI().Get(ctx, parsedUUID)
		if err != nil {
			if IsNotFoundError(err) {
				return nil, ErrVolumeNotFound
			}
			return nil, fmt.Errorf("failed to get VDI by direct UUID %s: %w", volumeId, err)
		}
		// Recover the tag so future lookups hit the primary path.
		klog.V(2).InfoS("Found VDI by direct UUID lookup, recovering tags", "volumeId", volumeId, "vdiID", vdi.ID)
		c.recoverVolumeLookupTags(ctx, vdi, volumeId)
		return vdi, nil
	}

	return nil, ErrVolumeNotFound
}

func (c xoClient) FindVDIByVolumeName(ctx context.Context, volumeName string) (*payloads.VDI, string, error) {
	filter := BuildTagFilter(VDITagKeyPVName, volumeName)
	vdis, err := c.VDI().GetAll(ctx, 2, filter)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list VDIs by volume name %s: %w", volumeName, err)
	}
	switch len(vdis) {
	case 0:
		return nil, "", ErrVolumeNotFound
	case 1:
		return vdis[0], ParseTagValue(vdis[0].Tags, VDITagKeyVolumeId), nil
	default:
		return nil, "", fmt.Errorf("%w: volumeName=%s matched %d VDIs", ErrVolumeNameAmbiguous, volumeName, len(vdis))
	}
}

func (c xoClient) FindLocalSRForHost(ctx context.Context, hostID uuid.UUID) (*payloads.StorageRepository, error) {
	filter := fmt.Sprintf("content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $container:%s", hostID)
	srs, err := c.SR().GetAll(ctx, 1, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list local SRs for host %s: %w", hostID, err)
	}
	if len(srs) == 0 {
		return nil, fmt.Errorf("no local SR with a connected PBD found on host %s", hostID)
	}
	return srs[0], nil
}

func (c xoClient) FindLocalSRsForPool(ctx context.Context, poolID uuid.UUID) ([]*payloads.StorageRepository, error) {
	filter := fmt.Sprintf("content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $pool:%s", poolID)
	srs, err := c.SR().GetAll(ctx, 0, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list local SRs for pool %s: %w", poolID, err)
	}
	if len(srs) == 0 {
		return nil, fmt.Errorf("no local SR with a connected PBD found in pool %s", poolID)
	}
	return srs, nil
}

func (c xoClient) MigrateVDIAndWait(ctx context.Context, vdi payloads.VDI, targetSRID uuid.UUID) (uuid.UUID, error) {
	// Workaround for the tags that are not copied during migration.
	oldTags := vdi.Tags

	klog.V(2).InfoS("Starting VDI migration", "vdiID", vdi.ID, "fromSR", vdi.SR, "toSR", targetSRID, "oldTags", oldTags)
	taskID, err := c.VDI().Migrate(ctx, vdi.ID, targetSRID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to start VDI migration (vdiID=%s targetSR=%s): %w", vdi.ID, targetSRID, err)
	}
	task, err := c.Task().Wait(ctx, taskID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to wait for VDI migration task %s: %w", taskID, err)
	}
	if task.Status != payloads.Success {
		return uuid.Nil, fmt.Errorf("VDI migration task %s finished with status %q: %s", taskID, task.Status, task.Result.Message)
	}
	if task.Result.ID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("VDI migration task %s succeeded but returned no new VDI UUID", taskID)
	}
	newVDIID := task.Result.ID
	klog.V(2).InfoS("VDI migration completed", "oldVDIID", vdi.ID, "newVDIID", newVDIID)

	// We need to manually copy the "k8s:volumeId:<uuid>" tag from the old VDI to the new one to ensure the new VDI can be found by volume ID after migration.
	c.writeTagsToVDI(ctx, newVDIID, oldTags)
	return newVDIID, nil
}

func (c xoClient) recoverVolumeLookupTags(ctx context.Context, vdi *payloads.VDI, volumeId string) {
	tagsToRecover := []string{BuildTag(VDITagKeyVolumeId, volumeId)}
	if recoveredVolumeName := recoverVolumeNameFromVDI(vdi, volumeId); recoveredVolumeName != "" {
		tagsToRecover = append(tagsToRecover, BuildTag(VDITagKeyPVName, recoveredVolumeName))
	} else {
		klog.V(3).InfoS("Could not recover pvName from VDI metadata during fallback", "volumeId", volumeId, "vdiID", vdi.ID)
	}
	c.writeTagsToVDI(ctx, vdi.ID, tagsToRecover)
}

func (c xoClient) writeTagsToVDI(ctx context.Context, vdiID uuid.UUID, tags []string) {
	for _, tag := range tags {
		if strings.HasPrefix(tag, tagPrefix+":") {
			if err := c.VDI().AddTag(ctx, vdiID, tag); err != nil {
				klog.ErrorS(err, "Failed to copy tag to VDI", "vdiID", vdiID, "tag", tag)
				// Not returning an error here since the migration itself succeeded and the volume can still be found by name, but logging it for troubleshooting.
			}
		}
	}
}

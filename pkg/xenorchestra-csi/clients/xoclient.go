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
	CreateNewVolume(ctx context.Context, srID uuid.UUID, diskName string, capacityBytes int64, volumeName string, createdBy string, clusterTag string) (uuid.UUID, uuid.UUID, error)
	WaitForVDIToBeFullyAttached(ctx context.Context, vbdID uuid.UUID) (*payloads.VBD, error)
	IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error)
	// FindVDIByVolumeName looks up a VDI by the Kubernetes PV name stored in
	// VDI.other_config["kubernetesPVName"]. It returns nil, nil when not found.
	FindVDIByVolumeName(ctx context.Context, volumeName string) (*payloads.VDI, string, error)
	// IsSRAttachedToHost checks that the given SR is connected (via a plugged PBD) to the given host.
	// Returns nil when the SR is reachable, or a descriptive error otherwise.
	IsSRAttachedToHost(ctx context.Context, srID uuid.UUID, hostID uuid.UUID) error
	// IsSRAttachedToVMHost checks that the SR backing the given VBD is connected (via a plugged PBD)
	// to the XCP-ng host where the VBD's VM is currently running.
	// Returns nil when the SR is reachable, or a descriptive error otherwise.
	IsSRAttachedToVMHost(ctx context.Context, vbdID uuid.UUID) error

	// GetVDIByVolumeId looks up a VDI by its CSI volume ID (UUID).
	// The ID is stored in VDI.other_config["csi-volume-handle"] at creation time
	// and remains stable across SR migrations.
	// Returns ErrVolumeNotFound if no VDI matches, ErrVolumeIdAmbiguous if multiple match.
	GetVDIByVolumeId(ctx context.Context, volumeId string) (*payloads.VDI, error)
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

func (c xoClient) CreateNewVolume(ctx context.Context, srID uuid.UUID, diskName string, capacityBytes int64, volumeName string, createdBy string, clusterTag string) (uuid.UUID, uuid.UUID, error) {
	volumeId, err := uuid.NewV4()
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("failed to generate volume ID UUID: %w", err)
	}

	otherConfig := map[string]string{
		VDIOtherConfigKeyCreatedBy: createdBy,
		VDIOtherConfigKeyPVName:    volumeName,
		VDIOtherConfigKeyVolumeId:  volumeId.String(),
	}

	vdiParams := payloads.VDICreateParams{
		SRId:            srID,
		NameLabel:       diskName,
		VirtualSize:     capacityBytes,
		NameDescription: "VDI managed by the Kubernetes CSI",
		OtherConfig:     otherConfig,
	}
	if clusterTag != "" {
		vdiParams.Tags = []string{clusterTag}
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
	pbds, err := c.PBD().GetAll(ctx, 1, fmt.Sprintf("SR:%s host:%s", srID, hostID))
	if err != nil {
		return fmt.Errorf("failed to list PBDs for SR %s on host %s: %w", srID, hostID, err)
	}
	if len(pbds) == 0 {
		return fmt.Errorf("SR %s has no PBD for host %s; the SR may not be shared with this host", srID, hostID)
	}
	if !pbds[0].Attached {
		return fmt.Errorf("SR %s is not connected to host %s (PBD %s is unplugged)", srID, hostID, pbds[0].ID)
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
	filter := fmt.Sprintf("other_config:%s:%s", VDIOtherConfigKeyVolumeId, volumeId)
	vdis, err := c.VDI().GetAll(ctx, 2, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list VDIs by volume ID %s: %w", volumeId, err)
	}
	switch len(vdis) {
	case 0:
		return nil, ErrVolumeNotFound
	case 1:
		return vdis[0], nil
	default:
		return nil, fmt.Errorf("%w: volumeId=%s matched %d VDIs", ErrVolumeIdAmbiguous, volumeId, len(vdis))
	}
}

func (c xoClient) FindVDIByVolumeName(ctx context.Context, volumeName string) (*payloads.VDI, string, error) {
	filter := fmt.Sprintf("other_config:%s:%s", VDIOtherConfigKeyPVName, volumeName)
	vdis, err := c.VDI().GetAll(ctx, 2, filter)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list VDIs by volume name %s: %w", volumeName, err)
	}
	switch len(vdis) {
	case 0:
		return nil, "", ErrVolumeNotFound
	case 1:
		return vdis[0], vdis[0].OtherConfig[VDIOtherConfigKeyVolumeId], nil
	default:
		return nil, "", fmt.Errorf("%w: volumeName=%s matched %d VDIs", ErrVolumeNameAmbiguous, volumeName, len(vdis))
	}
}

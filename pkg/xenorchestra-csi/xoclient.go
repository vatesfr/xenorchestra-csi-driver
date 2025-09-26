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
	"fmt"
	"time"

	xoV1 "github.com/vatesfr/xenorchestra-go-sdk/client"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"

	"k8s.io/klog/v2"
)

// This interface extends the library.Library interface to add methods specific to Xen Orchestra operations needed by the CSI driver.
// It's done to encapsulate call to the legacy v1 client waiting for the v2 client to support all required operations.
type XoClient interface {
	library.Library
	GetVBDFromVDIAndVM(ctx context.Context, vdi xoV1.VDI, vmUUID string) (xoV1.VBD, error)
	ConnectVBDToVM(ctx context.Context, vbd xoV1.VBD) (xoV1.VBD, error)
	DisconnectVBDFromVM(ctx context.Context, vdi xoV1.VDI, vmUUID string) error
	AttachVDIToVM(ctx context.Context, vdi xoV1.VDI, vmUUID string) (xoV1.VBD, error)
	WaitForVDIToBeFullyAttached(ctx context.Context, vdi xoV1.VDI, vmUUID string) (xoV1.VBD, error)
	IsVDIUsedAnywhere(ctx context.Context, vdi xoV1.VDI) ([]xoV1.VBD, error)
	GetVDI(ctx context.Context, vdiUUID string) (xoV1.VDI, error)
}

type xoClient struct {
	library.Library
	v1Client *xoV1.Client
}

func NewXoClient(libraryService library.Library) XoClient {
	// HACK: use the old client to call the JRPC method directly
	rawV1Client := libraryService.V1Client().(*xoV1.Client)
	return xoClient{
		Library:  libraryService,
		v1Client: rawV1Client,
	}
}

func (c xoClient) GetVBDFromVDIAndVM(ctx context.Context, vdi xoV1.VDI, vmUUID string) (xoV1.VBD, error) {
	// TODO: Use the REST API to get the VBD
	var allVDBs map[string]xoV1.VBD
	err := c.V1Client().GetAllObjectsOfType(xoV1.VBD{}, &allVDBs)
	if err != nil {
		return xoV1.VBD{}, err
	}

	for _, vdb := range allVDBs {
		if vdb.VDI == vdi.VDIId && vdb.VmId == vmUUID {
			return vdb, nil
		}
	}
	return xoV1.VBD{}, fmt.Errorf("VBD not found for vdi=%s and vm=%s", vdi.VDIId, vmUUID)
}

func (c xoClient) ConnectVBDToVM(ctx context.Context, vbd xoV1.VBD) (xoV1.VBD, error) {
	err := c.V1Client().ConnectDisk(xoV1.Disk{
		VBD: vbd,
	})
	if err != nil {
		klog.ErrorS(err, "Failed to connect existing VBD to the node", "vbd", vbd)
		return xoV1.VBD{}, err
	}

	return vbd, nil
}

func (c xoClient) AttachVDIToVM(ctx context.Context, vdi xoV1.VDI, vmUUID string) (xoV1.VBD, error) {
	// TODO: Use the v2 client and REST API
	var result bool

	err := c.v1Client.Call("vm.attachDisk", map[string]interface{}{
		"mode": "RW", // TODO: change it if "ReadOnly" is required
		"vdi":  vdi.VDIId,
		"vm":   vmUUID,
	}, &result)
	if err != nil {
		return xoV1.VBD{}, err
	}

	if !result {
		return xoV1.VBD{}, fmt.Errorf("failed to attach VDI %s to VM %s: unknown error", vdi.VDIId, vmUUID)
	}

	// Wait for the attach operation to complete
	vbd, err := c.WaitForVDIToBeFullyAttached(ctx, vdi, vmUUID)
	if err != nil {
		klog.ErrorS(err, "Failed to wait for attach disk task", "vdi", vdi.VDIId, "vm", vmUUID)
		return xoV1.VBD{}, err
	}

	klog.V(4).InfoS("attachDiskToVM: disk attached", "vdi", vdi.VDIId, "vm", vmUUID, "vbd", vbd)

	return vbd, nil
}

// waitForDiskToBeAttached waits for the disk to be attached.
// This pools every 500ms to check if the disk is attached and if a device name is assigned.
// Hardcoded timeout of 2 minutes.
// NOTE: This is required because the VBD can be attached but returned without a device name when `vm.attach` command succeeded.
// See: https://github.com/vatesfr/xen-orchestra/pull/9192
func (c xoClient) WaitForVDIToBeFullyAttached(ctx context.Context, vdi xoV1.VDI, vmUUID string) (xoV1.VBD, error) {
	timeout := time.After(2 * time.Minute)
	tick := time.Tick(1 * time.Second)

	for {
		select {
		case <-timeout:
			return xoV1.VBD{}, fmt.Errorf("timed out waiting for VDI %s to be attached to VM %s", vdi.VDIId, vmUUID)
		case <-tick:
			vbd, err := c.GetVBDFromVDIAndVM(ctx, vdi, vmUUID)
			if err != nil {
				klog.ErrorS(err, "Failed to get VBD while waiting for disk to be attached", "vdi", vdi.VDIId, "vm", vmUUID)
				continue
			}
			if vbd.Attached && vbd.Device != "" {
				klog.V(4).InfoS("Disk is now attached", "vbd", vbd.Id, "vm", vmUUID, "device", vbd.Device)
				return vbd, nil
			}
			klog.V(5).InfoS("Disk not yet attached, waiting...", "vdi", vdi.VDIId, "vm", vmUUID)
		}
	}
}

// IsVDIUsedAnywhere checks if a VDI is used by any VM in the Xen Orchestra instance.
// If it is used, it returns the list of VBDs it is added to.
// If it is not used, it returns an empty slice.
func (c xoClient) IsVDIUsedAnywhere(ctx context.Context, vdi xoV1.VDI) ([]xoV1.VBD, error) {
	// TODO: Use v2 client and Rest API with filter: VDI:"<vdi-id>"
	var allVDBs map[string]xoV1.VBD
	err := c.V1Client().GetAllObjectsOfType(xoV1.VBD{}, &allVDBs)
	if err != nil {
		return nil, err
	}

	var vbds []xoV1.VBD
	for _, vdb := range allVDBs {
		if vdb.VDI == vdi.VDIId {
			vbds = append(vbds, vdb)
		}
	}
	return vbds, nil
}

func (c xoClient) GetVDI(ctx context.Context, vdiUUID string) (xoV1.VDI, error) {
	// TODO: Use v2 client and REST API
	return c.V1Client().GetVDI(xoV1.VDI{VDIId: vdiUUID})
}

func (c xoClient) DisconnectVBDFromVM(ctx context.Context, vdi xoV1.VDI, vmUUID string) error {
	vbd, err := c.GetVBDFromVDIAndVM(ctx, vdi, vmUUID)
	if err != nil {
		return err
	}
	return c.V1Client().DisconnectDisk(xoV1.Disk{
		VBD: vbd,
	})
}

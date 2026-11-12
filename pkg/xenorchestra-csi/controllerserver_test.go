/*
Copyright (c) 2026 Vates

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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/mock"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	libmock "github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library/mock"
)

func TestListVolumes(t *testing.T) {
	volumes := map[string][]string{"vol-b": {"vm-1"}, "vol-a": nil, "vol-c": {"vm-2"}}

	newDriver := func(t *testing.T, ret map[string][]string, err error) *xenorchestraCSIDriver {
		xo := mock.NewMockXoClient(gomock.NewController(t))
		xo.EXPECT().ListVolumes(gomock.Any(), "k8s-managed").Return(ret, err).AnyTimes()
		return &xenorchestraCSIDriver{xoClient: xo, clusterTag: "k8s-managed"}
	}
	entries := func(r *csi.ListVolumesResponse) map[string][]string {
		out := map[string][]string{}
		for _, e := range r.GetEntries() {
			out[e.GetVolume().GetVolumeId()] = e.GetStatus().GetPublishedNodeIds()
		}
		return out
	}

	t.Run("AllWithPublishedNodes", func(t *testing.T) {
		resp, err := newDriver(t, volumes, nil).ListVolumes(context.Background(), &csi.ListVolumesRequest{})
		require.NoError(t, err)
		assert.Equal(t, volumes, entries(resp))
		assert.Empty(t, resp.GetNextToken())
	})

	t.Run("Pagination", func(t *testing.T) {
		d := newDriver(t, volumes, nil)
		page1, err := d.ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: 2})
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{"vol-a": nil, "vol-b": {"vm-1"}}, entries(page1))
		require.Equal(t, "2", page1.GetNextToken())

		page2, err := d.ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: 2, StartingToken: page1.GetNextToken()})
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{"vol-c": {"vm-2"}}, entries(page2))
		assert.Empty(t, page2.GetNextToken())
	})

	for _, token := range []string{"x", "-1", "4"} {
		t.Run("BadToken_"+token, func(t *testing.T) {
			_, err := newDriver(t, volumes, nil).ListVolumes(context.Background(), &csi.ListVolumesRequest{StartingToken: token})
			assert.Equal(t, codes.Aborted, status.Code(err))
		})
	}

	t.Run("NegativeMaxEntries", func(t *testing.T) {
		_, err := newDriver(t, volumes, nil).ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: -1})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("XOErrorIsNotPartial", func(t *testing.T) {
		resp, err := newDriver(t, nil, errors.New("boom")).ListVolumes(context.Background(), &csi.ListVolumesRequest{})
		assert.Nil(t, resp)
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("CapabilitiesAdvertised", func(t *testing.T) {
		resp, err := newDriver(t, nil, nil).ControllerGetCapabilities(context.Background(), &csi.ControllerGetCapabilitiesRequest{})
		require.NoError(t, err)
		var got []csi.ControllerServiceCapability_RPC_Type
		for _, c := range resp.GetCapabilities() {
			got = append(got, c.GetRpc().GetType())
		}
		assert.Contains(t, got, csi.ControllerServiceCapability_RPC_LIST_VOLUMES)
		assert.Contains(t, got, csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES)
	})
}

func TestControllerPublishVolume(t *testing.T) {
	const tag, volID = "k8s-managed", "vol"
	// Default UUIDs. Each test can override them to simulate different scenarios.
	pool, host, sharedSR, localSR := uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4())
	node, otherVM := uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4())
	device := "xvdb"
	newVBD := func(vm uuid.UUID, attached bool) *payloads.VBD {
		vbd := &payloads.VBD{ID: uuid.Must(uuid.NewV4()), VM: vm, Attached: attached}
		if attached {
			vbd.Device = &device
		}
		return vbd
	}
	pluggedOther, unpluggedOther := newVBD(otherVM, true), newVBD(otherVM, false)
	pluggedSame, unpluggedSame := newVBD(node, true), newVBD(node, false)
	created := newVBD(node, true)

	// The VDI lacks the cluster tag and sits on the shared SR, so a local-storage publish
	// tags and migrates it unless the VBD scan stops or skips that first. gomock fails on
	// any XO call a case does not expect.
	tests := []struct {
		name        string
		storageType string
		otherPool   bool            // the node VM is in another pool than the VDI
		vbds        []*payloads.VBD // VBDs of the VDI returned by the scan
		wantCode    codes.Code
		wantErr     string        // substring of the error message
		wantMigrate bool          // VDI migrated to the node host's local SR
		wantConnect *payloads.VBD // existing VBD plugged with ConnectVBDToVM
		wantAttach  bool          // new VBD created with AttachVDIToVM (returns wantVBD)
		wantVBD     *payloads.VBD // VBD in the publish context
	}{
		{
			name:        "plugged on other VM: refused without side effects",
			storageType: StorageTypeLocal,
			vbds:        []*payloads.VBD{pluggedOther},
			wantCode:    codes.FailedPrecondition,
			wantErr:     otherVM.String(),
		},
		{
			name:        "pool mismatch: refused before the scan",
			storageType: StorageTypeLocal,
			otherPool:   true,
			wantCode:    codes.FailedPrecondition,
		},
		{
			name:       "unplugged on other VM: tolerated",
			vbds:       []*payloads.VBD{unpluggedOther},
			wantAttach: true,
			wantVBD:    created,
		},
		{
			name:        "unplugged on same VM: reconnected without migration",
			storageType: StorageTypeLocal,
			vbds:        []*payloads.VBD{unpluggedSame},
			wantConnect: unpluggedSame,
			wantVBD:     unpluggedSame,
		},
		{
			name:    "plugged on same VM: preferred over a stale one",
			vbds:    []*payloads.VBD{pluggedSame, unpluggedSame},
			wantVBD: pluggedSame,
		},
		{
			name:        "no VBD: migrated then attached",
			storageType: StorageTypeLocal,
			wantMigrate: true,
			wantAttach:  true,
			wantVBD:     created,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			xo, vmSvc, vdiSvc := mock.NewMockXoClient(ctrl), libmock.NewMockVM(ctrl), libmock.NewMockVDI(ctrl)
			xo.EXPECT().VM().Return(vmSvc).AnyTimes()
			xo.EXPECT().VDI().Return(vdiSvc).AnyTimes()

			vdi := &payloads.VDI{ID: uuid.Must(uuid.NewV4()), PoolID: pool, SR: sharedSR}
			vmPool := pool
			if tt.otherPool {
				vmPool = uuid.Must(uuid.NewV4())
			}
			xo.EXPECT().GetVDIByVolumeId(gomock.Any(), volID).Return(vdi, nil)
			vmSvc.EXPECT().GetByID(gomock.Any(), node).Return(&payloads.VM{ID: node, PoolID: vmPool, Container: host}, nil)
			if !tt.otherPool {
				xo.EXPECT().IsVDIUsedAnywhere(gomock.Any(), vdi).Return(tt.vbds, nil)
			}
			if tt.wantCode == codes.OK {
				vdiSvc.EXPECT().AddTag(gomock.Any(), vdi.ID, tag).Return(nil)
				target := vdi
				if tt.wantMigrate {
					target = &payloads.VDI{ID: uuid.Must(uuid.NewV4()), PoolID: pool, SR: localSR}
					xo.EXPECT().FindLocalSRForHost(gomock.Any(), host).Return(&payloads.StorageRepository{ID: localSR}, nil)
					xo.EXPECT().MigrateVDIAndWait(gomock.Any(), *vdi, localSR).Return(target.ID, nil)
					vdiSvc.EXPECT().Get(gomock.Any(), target.ID).Return(target, nil)
				}
				xo.EXPECT().IsSRAttachedToHost(gomock.Any(), target.SR, host).Return(nil)
				if tt.wantConnect != nil {
					connected := *tt.wantConnect
					connected.Attached = true
					connected.Device = &device
					xo.EXPECT().ConnectVBDToVM(gomock.Any(), *tt.wantConnect).Return(&connected, nil)
				}
				if tt.wantAttach {
					xo.EXPECT().AttachVDIToVM(gomock.Any(), *target, node).Return(tt.wantVBD, nil)
				}
			}

			d := &xenorchestraCSIDriver{xoClient: xo, clusterTag: tag}
			resp, err := d.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
				VolumeId: volID,
				NodeId:   node.String(),
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
					AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
				},
				VolumeContext: map[string]string{VolumeContextKeyStorageType: tt.storageType},
			})

			require.Equal(t, tt.wantCode, status.Code(err), "error: %v", err)
			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
			if tt.wantVBD != nil {
				assert.Equal(t, map[string]string{"device": device, "vbd": tt.wantVBD.ID.String()}, resp.GetPublishContext())
			}
		})
	}
}

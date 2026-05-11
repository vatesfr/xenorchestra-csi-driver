package sanity_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/gofrs/uuid"
	gomock "go.uber.org/mock/gomock"

	xenorchestracsi "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
	clientsMock "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/mock"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/stub"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	xoLibMock "github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library/mock"
)

// vdiStore is a package-level in-memory store used by mockVDI to simulate
// a VDI database during sanity tests without a real Xen Orchestra connection.
var vdiStore = struct {
	sync.RWMutex
	byID map[uuid.UUID]payloads.VDI
}{
	byID: make(map[uuid.UUID]payloads.VDI),
}

// NewFakeDriver creates a driver with a gomock XoClient and in-memory stubs
// for all other external dependencies. It is intended exclusively for use in
// tests. The returned MockXoClient can be used to set up additional
// expectations in individual test cases.
func NewFakeDriver(t *testing.T, options *xenorchestracsi.DriverOptions, fakeMounter *FakeMounter) (xenorchestracsi.Driver, *clientsMock.MockXoClient) {
	ctrl := gomock.NewController(t)

	mockPool := newMockPool(ctrl)
	mockVDI := newMockVDI(ctrl)
	mockVM := newMockVM(ctrl)
	mockSR := newMockSR(ctrl)

	mockXoClient := clientsMock.NewMockXoClient(ctrl)
	mockXoClient.EXPECT().Pool().Return(mockPool).AnyTimes()
	mockXoClient.EXPECT().VDI().Return(mockVDI).AnyTimes()
	mockXoClient.EXPECT().VM().Return(mockVM).AnyTimes()
	mockXoClient.EXPECT().SR().Return(mockSR).AnyTimes()

	mockXoClient.EXPECT().CreateNewVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, srID uuid.UUID, diskName string, capacityBytes int64, volumeName string, _ string, _ string) (uuid.UUID, uuid.UUID, error) {
			vdiID := uuid.Must(uuid.NewV4())
			volumeId := uuid.Must(uuid.NewV4())
			vdiStore.Lock()
			defer vdiStore.Unlock()
			vdiStore.byID[vdiID] = payloads.VDI{
				ID:        vdiID,
				SR:        srID,
				NameLabel: diskName,
				Size:      capacityBytes,
				OtherConfig: map[string]string{
					clients.VDIOtherConfigKeyPVName:   volumeName,
					clients.VDIOtherConfigKeyVolumeId: volumeId.String(),
				},
				PoolID: uuid.FromStringOrNil(stub.PoolId),
			}
			return vdiID, volumeId, nil
		}).AnyTimes()

	// IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error)
	mockXoClient.EXPECT().IsVDIUsedAnywhere(gomock.Any(), gomock.Any()).Return([]*payloads.VBD{}, nil).AnyTimes()
	mockXoClient.EXPECT().FindVDIByVolumeName(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, volumeName string) (*payloads.VDI, string, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()

		var matched *payloads.VDI
		for _, vdi := range vdiStore.byID {
			if vdi.OtherConfig[clients.VDIOtherConfigKeyPVName] != volumeName {
				continue
			}
			if matched != nil {
				return nil, "", fmt.Errorf("%w: volumeName=%s matched multiple VDIs", clients.ErrVolumeNameAmbiguous, volumeName)
			}
			vdiCopy := vdi
			matched = &vdiCopy
		}

		if matched == nil {
			return nil, "", clients.ErrVolumeNotFound
		}

		return matched, matched.OtherConfig[clients.VDIOtherConfigKeyVolumeId], nil
	}).AnyTimes()
	mockXoClient.EXPECT().GetVDIByVolumeId(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, volumeId string) (*payloads.VDI, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()

		var matched *payloads.VDI
		for _, vdi := range vdiStore.byID {
			if vdi.OtherConfig[clients.VDIOtherConfigKeyVolumeId] != volumeId {
				continue
			}
			if matched != nil {
				return nil, fmt.Errorf("%w: volumeId=%s matched multiple VDIs", clients.ErrVolumeIdAmbiguous, volumeId)
			}
			vdiCopy := vdi
			matched = &vdiCopy
		}

		if matched == nil {
			return nil, clients.ErrVolumeNotFound
		}

		return matched, nil
	}).AnyTimes()

	device := "/dev/xvdc"
	mockXoClient.EXPECT().AttachVDIToVM(gomock.Any(), gomock.Any(), gomock.Any()).Return(&payloads.VBD{
		ID:     uuid.Must(uuid.NewV4()),
		Device: &device,
	}, nil).AnyTimes()
	mockXoClient.EXPECT().DisconnectVBDFromVM(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockXoClient.EXPECT().IsSRAttachedToHost(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return xenorchestracsi.NewDriverWithDependencies(
		options,
		stub.NewNodeMetadataGetterStub(),
		mockXoClient,
		fakeMounter,
	), mockXoClient
}

func newMockVM(ctrl *gomock.Controller) *xoLibMock.MockVM {
	mockVM := xoLibMock.NewMockVM(ctrl)
	mockVM.EXPECT().GetByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (*payloads.VM, error) {
		// The NodeMetadataGetterStub returns a fixed NodeId and PoolId, so we can return a VM with matching IDs to simulate a valid node metadata scenario.
		if id == uuid.FromStringOrNil(stub.NodeId) {
			return &payloads.VM{
				ID:     id,
				PoolID: uuid.FromStringOrNil(stub.PoolId),
			}, nil
		}
		// Otherwise, simulate a "not found" error as the driver may query for non-existent VM IDs during tests.
		return nil, fmt.Errorf("API error: 404 Not Found - {\n  \"error\": \"no such VM %s\",\n  \"data\": {\n    \"id\": \"%s\",\n    \"type\": \"VM\"\n  }\n}", id, id)
	}).AnyTimes()
	return mockVM
}

func newMockPool(ctrl *gomock.Controller) *xoLibMock.MockPool {
	mockPool := xoLibMock.NewMockPool(ctrl)
	mockPool.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (*payloads.Pool, error) {
		if id == uuid.FromStringOrNil(stub.PoolId) {
			return &payloads.Pool{
				ID:        id,
				NameLabel: "fake-pool",
				DefaultSR: uuid.FromStringOrNil(stub.DefaultSRId),
			}, nil
		}
		return nil, fmt.Errorf("API error: 404 Not Found - {\n  \"error\": \"no such Pool %s\",\n  \"data\": {\n    \"id\": \"%s\",\n    \"type\": \"Pool\"\n  }\n}", id, id)
	}).AnyTimes()
	return mockPool
}

func newMockVDI(ctrl *gomock.Controller) *xoLibMock.MockVDI {
	mockVDI := xoLibMock.NewMockVDI(ctrl)
	mockVDI.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (*payloads.VDI, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()
		vdi, exists := vdiStore.byID[id]
		if !exists {
			return nil, fmt.Errorf("API error: 404 Not Found - {\n  \"error\": \"no such VDI %s\",\n  \"data\": {\n    \"id\": \"%s\",\n    \"type\": \"VDI\"\n  }\n}", id, id)
		}
		return &vdi, nil
	}).AnyTimes()
	mockVDI.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) error {
		vdiStore.Lock()
		defer vdiStore.Unlock()
		delete(vdiStore.byID, id)
		return nil
	}).AnyTimes()

	return mockVDI
}

func newMockSR(ctrl *gomock.Controller) *xoLibMock.MockSR {
	mockSR := xoLibMock.NewMockSR(ctrl)
	mockSR.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (*payloads.StorageRepository, error) {
		if id == uuid.FromStringOrNil(stub.DefaultSRId) {
			return &payloads.StorageRepository{
				ID:        id,
				NameLabel: "fake-sr",
				Type:      "nfs",
			}, nil
		}
		return nil, fmt.Errorf("API error: 404 Not Found - {\n  \"error\": \"no such SR %s\",\n  \"data\": {\n    \"id\": \"%s\",\n    \"type\": \"SR\"\n  }\n}", id, id)
	}).AnyTimes()
	return mockSR
}

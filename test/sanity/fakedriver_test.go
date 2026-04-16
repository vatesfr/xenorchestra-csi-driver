package sanity_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gofrs/uuid"
	gomock "go.uber.org/mock/gomock"

	xenorchestracsi "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi"
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
func NewFakeDriver(t *testing.T, options *xenorchestracsi.DriverOptions, fakeMounter *stub.StubMounter) (xenorchestracsi.Driver, *clientsMock.MockXoClient) {
	ctrl := gomock.NewController(t)

	mockPool := newMockPool(ctrl)
	mockVDI := newMockVDI(ctrl)
	mockVM := newMockVM(ctrl)

	mockXoClient := clientsMock.NewMockXoClient(ctrl)
	mockXoClient.EXPECT().Pool().Return(mockPool).AnyTimes()
	mockXoClient.EXPECT().VDI().Return(mockVDI).AnyTimes()
	mockXoClient.EXPECT().VM().Return(mockVM).AnyTimes()

	// IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error)
	mockXoClient.EXPECT().IsVDIUsedAnywhere(gomock.Any(), gomock.Any()).Return([]*payloads.VBD{}, nil).AnyTimes()
	device := "/dev/xvdc"
	mockXoClient.EXPECT().AttachVDIToVM(gomock.Any(), gomock.Any(), gomock.Any()).Return(&payloads.VBD{
		ID:     uuid.Must(uuid.NewV4()),
		Device: &device,
	}, nil).AnyTimes()
	mockXoClient.EXPECT().DisconnectVBDFromVM(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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
				DefaultSR: uuid.Must(uuid.NewV4()),
			}, nil
		}
		return nil, fmt.Errorf("API error: 404 Not Found - {\n  \"error\": \"no such Pool %s\",\n  \"data\": {\n    \"id\": \"%s\",\n    \"type\": \"Pool\"\n  }\n}", id, id)
	}).AnyTimes()
	return mockPool
}

func newMockVDI(ctrl *gomock.Controller) *xoLibMock.MockVDI {
	mockVDI := xoLibMock.NewMockVDI(ctrl)
	mockVDI.EXPECT().GetAll(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ int, filters string) ([]*payloads.VDI, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()

		// Parse "other_config:VDIOtherConfigKeyPVName:<value>" filter
		prefix := fmt.Sprintf("other_config:%s:", xenorchestracsi.VDIOtherConfigKeyPVName)
		filterValue, hasFilter := strings.CutPrefix(filters, prefix)
		vdis := make([]*payloads.VDI, 0, len(vdiStore.byID))
		for _, vdi := range vdiStore.byID {
			if hasFilter && strings.Compare(vdi.OtherConfig[xenorchestracsi.VDIOtherConfigKeyPVName], filterValue) != 0 {
				continue
			}
			vdis = append(vdis, &vdi)
		}
		return vdis, nil
	}).AnyTimes()
	mockVDI.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (*payloads.VDI, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()
		vdi, exists := vdiStore.byID[id]
		if !exists {
			return nil, fmt.Errorf("API error: 404 Not Found - {\n  \"error\": \"no such VDI %s\",\n  \"data\": {\n    \"id\": \"%s\",\n    \"type\": \"VDI\"\n  }\n}", id, id)
		}
		return &vdi, nil
	}).AnyTimes()
	mockVDI.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, p payloads.VDICreateParams) (uuid.UUID, error) {
		id := uuid.Must(uuid.NewV4())
		vdiStore.Lock()
		defer vdiStore.Unlock()
		vdiStore.byID[id] = payloads.VDI{
			ID:              id,
			SR:              p.SRId,
			NameLabel:       p.NameLabel,
			Size:            p.VirtualSize,
			NameDescription: p.NameDescription,
			OtherConfig:     p.OtherConfig,
			PoolID:          uuid.FromStringOrNil(stub.PoolId),
		}
		return id, nil
	}).AnyTimes()
	mockVDI.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) error {
		vdiStore.Lock()
		defer vdiStore.Unlock()
		delete(vdiStore.byID, id)
		return nil
	}).AnyTimes()

	return mockVDI
}

package sanity_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gofrs/uuid"
	xenorchestracsi "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi"
	clientsMock "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/mock"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/stub"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	xoLibMock "github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library/mock"
	gomock "go.uber.org/mock/gomock"
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
func NewFakeDriver(t *testing.T, options *xenorchestracsi.DriverOptions) (xenorchestracsi.Driver, *clientsMock.MockXoClient) {
	ctrl := gomock.NewController(t)

	mockPool := newMockPool(ctrl)
	mockVDI := newMockVDI(ctrl)

	mockXoClient := clientsMock.NewMockXoClient(ctrl)
	mockXoClient.EXPECT().Pool().Return(mockPool).AnyTimes()
	mockXoClient.EXPECT().VDI().Return(mockVDI).AnyTimes()

	// IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error)
	mockXoClient.EXPECT().IsVDIUsedAnywhere(gomock.Any(), gomock.Any()).Return([]*payloads.VBD{}, nil).AnyTimes()

	return xenorchestracsi.NewDriverWithDependencies(
		options,
		stub.NewNodeMetadataGetterStub(),
		mockXoClient,
		stub.NewStubMounter(),
	), mockXoClient
}

func newMockPool(ctrl *gomock.Controller) *xoLibMock.MockPool {
	mockPool := xoLibMock.NewMockPool(ctrl)
	mockPool.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&payloads.Pool{
		ID:        uuid.Must(uuid.NewV4()),
		NameLabel: "fake-pool",
		DefaultSR: uuid.Must(uuid.NewV4()),
	}, nil).AnyTimes()
	return mockPool
}

func newMockVDI(ctrl *gomock.Controller) *xoLibMock.MockVDI {
	mockVDI := xoLibMock.NewMockVDI(ctrl)
	mockVDI.EXPECT().GetAll(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ int, filters string) ([]payloads.VDI, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()

		// Parse "other_config:VDIOtherConfigKeyPVName:<value>" filter
		prefix := fmt.Sprintf("other_config:%s:", xenorchestracsi.VDIOtherConfigKeyPVName)
		filterValue, hasFilter := strings.CutPrefix(filters, prefix)

		vdis := make([]payloads.VDI, 0, len(vdiStore.byID))
		for _, vdi := range vdiStore.byID {
			if hasFilter && vdi.OtherConfig[xenorchestracsi.VDIOtherConfigKeyPVName] != filterValue {
				continue
			}
			vdis = append(vdis, vdi)
		}
		return vdis, nil
	}).AnyTimes()
	mockVDI.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (*payloads.VDI, error) {
		vdiStore.RLock()
		defer vdiStore.RUnlock()
		vdi, exists := vdiStore.byID[id]
		if !exists {
			return nil, fmt.Errorf("VDI not found")
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

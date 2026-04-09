// Copyright (c) 2025 Vates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// this file has been written by Copilot with no review.
// TODO: add real unit tests.

//go:build unit

package xenorchestracsi

import (
	"context"
	"errors"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gofrs/uuid"

	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"
	xok8s "github.com/vatesfr/xenorchestra-k8s-common"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// Minimal fakes — embed the interface to zero-implement unused methods.
// Any test that triggers an un-overridden method will panic, which surfaces
// unexpected calls clearly.
// ---------------------------------------------------------------------------

type fakePool struct {
	library.Pool
	pool *payloads.Pool
	err  error
}

func (f *fakePool) Get(_ context.Context, _ uuid.UUID) (*payloads.Pool, error) {
	return f.pool, f.err
}

type fakeVDI struct {
	library.VDI
	// Create
	id             uuid.UUID
	err            error
	capturedParams payloads.VDICreateParams
	// Get
	getResult *payloads.VDI
	getErr    error
	// GetAll
	getAllResult []*payloads.VDI
	getAllErr    error
	// Delete
	deleteErr error
	// AddTag
	addTagCalled bool
	addTagID     uuid.UUID
	addTagTag    string
	addTagErr    error
}

func (f *fakeVDI) Create(_ context.Context, p payloads.VDICreateParams) (uuid.UUID, error) {
	f.capturedParams = p
	return f.id, f.err
}

func (f *fakeVDI) Get(_ context.Context, _ uuid.UUID) (*payloads.VDI, error) {
	return f.getResult, f.getErr
}

func (f *fakeVDI) GetAll(_ context.Context, _ int, _ string) ([]*payloads.VDI, error) {
	return f.getAllResult, f.getAllErr
}

func (f *fakeVDI) Delete(_ context.Context, _ uuid.UUID) error {
	return f.deleteErr
}

func (f *fakeVDI) AddTag(_ context.Context, id uuid.UUID, tag string) error {
	f.addTagCalled = true
	f.addTagID = id
	f.addTagTag = tag
	return f.addTagErr
}

// fakeVBD implements library.VBD for testing ListVolumes published nodes.
type fakeVBD struct {
	library.VBD
	getAllResult []*payloads.VBD
	getAllErr    error
}

func (f *fakeVBD) GetAll(_ context.Context, _ int, _ string) ([]*payloads.VBD, error) {
	return f.getAllResult, f.getAllErr
}

// fakeSR implements library.SR for testing volume condition reporting.
type fakeSR struct {
	library.SR
	getResult    *payloads.StorageRepository
	getErr       error
	getAllResult []*payloads.StorageRepository
	getAllErr    error
}

func (f *fakeSR) Get(_ context.Context, _ uuid.UUID) (*payloads.StorageRepository, error) {
	return f.getResult, f.getErr
}

func (f *fakeSR) GetAll(_ context.Context, _ int, _ string) ([]*payloads.StorageRepository, error) {
	return f.getAllResult, f.getAllErr
}

// fakeVM implements library.VM for testing ControllerPublishVolume and PBD checks.
// getByIDResults allows returning different VMs per UUID; if nil, getByIDResult is used.
type fakeVM struct {
	library.VM
	getByIDResult  *payloads.VM
	getByIDErr     error
	getByIDResults map[uuid.UUID]*payloads.VM
}

func (f *fakeVM) GetByID(_ context.Context, id uuid.UUID) (*payloads.VM, error) {
	if f.getByIDResults != nil {
		if vm, ok := f.getByIDResults[id]; ok {
			return vm, f.getByIDErr
		}
		return nil, f.getByIDErr
	}
	return f.getByIDResult, f.getByIDErr
}

// fakePBD implements library.PBD for testing PBD connectivity checks.
type fakePBD struct {
	library.PBD
	getAllResult []*payloads.PBD
	getAllErr    error
}

func (f *fakePBD) GetAll(_ context.Context, _ int, _ string) ([]*payloads.PBD, error) {
	return f.getAllResult, f.getAllErr
}

// fakeXoClient only overrides Pool(), VDI(), VBD(), SR(), VM(), PBD(), IsVDIUsedAnywhere(), and
// AttachVDIToVM(); all other calls panic.
type fakeXoClient struct {
	XoClient
	pool            *fakePool
	vdi             *fakeVDI
	vbd             *fakeVBD
	sr              *fakeSR
	vm              *fakeVM
	pbd             *fakePBD
	isUsedResult    []*payloads.VBD
	isUsedErr       error
	attachVBDResult *payloads.VBD
	attachVBDErr    error
}

func (f *fakeXoClient) Pool() library.Pool { return f.pool }
func (f *fakeXoClient) VDI() library.VDI   { return f.vdi }
func (f *fakeXoClient) VBD() library.VBD   { return f.vbd }
func (f *fakeXoClient) SR() library.SR     { return f.sr }
func (f *fakeXoClient) VM() library.VM     { return f.vm }
func (f *fakeXoClient) PBD() library.PBD   { return f.pbd }
func (f *fakeXoClient) IsVDIUsedAnywhere(_ context.Context, _ *payloads.VDI) ([]*payloads.VBD, error) {
	return f.isUsedResult, f.isUsedErr
}
func (f *fakeXoClient) AttachVDIToVM(_ context.Context, _ payloads.VDI, _ uuid.UUID) (*payloads.VBD, error) {
	return f.attachVBDResult, f.attachVBDErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var (
	validPoolUUID = uuid.Must(uuid.FromString("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"))
	validSRUUID   = uuid.Must(uuid.FromString("11111111-2222-3333-4444-555555555555"))
	validVDIUUID  = uuid.Must(uuid.FromString("66666666-7777-8888-9999-aaaaaaaaaaaa"))
)

func validCreateVolumeRequest(poolIDStr string) *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{
		Name: "pvc-test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1 << 30},
		Parameters:    map[string]string{ParameterPoolID: poolIDStr},
	}
}

func grpcCode(err error) codes.Code {
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}
	return codes.Unknown
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateVolume_PoolIDMissing(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	req := &csi.CreateVolumeRequest{
		Name: "pvc-test",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1 << 30},
		Parameters:    map[string]string{},
	}

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateVolume_PoolIDEmpty(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	req := validCreateVolumeRequest("")

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateVolume_PoolIDMalformed(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	req := validCreateVolumeRequest("not-a-uuid")

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateVolume_PoolIDIsZeroUUID(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	req := validCreateVolumeRequest(uuid.Nil.String())

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCreateVolume_PoolNotFound(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			pool: &fakePool{err: errors.New("pool not found")},
			vdi:  &fakeVDI{},
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestCreateVolume_PoolHasNoDefaultSR(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: uuid.Nil}},
			vdi:  &fakeVDI{},
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", err)
	}
}

func TestCreateVolume_VDICreateFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: validSRUUID}},
			vdi:  &fakeVDI{err: errors.New("storage full")},
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	_, err := driver.CreateVolume(context.Background(), req)
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestCreateVolume_Success(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: validSRUUID}},
			vdi:  &fakeVDI{id: validVDIUUID},
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	resp, err := driver.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vol := resp.GetVolume()
	if vol.VolumeId != validVDIUUID.String() {
		t.Errorf("VolumeId: got %q, want %q", vol.VolumeId, validVDIUUID.String())
	}
	if vol.CapacityBytes != 1<<30 {
		t.Errorf("CapacityBytes: got %d, want %d", vol.CapacityBytes, int64(1<<30))
	}

	topo := vol.GetAccessibleTopology()
	if len(topo) != 1 {
		t.Fatalf("expected 1 topology segment, got %d", len(topo))
	}
	seg := topo[0].GetSegments()
	if got := seg[xok8s.XOLabelTopologyPoolID]; got != validPoolUUID.String() {
		t.Errorf("topology pool: got %q, want %q", got, validPoolUUID.String())
	}
}

func TestCreateVolume_DefaultVDINamePrefix(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{id: validVDIUUID}
	driver := &xenorchestraCSIDriver{
		vdiNamePrefix: DefaultVDINamePrefix,
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: validSRUUID}},
			vdi:  fv,
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	if _, err := driver.CreateVolume(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := DefaultVDINamePrefix + req.GetName()
	if fv.capturedParams.NameLabel != want {
		t.Errorf("VDI NameLabel: got %q, want %q", fv.capturedParams.NameLabel, want)
	}
}

func TestCreateVolume_CustomVDINamePrefix(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{id: validVDIUUID}
	driver := &xenorchestraCSIDriver{
		vdiNamePrefix: "mycluster-",
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: validSRUUID}},
			vdi:  fv,
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	if _, err := driver.CreateVolume(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "mycluster-" + req.GetName()
	if fv.capturedParams.NameLabel != want {
		t.Errorf("VDI NameLabel: got %q, want %q", fv.capturedParams.NameLabel, want)
	}
}

// ---------------------------------------------------------------------------
// DeleteVolume tests
// ---------------------------------------------------------------------------

func TestDeleteVolume_VolumeIDMissing(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: ""})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestDeleteVolume_VolumeIDMalformed(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: "not-a-uuid"})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestDeleteVolume_VolumeIDIsZeroUUID(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: uuid.Nil.String()})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestDeleteVolume_VolumeNotFound(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getErr: errors.New("API error: 404 Not Found - no such VDI")},
		},
	}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Errorf("expected success (idempotent), got %v", err)
	}
}

func TestDeleteVolume_GetVDIFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getErr: errors.New("connection refused")},
		},
	}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestDeleteVolume_VolumeStillAttached(t *testing.T) {
	t.Parallel()

	vmUUID := uuid.Must(uuid.FromString("cccccccc-dddd-eeee-ffff-000000000000"))
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi:          &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID}},
			isUsedResult: []*payloads.VBD{{VM: vmUUID, Attached: true}},
		},
	}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", err)
	}
}

func TestDeleteVolume_DeleteFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi:          &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID}, deleteErr: errors.New("storage error")},
			isUsedResult: nil,
		},
	}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestDeleteVolume_Success(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi:          &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID}},
			isUsedResult: nil,
		},
	}
	_, err := driver.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListVolumes tests
// ---------------------------------------------------------------------------

var (
	validVDIUUID2 = uuid.Must(uuid.FromString("77777777-8888-9999-aaaa-bbbbbbbbbbbb"))
	validVDIUUID3 = uuid.Must(uuid.FromString("88888888-9999-aaaa-bbbb-cccccccccccc"))
	validVMUUID   = uuid.Must(uuid.FromString("cccccccc-dddd-eeee-ffff-000000000000"))
	validHostUUID = uuid.Must(uuid.FromString("aaaabbbb-cccc-dddd-eeee-ffff00001111"))
	validPBDUUID  = uuid.Must(uuid.FromString("11112222-3333-4444-5555-666677778888"))
)

func TestListVolumes_Empty(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: nil},
			vbd: &fakeVBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetEntries()) != 0 {
		t.Errorf("expected 0 entries, got %d", len(resp.GetEntries()))
	}
	if resp.GetNextToken() != "" {
		t.Errorf("expected empty NextToken, got %q", resp.GetNextToken())
	}
}

func TestListVolumes_Success(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30},
		{ID: validVDIUUID2, Size: 2 << 30},
	}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{},
			pbd: &fakePBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetEntries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.GetEntries()))
	}
	if resp.GetEntries()[0].GetVolume().GetVolumeId() != validVDIUUID.String() {
		t.Errorf("entry[0] VolumeId: got %q, want %q", resp.GetEntries()[0].GetVolume().GetVolumeId(), validVDIUUID.String())
	}
	if resp.GetEntries()[1].GetVolume().GetCapacityBytes() != 2<<30 {
		t.Errorf("entry[1] CapacityBytes: got %d, want %d", resp.GetEntries()[1].GetVolume().GetCapacityBytes(), int64(2<<30))
	}
	if resp.GetNextToken() != "" {
		t.Errorf("expected empty NextToken, got %q", resp.GetNextToken())
	}
}

func TestListVolumes_Pagination(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30},
		{ID: validVDIUUID2, Size: 2 << 30},
		{ID: validVDIUUID3, Size: 3 << 30},
	}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{},
			pbd: &fakePBD{},
		},
	}

	// First page: MaxEntries=2
	resp1, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: 2})
	if err != nil {
		t.Fatalf("page1: unexpected error: %v", err)
	}
	if len(resp1.GetEntries()) != 2 {
		t.Fatalf("page1: expected 2 entries, got %d", len(resp1.GetEntries()))
	}
	if resp1.GetNextToken() != "2" {
		t.Errorf("page1: expected NextToken=2, got %q", resp1.GetNextToken())
	}

	// Second page: use NextToken from first page
	resp2, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{
		MaxEntries:    2,
		StartingToken: resp1.GetNextToken(),
	})
	if err != nil {
		t.Fatalf("page2: unexpected error: %v", err)
	}
	if len(resp2.GetEntries()) != 1 {
		t.Fatalf("page2: expected 1 entry, got %d", len(resp2.GetEntries()))
	}
	if resp2.GetEntries()[0].GetVolume().GetVolumeId() != validVDIUUID3.String() {
		t.Errorf("page2: entry[0] VolumeId: got %q, want %q", resp2.GetEntries()[0].GetVolume().GetVolumeId(), validVDIUUID3.String())
	}
	if resp2.GetNextToken() != "" {
		t.Errorf("page2: expected empty NextToken, got %q", resp2.GetNextToken())
	}
}

func TestListVolumes_InvalidStartingToken(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: nil},
			vbd: &fakeVBD{},
		},
	}
	_, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{StartingToken: "notanumber"})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListVolumes_NegativeStartingToken(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: nil},
			vbd: &fakeVBD{},
		},
	}
	_, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{StartingToken: "-1"})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListVolumes_GetAllVDIsFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllErr: errors.New("connection refused")},
			vbd: &fakeVBD{},
		},
	}
	_, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestListVolumes_GetAllVBDsFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30}}},
			vbd: &fakeVBD{getAllErr: errors.New("network error")},
			sr:  &fakeSR{},
			pbd: &fakePBD{},
		},
	}
	_, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestListVolumes_PublishedNodes(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID},
	}
	vbds := []*payloads.VBD{
		{VM: validVMUUID, Attached: true},
	}
	pbds := []*payloads.PBD{
		{ID: validPBDUUID, SR: validSRUUID, Host: validHostUUID, Attached: true},
	}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: vbds},
			sr:  &fakeSR{getAllResult: []*payloads.StorageRepository{{ID: validSRUUID}}},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: pbds},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetEntries()) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.GetEntries()))
	}
	nodeIDs := resp.GetEntries()[0].GetStatus().GetPublishedNodeIds()
	if len(nodeIDs) != 1 {
		t.Fatalf("expected 1 published node ID, got %d", len(nodeIDs))
	}
	if nodeIDs[0] != validVMUUID.String() {
		t.Errorf("PublishedNodeIds[0]: got %q, want %q", nodeIDs[0], validVMUUID.String())
	}
}

func TestListVolumes_DetachedVBDNotPublished(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30},
	}
	// The "attached?" server-side filter excludes detached VBDs, so the fake returns nil.
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{},
			pbd: &fakePBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nodeIDs := resp.GetEntries()[0].GetStatus().GetPublishedNodeIds()
	if len(nodeIDs) != 0 {
		t.Errorf("expected 0 published node IDs for detached VBD, got %d: %v", len(nodeIDs), nodeIDs)
	}
}

func TestListVolumes_SRGetAllFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30}}},
			vbd: &fakeVBD{},
			sr:  &fakeSR{getAllErr: errors.New("SR service unavailable")},
			pbd: &fakePBD{},
		},
	}
	_, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestListVolumes_VolumeConditionHealthy(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID},
	}
	srs := []*payloads.StorageRepository{
		{ID: validSRUUID, InMaintenanceMode: false},
	}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{getAllResult: srs},
			pbd: &fakePBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if cond.GetAbnormal() {
		t.Errorf("expected Abnormal=false for healthy SR, got true")
	}
}

func TestListVolumes_VolumeConditionSRMaintenance(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID},
	}
	srs := []*payloads.StorageRepository{
		{ID: validSRUUID, InMaintenanceMode: true},
	}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{getAllResult: srs},
			pbd: &fakePBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true for SR in maintenance mode")
	}
}

func TestListVolumes_VolumeConditionSRNotFound(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{
		{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID},
	}
	// SR list does not contain validSRUUID → condition must be abnormal
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{getAllResult: nil},
			pbd: &fakePBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true for SR not found in SR list")
	}
}

// ---------------------------------------------------------------------------
// ControllerGetVolume tests
// ---------------------------------------------------------------------------

func TestControllerGetVolume_VolumeIDMissing(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: ""})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestControllerGetVolume_VolumeIDMalformed(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: "not-a-uuid"})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestControllerGetVolume_VolumeIDZero(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{xoClient: &fakeXoClient{}}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: uuid.Nil.String()})
	if grpcCode(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestControllerGetVolume_VolumeNotFound(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getErr: errors.New("API error: 404 Not Found - no such VDI")},
		},
	}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestControllerGetVolume_VDIGetFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getErr: errors.New("connection refused")},
		},
	}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestControllerGetVolume_SRGetFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getErr: errors.New("SR service unavailable")},
			vbd: &fakeVBD{},
		},
	}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestControllerGetVolume_SRNotFound(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getErr: errors.New("API error: 404 Not Found - SR not found")},
			vbd: &fakeVBD{getAllResult: nil},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true for SR not found")
	}
}

func TestControllerGetVolume_SRInMaintenanceMode(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID, InMaintenanceMode: true}},
			vbd: &fakeVBD{getAllResult: nil},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true for SR in maintenance mode")
	}
}

func TestControllerGetVolume_VBDGetFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllErr: errors.New("network error")},
		},
	}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestControllerGetVolume_Success(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID, InMaintenanceMode: false}},
			vbd: &fakeVBD{getAllResult: nil},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetVolume().GetVolumeId() != validVDIUUID.String() {
		t.Errorf("VolumeId: got %q, want %q", resp.GetVolume().GetVolumeId(), validVDIUUID.String())
	}
	if resp.GetVolume().GetCapacityBytes() != 1<<30 {
		t.Errorf("CapacityBytes: got %d, want %d", resp.GetVolume().GetCapacityBytes(), int64(1<<30))
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if cond.GetAbnormal() {
		t.Errorf("expected Abnormal=false for healthy volume")
	}
}

func TestControllerGetVolume_PublishedNodes(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{
		{VM: validVMUUID, Attached: true},
	}
	pbds := []*payloads.PBD{
		{ID: validPBDUUID, SR: validSRUUID, Host: validHostUUID, Attached: true},
	}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: pbds},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nodeIDs := resp.GetStatus().GetPublishedNodeIds()
	if len(nodeIDs) != 1 {
		t.Fatalf("expected 1 published node ID, got %d", len(nodeIDs))
	}
	if nodeIDs[0] != validVMUUID.String() {
		t.Errorf("PublishedNodeIds[0]: got %q, want %q", nodeIDs[0], validVMUUID.String())
	}
}

// ---------------------------------------------------------------------------
// Cluster-tag tests
// ---------------------------------------------------------------------------

func TestCreateVolume_ClusterTagAdded(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{id: validVDIUUID}
	driver := &xenorchestraCSIDriver{
		clusterTag: DefaultClusterTag,
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: validSRUUID}},
			vdi:  fv,
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	if _, err := driver.CreateVolume(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fv.capturedParams.Tags) != 1 || fv.capturedParams.Tags[0] != DefaultClusterTag {
		t.Errorf("Tags: got %v, want [%q]", fv.capturedParams.Tags, DefaultClusterTag)
	}
}

func TestCreateVolume_NoClusterTagWhenEmpty(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{id: validVDIUUID}
	driver := &xenorchestraCSIDriver{
		clusterTag: "", // disabled
		xoClient: &fakeXoClient{
			pool: &fakePool{pool: &payloads.Pool{ID: validPoolUUID, DefaultSR: validSRUUID}},
			vdi:  fv,
		},
	}
	req := validCreateVolumeRequest(validPoolUUID.String())

	if _, err := driver.CreateVolume(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fv.capturedParams.Tags) != 0 {
		t.Errorf("Tags: got %v, want nil/empty (no tag when clusterTag='')", fv.capturedParams.Tags)
	}
}

// ---------------------------------------------------------------------------
// ControllerPublishVolume – cluster tag adoption tests
// ---------------------------------------------------------------------------

// validPublishRequest returns a minimal valid ControllerPublishVolumeRequest.
func validPublishRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId: validVDIUUID.String(),
		NodeId:   validVMUUID.String(),
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
}

// publishClient builds a fakeXoClient wired for a successful ControllerPublishVolume path.
// fv is the fakeVDI under test; vdiTags are set on the VDI returned by Get.
func publishClient(fv *fakeVDI, vdiTags []string) *fakeXoClient {
	device := "xvda"
	fv.getResult = &payloads.VDI{ID: validVDIUUID, PoolID: validPoolUUID, Tags: vdiTags}
	return &fakeXoClient{
		vdi: fv,
		vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, PoolID: validPoolUUID}},
		attachVBDResult: &payloads.VBD{
			ID:       validVDIUUID,
			VM:       validVMUUID,
			Device:   &device,
			Attached: true,
		},
	}
}

func TestControllerPublishVolume_ClusterTagAddedToUntaggedVDI(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{}
	driver := &xenorchestraCSIDriver{
		clusterTag: DefaultClusterTag,
		xoClient:   publishClient(fv, nil /* no tags */),
	}
	if _, err := driver.ControllerPublishVolume(context.Background(), validPublishRequest()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fv.addTagCalled {
		t.Error("expected AddTag to be called for untagged VDI")
	}
	if fv.addTagTag != DefaultClusterTag {
		t.Errorf("AddTag tag: got %q, want %q", fv.addTagTag, DefaultClusterTag)
	}
	if fv.addTagID != validVDIUUID {
		t.Errorf("AddTag id: got %q, want %q", fv.addTagID, validVDIUUID)
	}
}

func TestControllerPublishVolume_ClusterTagSkippedWhenAlreadyPresent(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{}
	driver := &xenorchestraCSIDriver{
		clusterTag: DefaultClusterTag,
		xoClient:   publishClient(fv, []string{DefaultClusterTag}),
	}
	if _, err := driver.ControllerPublishVolume(context.Background(), validPublishRequest()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fv.addTagCalled {
		t.Error("expected AddTag NOT to be called when tag is already present")
	}
}

func TestControllerPublishVolume_ClusterTagSkippedWhenDisabled(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{}
	driver := &xenorchestraCSIDriver{
		clusterTag: "", // disabled
		xoClient:   publishClient(fv, nil),
	}
	if _, err := driver.ControllerPublishVolume(context.Background(), validPublishRequest()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fv.addTagCalled {
		t.Error("expected AddTag NOT to be called when clusterTag is empty")
	}
}

func TestControllerPublishVolume_ClusterTagAddTagFails(t *testing.T) {
	t.Parallel()

	fv := &fakeVDI{addTagErr: errors.New("XO API unavailable")}
	driver := &xenorchestraCSIDriver{
		clusterTag: DefaultClusterTag,
		xoClient:   publishClient(fv, nil /* untagged → AddTag will be attempted */),
	}
	_, err := driver.ControllerPublishVolume(context.Background(), validPublishRequest())
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal when AddTag fails, got %v", err)
	}
}

func TestListVolumes_OnlyTaggedVolumesReturned(t *testing.T) {
	t.Parallel()

	const tag = "k8s-prod"
	taggedVDI := &payloads.VDI{ID: validVDIUUID, Size: 1 << 30, Tags: []string{tag}}
	untaggedVDI := &payloads.VDI{ID: validVDIUUID2, Size: 2 << 30, Tags: nil}

	driver := &xenorchestraCSIDriver{
		clusterTag: tag,
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: []*payloads.VDI{taggedVDI, untaggedVDI}},
			vbd: &fakeVBD{getAllResult: nil},
			sr:  &fakeSR{getAllResult: nil},
			pbd: &fakePBD{},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetEntries()) != 1 {
		t.Fatalf("expected 1 entry (only tagged VDI), got %d", len(resp.GetEntries()))
	}
	if resp.GetEntries()[0].GetVolume().GetVolumeId() != validVDIUUID.String() {
		t.Errorf("entry[0] VolumeId: got %q, want %q",
			resp.GetEntries()[0].GetVolume().GetVolumeId(), validVDIUUID.String())
	}
}

// ---------------------------------------------------------------------------
// PBD health check tests — ControllerGetVolume
// ---------------------------------------------------------------------------

// TestControllerGetVolume_PBDHealthy verifies that a published volume on a host
// with an attached PBD reports a healthy condition.
func TestControllerGetVolume_PBDHealthy(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	pbds := []*payloads.PBD{{ID: validPBDUUID, SR: validSRUUID, Host: validHostUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: pbds},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if cond.GetAbnormal() {
		t.Errorf("expected Abnormal=false when PBD is attached, got true (message: %q)", cond.GetMessage())
	}
}

// TestControllerGetVolume_PBDMissing verifies that a published volume whose host
// has no PBD for the SR reports an abnormal condition.
func TestControllerGetVolume_PBDMissing(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	// No PBD for the SR at all.
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: nil},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true when PBD is missing for host")
	}
}

// TestControllerGetVolume_PBDDetached verifies that a published volume whose host
// has a PBD record but it is not attached (disconnected) reports an abnormal condition.
func TestControllerGetVolume_PBDDetached(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	// PBD exists but is not attached.
	pbds := []*payloads.PBD{{ID: validPBDUUID, SR: validSRUUID, Host: validHostUUID, Attached: false}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: pbds},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true when PBD is detached")
	}
}

// TestControllerGetVolume_PBDSkippedForHaltedVM verifies that a published volume
// whose VM is halted (Container == uuid.Nil) skips the PBD check and remains healthy.
func TestControllerGetVolume_PBDSkippedForHaltedVM(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	// VM is halted: Container is uuid.Nil.
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: uuid.Nil}},
			pbd: &fakePBD{getAllResult: nil},
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if cond.GetAbnormal() {
		t.Errorf("expected Abnormal=false for halted VM (PBD check skipped), got true (message: %q)", cond.GetMessage())
	}
}

// TestControllerGetVolume_PBDSkippedWhenNoPublishedNodes verifies that a volume
// with no published nodes skips the PBD check entirely.
func TestControllerGetVolume_PBDSkippedWhenNoPublishedNodes(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: nil}, // no published nodes
			// vm and pbd are not set; if called they would panic.
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if cond.GetAbnormal() {
		t.Errorf("expected Abnormal=false when no published nodes, got true")
	}
}

// TestControllerGetVolume_PBDGetFails verifies that a PBD lookup failure returns Internal.
func TestControllerGetVolume_PBDGetFails(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllErr: errors.New("PBD service unavailable")},
		},
	}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal when PBD fetch fails, got %v", err)
	}
}

// TestControllerGetVolume_VMGetFailsForPBDCheck verifies that a VM lookup failure
// during PBD check returns Internal.
func TestControllerGetVolume_VMGetFailsForPBDCheck(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID}},
			vbd: &fakeVBD{getAllResult: vbds},
			vm:  &fakeVM{getByIDErr: errors.New("VM service unavailable")},
			pbd: &fakePBD{},
		},
	}
	_, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal when VM fetch fails for PBD check, got %v", err)
	}
}

// TestControllerGetVolume_PBDSkippedWhenSRUnhealthy verifies that the PBD check is
// skipped when the SR-level condition is already abnormal (no need to check PBDs).
func TestControllerGetVolume_PBDSkippedWhenSRUnhealthy(t *testing.T) {
	t.Parallel()

	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getResult: &payloads.VDI{ID: validVDIUUID, SR: validSRUUID}},
			sr:  &fakeSR{getResult: &payloads.StorageRepository{ID: validSRUUID, InMaintenanceMode: true}},
			vbd: &fakeVBD{getAllResult: vbds},
			// vm and pbd are not set; if called they would panic.
		},
	}
	resp, err := driver.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: validVDIUUID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true for SR in maintenance mode")
	}
}

// ---------------------------------------------------------------------------
// PBD health check tests — ListVolumes
// ---------------------------------------------------------------------------

// TestListVolumes_PBDHealthy verifies a published VDI on a host with an attached PBD
// reports healthy.
func TestListVolumes_PBDHealthy(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID}}
	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	pbds := []*payloads.PBD{{ID: validPBDUUID, SR: validSRUUID, Host: validHostUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: vbds},
			sr:  &fakeSR{getAllResult: []*payloads.StorageRepository{{ID: validSRUUID}}},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: pbds},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if cond.GetAbnormal() {
		t.Errorf("expected Abnormal=false when PBD is attached, got true (message: %q)", cond.GetMessage())
	}
}

// TestListVolumes_PBDMissing verifies that a published VDI whose host has no PBD
// for the SR reports abnormal.
func TestListVolumes_PBDMissing(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID}}
	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: vbds},
			sr:  &fakeSR{getAllResult: []*payloads.StorageRepository{{ID: validSRUUID}}},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: nil}, // no PBDs at all
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true when PBD is missing for host")
	}
}

// TestListVolumes_PBDDetached verifies that a VDI published to a host with a detached
// PBD reports abnormal.
func TestListVolumes_PBDDetached(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID}}
	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	pbds := []*payloads.PBD{{ID: validPBDUUID, SR: validSRUUID, Host: validHostUUID, Attached: false}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: vbds},
			sr:  &fakeSR{getAllResult: []*payloads.StorageRepository{{ID: validSRUUID}}},
			vm:  &fakeVM{getByIDResult: &payloads.VM{ID: validVMUUID, Container: validHostUUID}},
			pbd: &fakePBD{getAllResult: pbds},
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true when PBD is detached")
	}
}

// TestListVolumes_PBDGetFails verifies that a PBD fetch failure returns Internal.
func TestListVolumes_PBDGetFails(t *testing.T) {
	t.Parallel()

	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID}}},
			vbd: &fakeVBD{},
			sr:  &fakeSR{getAllResult: []*payloads.StorageRepository{{ID: validSRUUID}}},
			pbd: &fakePBD{getAllErr: errors.New("PBD service unavailable")},
		},
	}
	_, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if grpcCode(err) != codes.Internal {
		t.Errorf("expected Internal when PBD fetch fails, got %v", err)
	}
}

// TestListVolumes_PBDSkippedWhenSRUnhealthy verifies that PBD check is skipped when
// the SR is already in an abnormal state.
func TestListVolumes_PBDSkippedWhenSRUnhealthy(t *testing.T) {
	t.Parallel()

	vdis := []*payloads.VDI{{ID: validVDIUUID, Size: 1 << 30, SR: validSRUUID}}
	vbds := []*payloads.VBD{{VM: validVMUUID, Attached: true}}
	driver := &xenorchestraCSIDriver{
		xoClient: &fakeXoClient{
			vdi: &fakeVDI{getAllResult: vdis},
			vbd: &fakeVBD{getAllResult: vbds},
			sr:  &fakeSR{getAllResult: []*payloads.StorageRepository{{ID: validSRUUID, InMaintenanceMode: true}}},
			pbd: &fakePBD{getAllResult: nil},
			// vm is not set; if called it would panic.
		},
	}
	resp, err := driver.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := resp.GetEntries()[0].GetStatus().GetVolumeCondition()
	if cond == nil {
		t.Fatal("expected VolumeCondition to be set")
	}
	if !cond.GetAbnormal() {
		t.Errorf("expected Abnormal=true for SR in maintenance mode")
	}
}

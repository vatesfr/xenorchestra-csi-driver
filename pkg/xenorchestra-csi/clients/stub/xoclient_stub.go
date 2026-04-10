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
package stub

import (
	"context"
	"io"
	"testing"

	"github.com/gofrs/uuid"
	gomock "go.uber.org/mock/gomock"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
	"github.com/vatesfr/xenorchestra-go-sdk/client"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"
	xoMock "github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library/mock"
)

type xoClientStub struct {
	t *testing.T
}

func NewXoClientStub(t *testing.T) xoClientStub {
	return xoClientStub{
		t: t,
	}
}

// Host implements [clients.XoClient].
func (c xoClientStub) Host() library.Host {
	return nil
}

// Pool implements [clients.XoClient].
func (c xoClientStub) Pool() library.Pool {
	ctrl := gomock.NewController(c.t)
	mockPool := xoMock.NewMockPool(ctrl)
	mockPool.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&payloads.Pool{
		ID:        uuid.Must(uuid.NewV4()),
		NameLabel: "stub-pool",
		DefaultSR: uuid.Must(uuid.NewV4()),
	}, nil).AnyTimes()
	return mockPool
}

// Task implements [clients.XoClient].
func (c xoClientStub) Task() library.Task {
	return nil
}

// V1Client implements [clients.XoClient].
func (c xoClientStub) V1Client() client.XOClient {
	return nil
}

// SR implements [clients.XoClient].
func (c xoClientStub) SR() library.SR {
	return nil
}

// PBD implements [clients.XoClient].
func (c xoClientStub) PBD() library.PBD {
	return nil
}

// VBD implements [clients.XoClient].
func (c xoClientStub) VBD() library.VBD {
	return nil
}

// VDI implements [clients.XoClient].
func (c xoClientStub) VDI() library.VDI {
	return vdiStub{}
}

// VM implements [clients.XoClient].
func (c xoClientStub) VM() library.VM {
	return nil
}

func (c xoClientStub) GetVBDFromVDIAndVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) (*payloads.VBD, error) {
	return nil, nil
}

func (c xoClientStub) ConnectVBDToVM(ctx context.Context, vbd payloads.VBD) (*payloads.VBD, error) {
	return nil, nil
}

func (c xoClientStub) AttachVDIToVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) (*payloads.VBD, error) {
	return nil, nil
}

func (c xoClientStub) WaitForVDIToBeFullyAttached(ctx context.Context, vbdID uuid.UUID) (*payloads.VBD, error) {
	return nil, nil
}

func (c xoClientStub) IsVDIUsedAnywhere(ctx context.Context, vdi *payloads.VDI) ([]*payloads.VBD, error) {
	return nil, nil
}

func (c xoClientStub) DisconnectVBDFromVM(ctx context.Context, vdi payloads.VDI, vmUUID uuid.UUID) error {
	return nil
}

// Compile time check to ensure xoClientStub implements the XoClient interface
var _ clients.XoClient = xoClientStub{}

type vdiStub struct{}

// Get implements library.VDI. Returns ErrVDINotFound since no volumes exist in the stub.
func (v vdiStub) Get(_ context.Context, _ uuid.UUID) (*payloads.VDI, error) {
	return nil, nil
}

func (v vdiStub) GetAll(_ context.Context, _ int, _ string) ([]*payloads.VDI, error) {
	return nil, nil
}

func (v vdiStub) AddTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (v vdiStub) RemoveTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (v vdiStub) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (v vdiStub) GetTasks(_ context.Context, _ uuid.UUID, _ int, _ string) ([]*payloads.Task, error) {
	return nil, nil
}

func (v vdiStub) Export(_ context.Context, _ uuid.UUID, _ payloads.VDIFormat, _ func(io.Reader) error) error {
	return nil
}

func (v vdiStub) Import(_ context.Context, _ uuid.UUID, _ payloads.VDIFormat, _ io.Reader, _ int64) error {
	return nil
}

func (v vdiStub) Migrate(_ context.Context, _ uuid.UUID, _ uuid.UUID) (string, error) {
	return "", nil
}

func (v vdiStub) Create(_ context.Context, _ payloads.VDICreateParams) (uuid.UUID, error) {
	return uuid.Must(uuid.NewV4()), nil
}

// Compile time check to ensure vdiStub implements the library.VDI interface
var _ library.VDI = vdiStub{}

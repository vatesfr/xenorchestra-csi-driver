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
	"strings"
	"sync"

	"github.com/gofrs/uuid"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
	"github.com/vatesfr/xenorchestra-go-sdk/client"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"
)

type xoClientStub struct{}

func NewXoClientStub() xoClientStub {
	return xoClientStub{}
}

// Host implements [clients.XoClient].
func (c xoClientStub) Host() library.Host {
	return nil
}

// Pool implements [clients.XoClient].
func (c xoClientStub) Pool() library.Pool {
	return poolStub{}
}

// Task implements [clients.XoClient].
func (c xoClientStub) Task() library.Task {
	return nil
}

// V1Client implements [clients.XoClient].
func (c xoClientStub) V1Client() client.XOClient {
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

// PBD implements [clients.XoClient].
func (c xoClientStub) PBD() library.PBD {
	return nil
}

// SR implements [clients.XoClient].
func (c xoClientStub) SR() library.SR {
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

// vdiStore is a package-level in-memory store used by vdiStub to simulate
// a VDI database during sanity tests without a real Xen Orchestra connection.
var vdiStore = struct {
	sync.RWMutex
	byID map[uuid.UUID]payloads.VDI
}{
	byID: make(map[uuid.UUID]payloads.VDI),
}

type vdiStub struct{}

// Get implements library.VDI.
func (v vdiStub) Get(_ context.Context, id uuid.UUID) (*payloads.VDI, error) {
	vdiStore.RLock()
	defer vdiStore.RUnlock()
	vdi, ok := vdiStore.byID[id]
	if !ok {
		return nil, nil
	}
	return &vdi, nil
}

func (v vdiStub) GetAll(_ context.Context, _ int, filter string) ([]*payloads.VDI, error) {
	vdiStore.RLock()
	defer vdiStore.RUnlock()
	var result []*payloads.VDI
	for i := range vdiStore.byID {
		vdi := vdiStore.byID[i]
		if filter == "" || strings.Contains(vdi.NameLabel, filter) {
			v := vdi
			result = append(result, &v)
		}
	}
	return result, nil
}

func (v vdiStub) AddTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (v vdiStub) RemoveTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (v vdiStub) Delete(_ context.Context, id uuid.UUID) error {
	vdiStore.Lock()
	defer vdiStore.Unlock()
	delete(vdiStore.byID, id)
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

func (v vdiStub) Create(_ context.Context, p payloads.VDICreateParams) (uuid.UUID, error) {
	id := uuid.Must(uuid.NewV4())
	vdiStore.Lock()
	defer vdiStore.Unlock()
	vdiStore.byID[id] = payloads.VDI{
		ID:        id,
		NameLabel: p.NameLabel,
		Size:      p.VirtualSize,
	}
	return id, nil
}

// Compile time check to ensure vdiStub implements the library.VDI interface
var _ library.VDI = vdiStub{}

type pbdStub struct{}

func (p pbdStub) Get(_ context.Context, _ uuid.UUID) (*payloads.PBD, error) {
	return nil, nil
}

func (p pbdStub) GetAll(_ context.Context, _ int, _ string) ([]*payloads.PBD, error) {
	return nil, nil
}

func (p pbdStub) Plug(_ context.Context, _ uuid.UUID) (string, error) {
	return "", nil
}

func (p pbdStub) Unplug(_ context.Context, _ uuid.UUID) (string, error) {
	return "", nil
}

// Compile time check to ensure pbdStub implements the library.PBD interface
var _ library.PBD = pbdStub{}

type srStub struct{}

func (s srStub) Get(_ context.Context, _ uuid.UUID) (*payloads.StorageRepository, error) {
	return nil, nil
}

func (s srStub) GetAll(_ context.Context, _ int, _ string) ([]*payloads.StorageRepository, error) {
	return nil, nil
}

func (s srStub) GetTasks(_ context.Context, _ uuid.UUID, _ int, _ string) ([]*payloads.Task, error) {
	return nil, nil
}

func (s srStub) AddTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (s srStub) RemoveTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (s srStub) ReclaimSpace(_ context.Context, _ uuid.UUID) (string, error) {
	return "", nil
}

func (s srStub) Scan(_ context.Context, _ uuid.UUID) (string, error) {
	return "", nil
}

// Compile time check to ensure srStub implements the library.SR interface
var _ library.SR = srStub{}

// stubDefaultSR is a fixed SR UUID returned by poolStub.Get so that CreateVolume
// succeeds in the sanity test without hitting a "pool has no default SR" error.
var stubDefaultSR = uuid.Must(uuid.FromString("00000000-0000-0000-0000-000000000002"))

type poolStub struct{}

func (p poolStub) Get(_ context.Context, id uuid.UUID) (*payloads.Pool, error) {
	return &payloads.Pool{ID: id, DefaultSR: stubDefaultSR}, nil
}

func (p poolStub) GetAll(_ context.Context, _ int, _ string) ([]*payloads.Pool, error) {
	return nil, nil
}

func (p poolStub) AddTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (p poolStub) RemoveTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (p poolStub) CreateVM(_ context.Context, _ uuid.UUID, _ payloads.CreateVMParams) (uuid.UUID, error) {
	return uuid.UUID{}, nil
}

func (p poolStub) CreateNetwork(_ context.Context, _ uuid.UUID, _ payloads.CreateNetworkParams) (uuid.UUID, error) {
	return uuid.UUID{}, nil
}

func (p poolStub) EmergencyShutdown(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (p poolStub) RollingReboot(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (p poolStub) RollingUpdate(_ context.Context, _ uuid.UUID) error {
	return nil
}

// Compile time check to ensure poolStub implements the library.Pool interface
var _ library.Pool = poolStub{}

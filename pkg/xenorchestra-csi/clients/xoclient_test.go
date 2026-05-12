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
package clients

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/vatesfr/xenorchestra-go-sdk/client"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"
	xoLibMock "github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library/mock"
)

// stubLibrary implements library.Library with only SR() wired; all other
// methods panic to catch accidental calls in tests.
type stubLibrary struct {
	library.Library
	sr   library.SR
	vdi  library.VDI
	task library.Task
}

func (s stubLibrary) SR() library.SR        { return s.sr }
func (s stubLibrary) VDI() library.VDI      { return s.vdi }
func (s stubLibrary) Task() library.Task    { return s.task }
func (s stubLibrary) V1Client() v1.XOClient { panic("V1Client not expected in this test") }

var (
	hostUUID   = uuid.Must(uuid.FromString("aaaaaaaa-0000-0000-0000-000000000001"))
	localSRID  = uuid.Must(uuid.FromString("bbbbbbbb-0000-0000-0000-000000000002"))
	localSRID2 = uuid.Must(uuid.FromString("bbbbbbbb-0000-0000-0000-000000000003"))
	poolUUID   = uuid.Must(uuid.FromString("eeeeeeee-0000-0000-0000-000000000005"))
	vdiUUID    = uuid.Must(uuid.FromString("cccccccc-0000-0000-0000-000000000003"))
	newVDIUUID = uuid.Must(uuid.FromString("dddddddd-0000-0000-0000-000000000004"))
	taskID     = "task-abc-123"
)

func newClientWithMockSR(t *testing.T) (*xoClient, *xoLibMock.MockSR) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockSR := xoLibMock.NewMockSR(ctrl)
	c := xoClient{Library: stubLibrary{sr: mockSR}}
	return &c, mockSR
}

func newClientWithMockVDIAndTask(t *testing.T) (*xoClient, *xoLibMock.MockVDI, *xoLibMock.MockTask) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockVDI := xoLibMock.NewMockVDI(ctrl)
	mockTask := xoLibMock.NewMockTask(ctrl)
	c := xoClient{Library: stubLibrary{vdi: mockVDI, task: mockTask}}
	return &c, mockVDI, mockTask
}

func expectedLocalSRFilter(hostID uuid.UUID) string {
	return fmt.Sprintf("content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $container:%s", hostID)
}

func TestFindLocalSRForHost_ReturnsSR(t *testing.T) {
	c, mockSR := newClientWithMockSR(t)

	sr := &payloads.StorageRepository{ID: localSRID, Shared: false, ContentType: "user"}
	mockSR.EXPECT().
		GetAll(gomock.Any(), 1, expectedLocalSRFilter(hostUUID)).
		Return([]*payloads.StorageRepository{sr}, nil)

	got, err := c.FindLocalSRForHost(context.Background(), hostUUID)
	require.NoError(t, err)
	assert.Equal(t, sr, got)
}

func TestFindLocalSRForHost_NoSRFound(t *testing.T) {
	c, mockSR := newClientWithMockSR(t)

	mockSR.EXPECT().
		GetAll(gomock.Any(), 1, expectedLocalSRFilter(hostUUID)).
		Return([]*payloads.StorageRepository{}, nil)

	_, err := c.FindLocalSRForHost(context.Background(), hostUUID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), hostUUID.String())
}

func TestFindLocalSRForHost_APIError(t *testing.T) {
	c, mockSR := newClientWithMockSR(t)

	apiErr := errors.New("connection refused")
	mockSR.EXPECT().
		GetAll(gomock.Any(), 1, expectedLocalSRFilter(hostUUID)).
		Return(nil, apiErr)

	_, err := c.FindLocalSRForHost(context.Background(), hostUUID)
	require.Error(t, err)
	assert.ErrorIs(t, err, apiErr)
}

func TestMigrateVDIAndWait_Success(t *testing.T) {
	c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

	mockVDI.EXPECT().
		Migrate(gomock.Any(), vdiUUID, localSRID).
		Return(taskID, nil)
	mockTask.EXPECT().
		Wait(gomock.Any(), taskID).
		Return(&payloads.Task{
			Status: payloads.Success,
			Result: payloads.Result{ID: newVDIUUID},
		}, nil)

	got, err := c.MigrateVDIAndWait(context.Background(), vdiUUID, localSRID)
	require.NoError(t, err)
	assert.Equal(t, newVDIUUID, got)
}

func TestMigrateVDIAndWait_MigrateError(t *testing.T) {
	c, mockVDI, _ := newClientWithMockVDIAndTask(t)

	migrateErr := errors.New("SR not found")
	mockVDI.EXPECT().
		Migrate(gomock.Any(), vdiUUID, localSRID).
		Return("", migrateErr)

	_, err := c.MigrateVDIAndWait(context.Background(), vdiUUID, localSRID)
	require.Error(t, err)
	assert.ErrorIs(t, err, migrateErr)
}

func TestMigrateVDIAndWait_TaskWaitError(t *testing.T) {
	c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

	waitErr := errors.New("context deadline exceeded")
	mockVDI.EXPECT().
		Migrate(gomock.Any(), vdiUUID, localSRID).
		Return(taskID, nil)
	mockTask.EXPECT().
		Wait(gomock.Any(), taskID).
		Return(nil, waitErr)

	_, err := c.MigrateVDIAndWait(context.Background(), vdiUUID, localSRID)
	require.Error(t, err)
	assert.ErrorIs(t, err, waitErr)
}

func TestMigrateVDIAndWait_TaskFailed(t *testing.T) {
	c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

	mockVDI.EXPECT().
		Migrate(gomock.Any(), vdiUUID, localSRID).
		Return(taskID, nil)
	mockTask.EXPECT().
		Wait(gomock.Any(), taskID).
		Return(&payloads.Task{
			Status: payloads.Failure,
			Result: payloads.Result{Message: "random failure message from XAPI"},
		}, nil)

	_, err := c.MigrateVDIAndWait(context.Background(), vdiUUID, localSRID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(payloads.Failure))
	assert.Contains(t, err.Error(), "random failure message from XAPI")
}

func TestMigrateVDIAndWait_ResultNilID(t *testing.T) {
	c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

	mockVDI.EXPECT().
		Migrate(gomock.Any(), vdiUUID, localSRID).
		Return(taskID, nil)
	mockTask.EXPECT().
		Wait(gomock.Any(), taskID).
		Return(&payloads.Task{
			Status: payloads.Success,
			Result: payloads.Result{ID: uuid.Nil},
		}, nil)

	_, err := c.MigrateVDIAndWait(context.Background(), vdiUUID, localSRID)
	require.Error(t, err)
}

func expectedLocalSRsForPoolFilter(poolID uuid.UUID) string {
	return fmt.Sprintf("content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $pool:%s", poolID)
}

func TestFindLocalSRsForPool_ReturnsSRs(t *testing.T) {
	c, mockSR := newClientWithMockSR(t)

	sr1 := &payloads.StorageRepository{ID: localSRID}
	sr2 := &payloads.StorageRepository{ID: localSRID2}
	mockSR.EXPECT().
		GetAll(gomock.Any(), 0, expectedLocalSRsForPoolFilter(poolUUID)).
		Return([]*payloads.StorageRepository{sr1, sr2}, nil)

	got, err := c.FindLocalSRsForPool(context.Background(), poolUUID)
	require.NoError(t, err)
	assert.Equal(t, []*payloads.StorageRepository{sr1, sr2}, got)
}

func TestFindLocalSRsForPool_NoSRFound(t *testing.T) {
	c, mockSR := newClientWithMockSR(t)

	mockSR.EXPECT().
		GetAll(gomock.Any(), 0, expectedLocalSRsForPoolFilter(poolUUID)).
		Return([]*payloads.StorageRepository{}, nil)

	_, err := c.FindLocalSRsForPool(context.Background(), poolUUID)
	require.Error(t, err)
}

func TestFindLocalSRsForPool_APIError(t *testing.T) {
	c, mockSR := newClientWithMockSR(t)

	apiErr := errors.New("connection refused")
	mockSR.EXPECT().
		GetAll(gomock.Any(), 0, expectedLocalSRsForPoolFilter(poolUUID)).
		Return(nil, apiErr)

	_, err := c.FindLocalSRsForPool(context.Background(), poolUUID)
	require.Error(t, err)
	assert.ErrorIs(t, err, apiErr)
}

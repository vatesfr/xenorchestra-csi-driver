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
	vdiTest    = payloads.VDI{
		ID: vdiUUID,
	}
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

func newClientWithMockVDI(t *testing.T) (*xoClient, *xoLibMock.MockVDI) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockVDI := xoLibMock.NewMockVDI(ctrl)
	c := xoClient{Library: stubLibrary{vdi: mockVDI}}
	return &c, mockVDI
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

func expectedLocalSRsForPoolFilter(poolID uuid.UUID) string {
	return fmt.Sprintf("content_type:user !shared? !inMaintenanceMode? $PBDs:length:>=1 $pool:%s", poolID)
}

// ---------------------------------------------------------------------------
// FindLocalSRForHost
// ---------------------------------------------------------------------------

func TestFindLocalSRForHost(t *testing.T) {
	t.Run("ReturnsSR", func(t *testing.T) {
		c, mockSR := newClientWithMockSR(t)

		sr := &payloads.StorageRepository{ID: localSRID, Shared: false, ContentType: "user"}
		mockSR.EXPECT().
			GetAll(gomock.Any(), 1, expectedLocalSRFilter(hostUUID)).
			Return([]*payloads.StorageRepository{sr}, nil)

		got, err := c.FindLocalSRForHost(context.Background(), hostUUID)
		require.NoError(t, err)
		assert.Equal(t, sr, got)
	})

	t.Run("NoSRFound", func(t *testing.T) {
		c, mockSR := newClientWithMockSR(t)

		mockSR.EXPECT().
			GetAll(gomock.Any(), 1, expectedLocalSRFilter(hostUUID)).
			Return([]*payloads.StorageRepository{}, nil)

		_, err := c.FindLocalSRForHost(context.Background(), hostUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), hostUUID.String())
	})

	t.Run("APIError", func(t *testing.T) {
		c, mockSR := newClientWithMockSR(t)

		apiErr := errors.New("connection refused")
		mockSR.EXPECT().
			GetAll(gomock.Any(), 1, expectedLocalSRFilter(hostUUID)).
			Return(nil, apiErr)

		_, err := c.FindLocalSRForHost(context.Background(), hostUUID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apiErr)
	})
}

// ---------------------------------------------------------------------------
// MigrateVDIAndWait
// ---------------------------------------------------------------------------

func TestMigrateVDIAndWait(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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

		got, err := c.MigrateVDIAndWait(context.Background(), vdiTest, localSRID)
		require.NoError(t, err)
		assert.Equal(t, newVDIUUID, got)
	})

	t.Run("MigrateError", func(t *testing.T) {
		c, mockVDI, _ := newClientWithMockVDIAndTask(t)

		migrateErr := errors.New("SR not found")
		mockVDI.EXPECT().
			Migrate(gomock.Any(), vdiUUID, localSRID).
			Return("", migrateErr)

		_, err := c.MigrateVDIAndWait(context.Background(), vdiTest, localSRID)
		require.Error(t, err)
		assert.ErrorIs(t, err, migrateErr)
	})

	t.Run("TaskWaitError", func(t *testing.T) {
		c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

		waitErr := errors.New("context deadline exceeded")
		mockVDI.EXPECT().
			Migrate(gomock.Any(), vdiUUID, localSRID).
			Return(taskID, nil)
		mockTask.EXPECT().
			Wait(gomock.Any(), taskID).
			Return(nil, waitErr)

		_, err := c.MigrateVDIAndWait(context.Background(), vdiTest, localSRID)
		require.Error(t, err)
		assert.ErrorIs(t, err, waitErr)
	})

	t.Run("TaskFailed", func(t *testing.T) {
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

		_, err := c.MigrateVDIAndWait(context.Background(), vdiTest, localSRID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), string(payloads.Failure))
		assert.Contains(t, err.Error(), "random failure message from XAPI")
	})

	t.Run("ResultNilID", func(t *testing.T) {
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

		_, err := c.MigrateVDIAndWait(context.Background(), vdiTest, localSRID)
		require.Error(t, err)
	})

	t.Run("CopiesTagsToNewVDI", func(t *testing.T) {
		c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000030"
		volumeName := "pvc-tag-copy"
		vdiWithTags := payloads.VDI{
			ID: vdiUUID,
			Tags: []string{
				BuildTag(VDITagKeyVolumeId, volumeId),
				BuildTag(VDITagKeyPVName, volumeName),
				"some-other-tag",
			},
		}

		mockVDI.EXPECT().
			Migrate(gomock.Any(), vdiUUID, localSRID).
			Return(taskID, nil)
		mockTask.EXPECT().
			Wait(gomock.Any(), taskID).
			Return(&payloads.Task{
				Status: payloads.Success,
				Result: payloads.Result{ID: newVDIUUID},
			}, nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), newVDIUUID, BuildTag(VDITagKeyVolumeId, volumeId)).
			Return(nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), newVDIUUID, BuildTag(VDITagKeyPVName, volumeName)).
			Return(nil)

		got, err := c.MigrateVDIAndWait(context.Background(), vdiWithTags, localSRID)
		require.NoError(t, err)
		assert.Equal(t, newVDIUUID, got)
	})

	t.Run("AddTagErrorDoesNotFailMigration", func(t *testing.T) {
		c, mockVDI, mockTask := newClientWithMockVDIAndTask(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000031"
		vdiWithTags := payloads.VDI{
			ID: vdiUUID,
			Tags: []string{
				BuildTag(VDITagKeyVolumeId, volumeId),
			},
		}

		mockVDI.EXPECT().
			Migrate(gomock.Any(), vdiUUID, localSRID).
			Return(taskID, nil)
		mockTask.EXPECT().
			Wait(gomock.Any(), taskID).
			Return(&payloads.Task{
				Status: payloads.Success,
				Result: payloads.Result{ID: newVDIUUID},
			}, nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), newVDIUUID, BuildTag(VDITagKeyVolumeId, volumeId)).
			Return(errors.New("tag write failed"))

		got, err := c.MigrateVDIAndWait(context.Background(), vdiWithTags, localSRID)
		require.NoError(t, err)
		assert.Equal(t, newVDIUUID, got)
	})
}

// ---------------------------------------------------------------------------
// GetVDIByVolumeId
// ---------------------------------------------------------------------------

func TestGetVDIByVolumeId(t *testing.T) {
	t.Run("FoundViaTag", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000010"
		vdi := &payloads.VDI{
			ID:   vdiUUID,
			Tags: []string{BuildTag(VDITagKeyVolumeId, volumeId)},
		}
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{vdi}, nil)

		got, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.NoError(t, err)
		assert.Equal(t, vdi, got)
	})

	t.Run("FallbackViaNameLabel", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000011"
		vdi := &payloads.VDI{
			ID:        vdiUUID,
			NameLabel: "csi-" + volumeId + "-pvc-xyz",
		}
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{vdi}, nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), vdiUUID, BuildTag(VDITagKeyVolumeId, volumeId)).
			Return(nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), vdiUUID, BuildTag(VDITagKeyPVName, "pvc-xyz")).
			Return(nil)

		got, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.NoError(t, err)
		assert.Equal(t, vdi, got)
	})

	t.Run("NotFoundInAllLookups", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000012"
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			Get(gomock.Any(), uuid.FromStringOrNil(volumeId)).
			Return(nil, fmt.Errorf("API error: 404 Not Found"))

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVolumeNotFound)
	})

	t.Run("FoundViaDirectUUID", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000017"
		vdi := &payloads.VDI{
			ID:              uuid.FromStringOrNil(volumeId),
			NameLabel:       "csi-pvc-static",
			NameDescription: "VDI managed by the Kubernetes CSI; pv-name=pvc-static",
		}
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			Get(gomock.Any(), uuid.FromStringOrNil(volumeId)).
			Return(vdi, nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), vdi.ID, BuildTag(VDITagKeyVolumeId, volumeId)).
			Return(nil)
		mockVDI.EXPECT().
			AddTag(gomock.Any(), vdi.ID, BuildTag(VDITagKeyPVName, "pvc-static")).
			Return(nil)

		got, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.NoError(t, err)
		assert.Equal(t, vdi, got)
	})

	t.Run("DirectUUIDNotFound", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000018"
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			Get(gomock.Any(), uuid.FromStringOrNil(volumeId)).
			Return(nil, fmt.Errorf("API error: 404 Not Found"))

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVolumeNotFound)
	})

	t.Run("DirectUUIDAPIError", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000019"
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		apiErr := errors.New("connection refused")
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			Get(gomock.Any(), uuid.FromStringOrNil(volumeId)).
			Return(nil, apiErr)

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, apiErr)
	})

	t.Run("NonUUIDVolumeIdSkipsDirectLookup", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "not-a-uuid"
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{}, nil)

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVolumeNotFound)
	})

	t.Run("AmbiguousViaTag", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000013"
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{{ID: vdiUUID}, {ID: newVDIUUID}}, nil)

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVolumeIdAmbiguous)
	})

	t.Run("AmbiguousViaNameLabelFallback", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000014"
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return([]*payloads.VDI{{ID: vdiUUID}, {ID: newVDIUUID}}, nil)

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVolumeIdAmbiguous)
	})

	t.Run("PrimaryAPIError", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000015"
		apiErr := errors.New("connection refused")
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return(nil, apiErr)

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, apiErr)
	})

	t.Run("FallbackAPIError", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		volumeId := "aaaaaaaa-0000-0000-0000-000000000016"
		apiErr := errors.New("timeout")
		primaryFilter := BuildTagFilter(VDITagKeyVolumeId, volumeId)
		fallbackFilter := fmt.Sprintf("name_label:%s", volumeId)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, primaryFilter).
			Return([]*payloads.VDI{}, nil)
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, fallbackFilter).
			Return(nil, apiErr)

		_, err := c.GetVDIByVolumeId(context.Background(), volumeId)
		require.Error(t, err)
		assert.ErrorIs(t, err, apiErr)
	})
}

// ---------------------------------------------------------------------------
// FindVDIByVolumeName
// ---------------------------------------------------------------------------

func TestFindVDIByVolumeName(t *testing.T) {
	const volumeName = "pvc-my-volume"
	volumeId := "aaaaaaaa-0000-0000-0000-000000000020"

	t.Run("Found", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		vdi := &payloads.VDI{
			ID: vdiUUID,
			Tags: []string{
				BuildTag(VDITagKeyPVName, volumeName),
				BuildTag(VDITagKeyVolumeId, volumeId),
			},
		}
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, BuildTagFilter(VDITagKeyPVName, volumeName)).
			Return([]*payloads.VDI{vdi}, nil)

		gotVDI, gotId, err := c.FindVDIByVolumeName(context.Background(), volumeName)
		require.NoError(t, err)
		assert.Equal(t, vdi, gotVDI)
		assert.Equal(t, volumeId, gotId)
	})

	t.Run("FoundButMissingVolumeIdTag", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		// VDI has the pvName tag but no volumeId tag — ParseTagValue returns "".
		vdi := &payloads.VDI{
			ID:   vdiUUID,
			Tags: []string{BuildTag(VDITagKeyPVName, volumeName)},
		}
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, BuildTagFilter(VDITagKeyPVName, volumeName)).
			Return([]*payloads.VDI{vdi}, nil)

		gotVDI, gotId, err := c.FindVDIByVolumeName(context.Background(), volumeName)
		require.NoError(t, err)
		assert.Equal(t, vdi, gotVDI)
		assert.Empty(t, gotId)
	})

	t.Run("NotFound", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, BuildTagFilter(VDITagKeyPVName, volumeName)).
			Return([]*payloads.VDI{}, nil)

		gotVDI, gotId, err := c.FindVDIByVolumeName(context.Background(), volumeName)
		require.ErrorIs(t, err, ErrVolumeNotFound)
		assert.Nil(t, gotVDI)
		assert.Empty(t, gotId)
	})

	t.Run("Ambiguous", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, BuildTagFilter(VDITagKeyPVName, volumeName)).
			Return([]*payloads.VDI{{ID: vdiUUID}, {ID: newVDIUUID}}, nil)

		gotVDI, gotId, err := c.FindVDIByVolumeName(context.Background(), volumeName)
		require.ErrorIs(t, err, ErrVolumeNameAmbiguous)
		assert.Nil(t, gotVDI)
		assert.Empty(t, gotId)
	})

	t.Run("APIError", func(t *testing.T) {
		c, mockVDI := newClientWithMockVDI(t)

		apiErr := errors.New("connection refused")
		mockVDI.EXPECT().
			GetAll(gomock.Any(), 2, BuildTagFilter(VDITagKeyPVName, volumeName)).
			Return(nil, apiErr)

		gotVDI, gotId, err := c.FindVDIByVolumeName(context.Background(), volumeName)
		require.ErrorIs(t, err, apiErr)
		assert.Nil(t, gotVDI)
		assert.Empty(t, gotId)
	})
}

// ---------------------------------------------------------------------------
// FindLocalSRsForPool
// ---------------------------------------------------------------------------

func TestFindLocalSRsForPool(t *testing.T) {
	t.Run("ReturnsSRs", func(t *testing.T) {
		c, mockSR := newClientWithMockSR(t)

		sr1 := &payloads.StorageRepository{ID: localSRID}
		sr2 := &payloads.StorageRepository{ID: localSRID2}
		mockSR.EXPECT().
			GetAll(gomock.Any(), 0, expectedLocalSRsForPoolFilter(poolUUID)).
			Return([]*payloads.StorageRepository{sr1, sr2}, nil)

		got, err := c.FindLocalSRsForPool(context.Background(), poolUUID)
		require.NoError(t, err)
		assert.Equal(t, []*payloads.StorageRepository{sr1, sr2}, got)
	})

	t.Run("NoSRFound", func(t *testing.T) {
		c, mockSR := newClientWithMockSR(t)

		mockSR.EXPECT().
			GetAll(gomock.Any(), 0, expectedLocalSRsForPoolFilter(poolUUID)).
			Return([]*payloads.StorageRepository{}, nil)

		_, err := c.FindLocalSRsForPool(context.Background(), poolUUID)
		require.Error(t, err)
	})

	t.Run("APIError", func(t *testing.T) {
		c, mockSR := newClientWithMockSR(t)

		apiErr := errors.New("connection refused")
		mockSR.EXPECT().
			GetAll(gomock.Any(), 0, expectedLocalSRsForPoolFilter(poolUUID)).
			Return(nil, apiErr)

		_, err := c.FindLocalSRsForPool(context.Background(), poolUUID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apiErr)
	})
}

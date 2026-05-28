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

package topology

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	xoLibMock "github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library/mock"
	xok8s "github.com/vatesfr/xenorchestra-k8s-common"
)

var (
	poolUUID1 = uuid.Must(uuid.FromString("aaaaaaaa-0000-0000-0000-000000000001"))
	poolUUID2 = uuid.Must(uuid.FromString("bbbbbbbb-0000-0000-0000-000000000002"))
	poolUUID3 = uuid.Must(uuid.FromString("cccccccc-0000-0000-0000-000000000003"))
)

func seg(poolID string) map[string]string {
	return map[string]string{xok8s.XOLabelTopologyPoolID: poolID}
}

func topo(poolID string) *csi.Topology {
	return &csi.Topology{Segments: seg(poolID)}
}

func TestOrderedPoolIDs(t *testing.T) {
	t.Run("NilRequirement", func(t *testing.T) {
		_, err := OrderedPoolIDs(nil)
		require.ErrorIs(t, err, ErrNoPoolInTopology)
	})

	t.Run("EmptyRequirement", func(t *testing.T) {
		_, err := OrderedPoolIDs(&csi.TopologyRequirement{})
		require.ErrorIs(t, err, ErrNoPoolInTopology)
	})

	t.Run("PreferredOnly", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID2.String())},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID2}, ids)
	})

	t.Run("RequisiteOnly", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Requisite: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID2.String())},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID2}, ids)
	})

	t.Run("PreferredBeforeRequisite", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{topo(poolUUID2.String())},
			Requisite: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID3.String())},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		// preferred first, then requisite not already in preferred
		assert.Equal(t, []uuid.UUID{poolUUID2, poolUUID1, poolUUID3}, ids)
	})

	t.Run("DeduplicatesAcrossPreferredAndRequisite", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID2.String())},
			Requisite: []*csi.Topology{topo(poolUUID3.String()), topo(poolUUID2.String())},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		// poolUUID2 is in both; should only appear once (from preferred)
		assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID2, poolUUID3}, ids)
	})

	t.Run("DeduplicatesWithinPreferred", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID1.String())},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{poolUUID1}, ids)
	})

	t.Run("SkipsTopologiesWithoutPoolSegment", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{
				{Segments: map[string]string{"other-key": "value-abc"}},
				topo(poolUUID1.String()),
			},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{poolUUID1}, ids)
	})

	t.Run("SkipsEmptyPoolSegment", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{
				{Segments: map[string]string{xok8s.XOLabelTopologyPoolID: ""}},
				topo(poolUUID1.String()),
			},
		}
		ids, err := OrderedPoolIDs(ar)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{poolUUID1}, ids)
	})

	t.Run("InvalidUUIDReturnsError", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{
				{Segments: map[string]string{xok8s.XOLabelTopologyPoolID: "not-a-uuid"}},
				topo(poolUUID1.String()),
			},
		}
		_, err := OrderedPoolIDs(ar)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNoPoolInTopology)
	})

	t.Run("NilUUIDReturnsError", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{Segments: map[string]string{xok8s.XOLabelTopologyPoolID: uuid.Nil.String()}},
				topo(poolUUID1.String()),
			},
		}
		_, err := OrderedPoolIDs(ar)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNoPoolInTopology)
	})

	t.Run("AllTopologiesMissingPoolSegment", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Preferred: []*csi.Topology{
				{Segments: map[string]string{"other": "value-def"}},
			},
			Requisite: []*csi.Topology{
				{Segments: map[string]string{"another": "value-ghi"}},
			},
		}
		_, err := OrderedPoolIDs(ar)
		require.ErrorIs(t, err, ErrNoPoolInTopology)
	})
}

func TestValidatePoolIDAgainstRequisite(t *testing.T) {
	t.Run("NilRequirement", func(t *testing.T) {
		err := ValidatePoolIDAgainstRequisite(nil, poolUUID1)
		require.NoError(t, err)
	})

	t.Run("EmptyRequisite", func(t *testing.T) {
		ar := &csi.TopologyRequirement{}
		err := ValidatePoolIDAgainstRequisite(ar, poolUUID1)
		require.NoError(t, err)
	})

	t.Run("PoolIDFound", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Requisite: []*csi.Topology{topo(poolUUID2.String()), topo(poolUUID1.String())},
		}
		err := ValidatePoolIDAgainstRequisite(ar, poolUUID1)
		require.NoError(t, err)
	})

	t.Run("PoolIDNotFound", func(t *testing.T) {
		ar := &csi.TopologyRequirement{
			Requisite: []*csi.Topology{topo(poolUUID2.String()), topo(poolUUID3.String())},
		}
		err := ValidatePoolIDAgainstRequisite(ar, poolUUID1)
		require.Error(t, err)
	})
}

func TestSelectPoolAndStorage(t *testing.T) {
	var (
		srUUID1 = uuid.Must(uuid.FromString("dddddddd-0000-0000-0000-000000000001"))
		srUUID2 = uuid.Must(uuid.FromString("dddddddd-0000-0000-0000-000000000002"))
	)
	t.Run("ReturnsFirstViablePool", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)
		mockSR := xoLibMock.NewMockSR(ctrl)

		pool := &payloads.Pool{DefaultSR: srUUID1}
		sr := &payloads.StorageRepository{InMaintenanceMode: false}

		mockPool.EXPECT().Get(gomock.Any(), poolUUID1).Return(pool, nil)
		mockSR.EXPECT().Get(gomock.Any(), srUUID1).Return(sr, nil)

		gotPool, gotSR, err := SelectPoolAndStorage(context.Background(), mockSR, mockPool, []uuid.UUID{poolUUID1})
		require.NoError(t, err)
		assert.Equal(t, pool, gotPool)
		assert.Equal(t, sr, gotSR)
	})

	t.Run("SkipsPoolNotFound", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)
		mockSR := xoLibMock.NewMockSR(ctrl)

		pool := &payloads.Pool{DefaultSR: srUUID1}
		sr := &payloads.StorageRepository{InMaintenanceMode: false}

		mockPool.EXPECT().Get(gomock.Any(), poolUUID1).Return(nil, errors.New("not found"))
		mockPool.EXPECT().Get(gomock.Any(), poolUUID2).Return(pool, nil)
		mockSR.EXPECT().Get(gomock.Any(), srUUID1).Return(sr, nil)

		gotPool, gotSR, err := SelectPoolAndStorage(context.Background(), mockSR, mockPool, []uuid.UUID{poolUUID1, poolUUID2})
		require.NoError(t, err)
		assert.Equal(t, pool, gotPool)
		assert.Equal(t, sr, gotSR)
	})

	t.Run("SkipsSRInMaintenanceMode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)
		mockSR := xoLibMock.NewMockSR(ctrl)

		pool1 := &payloads.Pool{DefaultSR: srUUID1}
		srMaintenance := &payloads.StorageRepository{InMaintenanceMode: true}
		pool2 := &payloads.Pool{DefaultSR: srUUID2}
		srOK := &payloads.StorageRepository{InMaintenanceMode: false}

		mockPool.EXPECT().Get(gomock.Any(), poolUUID1).Return(pool1, nil)
		mockSR.EXPECT().Get(gomock.Any(), srUUID1).Return(srMaintenance, nil)
		mockPool.EXPECT().Get(gomock.Any(), poolUUID2).Return(pool2, nil)
		mockSR.EXPECT().Get(gomock.Any(), srUUID2).Return(srOK, nil)

		gotPool, gotSR, err := SelectPoolAndStorage(context.Background(), mockSR, mockPool, []uuid.UUID{poolUUID1, poolUUID2})
		require.NoError(t, err)
		assert.Equal(t, pool2, gotPool)
		assert.Equal(t, srOK, gotSR)
	})

	t.Run("NoViablePool", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)
		mockSR := xoLibMock.NewMockSR(ctrl)

		mockPool.EXPECT().Get(gomock.Any(), poolUUID1).Return(nil, errors.New("unreachable"))
		mockPool.EXPECT().Get(gomock.Any(), poolUUID2).Return(&payloads.Pool{DefaultSR: uuid.Nil}, nil)

		_, _, err := SelectPoolAndStorage(context.Background(), mockSR, mockPool, []uuid.UUID{poolUUID1, poolUUID2})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrPoolNotViable)
	})
}

// --- TaggedPoolIDs ---

func TestTaggedPoolIDs(t *testing.T) {
	tagFilter := func(tag string) string {
		return fmt.Sprintf("tags:/^%s$/", tag)
	}
	const testK8sPoolTag = "k8s-pool"

	t.Run("FindsTaggedPools", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)

		mockPool.EXPECT().GetAll(gomock.Any(), 0, tagFilter(testK8sPoolTag)).Return([]*payloads.Pool{
			{ID: poolUUID1, NameLabel: "k8s-pool", Tags: []string{testK8sPoolTag}},
			{ID: poolUUID3, NameLabel: "k8s-pool-2", Tags: []string{"another", testK8sPoolTag}},
		}, nil)

		ids, err := TaggedPoolIDs(context.Background(), mockPool, testK8sPoolTag)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID3}, ids)
	})

	t.Run("NoMatchingPools", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)

		mockPool.EXPECT().GetAll(gomock.Any(), 0, tagFilter(testK8sPoolTag)).Return([]*payloads.Pool{}, nil)

		ids, err := TaggedPoolIDs(context.Background(), mockPool, testK8sPoolTag)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("XoClientError", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockPool := xoLibMock.NewMockPool(ctrl)

		mockPool.EXPECT().GetAll(gomock.Any(), 0, tagFilter(testK8sPoolTag)).Return(nil, errors.New("xo unavailable"))

		_, err := TaggedPoolIDs(context.Background(), mockPool, testK8sPoolTag)
		require.Error(t, err)
	})
}

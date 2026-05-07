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

func TestOrderedPoolIDs_NilRequirement(t *testing.T) {
	_, err := OrderedPoolIDs(nil)
	require.ErrorIs(t, err, ErrNoPoolInTopology)
}

func TestOrderedPoolIDs_EmptyRequirement(t *testing.T) {
	_, err := OrderedPoolIDs(&csi.TopologyRequirement{})
	require.ErrorIs(t, err, ErrNoPoolInTopology)
}

func TestOrderedPoolIDs_PreferredOnly(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID2.String())},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID2}, ids)
}

func TestOrderedPoolIDs_RequisiteOnly(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Requisite: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID2.String())},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID2}, ids)
}

func TestOrderedPoolIDs_PreferredBeforeRequisite(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{topo(poolUUID2.String())},
		Requisite: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID3.String())},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	// preferred first, then requisite not already in preferred
	assert.Equal(t, []uuid.UUID{poolUUID2, poolUUID1, poolUUID3}, ids)
}

func TestOrderedPoolIDs_DeduplicatesAcrossPreferredAndRequisite(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID2.String())},
		Requisite: []*csi.Topology{topo(poolUUID3.String()), topo(poolUUID2.String())},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	// poolUUID2 is in both; should only appear once (from preferred)
	assert.Equal(t, []uuid.UUID{poolUUID1, poolUUID2, poolUUID3}, ids)
}

func TestOrderedPoolIDs_DeduplicatesWithinPreferred(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{topo(poolUUID1.String()), topo(poolUUID1.String())},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{poolUUID1}, ids)
}

func TestOrderedPoolIDs_SkipsTopologiesWithoutPoolSegment(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{
			{Segments: map[string]string{"other-key": "value-abc"}},
			topo(poolUUID1.String()),
		},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{poolUUID1}, ids)
}

func TestOrderedPoolIDs_SkipsEmptyPoolSegment(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{
			{Segments: map[string]string{xok8s.XOLabelTopologyPoolID: ""}},
			topo(poolUUID1.String()),
		},
	}
	ids, err := OrderedPoolIDs(ar)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{poolUUID1}, ids)
}

func TestOrderedPoolIDs_InvalidUUIDReturnsError(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Preferred: []*csi.Topology{
			{Segments: map[string]string{xok8s.XOLabelTopologyPoolID: "not-a-uuid"}},
			topo(poolUUID1.String()),
		},
	}
	_, err := OrderedPoolIDs(ar)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNoPoolInTopology)
}

func TestOrderedPoolIDs_NilUUIDReturnsError(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Requisite: []*csi.Topology{
			{Segments: map[string]string{xok8s.XOLabelTopologyPoolID: uuid.Nil.String()}},
			topo(poolUUID1.String()),
		},
	}
	_, err := OrderedPoolIDs(ar)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNoPoolInTopology)
}

func TestOrderedPoolIDs_AllTopologiesMissingPoolSegment(t *testing.T) {
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
}

func TestValidatePoolIDAgainstRequisite_NilRequirement(t *testing.T) {
	err := ValidatePoolIDAgainstRequisite(nil, poolUUID1)
	require.NoError(t, err)
}

func TestValidatePoolIDAgainstRequisite_EmptyRequisite(t *testing.T) {
	ar := &csi.TopologyRequirement{}
	err := ValidatePoolIDAgainstRequisite(ar, poolUUID1)
	require.NoError(t, err)
}

func TestValidatePoolIDAgainstRequisite_PoolIDFound(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Requisite: []*csi.Topology{topo(poolUUID2.String()), topo(poolUUID1.String())},
	}
	err := ValidatePoolIDAgainstRequisite(ar, poolUUID1)
	require.NoError(t, err)
}

func TestValidatePoolIDAgainstRequisite_PoolIDNotFound(t *testing.T) {
	ar := &csi.TopologyRequirement{
		Requisite: []*csi.Topology{topo(poolUUID2.String()), topo(poolUUID3.String())},
	}
	err := ValidatePoolIDAgainstRequisite(ar, poolUUID1)
	require.Error(t, err)
}

var (
	srUUID1 = uuid.Must(uuid.FromString("dddddddd-0000-0000-0000-000000000001"))
	srUUID2 = uuid.Must(uuid.FromString("dddddddd-0000-0000-0000-000000000002"))
)

func TestSelectPoolAndStorage_ReturnsFirstViablePool(t *testing.T) {
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
}

func TestSelectPoolAndStorage_SkipsPoolNotFound(t *testing.T) {
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
}

func TestSelectPoolAndStorage_SkipsSRInMaintenanceMode(t *testing.T) {
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
}

func TestSelectPoolAndStorage_NoViablePool(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPool := xoLibMock.NewMockPool(ctrl)
	mockSR := xoLibMock.NewMockSR(ctrl)

	mockPool.EXPECT().Get(gomock.Any(), poolUUID1).Return(nil, errors.New("unreachable"))
	mockPool.EXPECT().Get(gomock.Any(), poolUUID2).Return(&payloads.Pool{DefaultSR: uuid.Nil}, nil)

	_, _, err := SelectPoolAndStorage(context.Background(), mockSR, mockPool, []uuid.UUID{poolUUID1, poolUUID2})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPoolNotViable)
}

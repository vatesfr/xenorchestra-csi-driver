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
package xenorchestracsi

import (
	"context"
	"errors"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/mock"
)

func TestListVolumes(t *testing.T) {
	volumes := map[string][]string{"vol-b": {"vm-1"}, "vol-a": nil, "vol-c": {"vm-2"}}

	newDriver := func(t *testing.T, ret map[string][]string, err error) *xenorchestraCSIDriver {
		xo := mock.NewMockXoClient(gomock.NewController(t))
		xo.EXPECT().ListVolumes(gomock.Any(), "k8s-managed").Return(ret, err).AnyTimes()
		return &xenorchestraCSIDriver{xoClient: xo, clusterTag: "k8s-managed"}
	}
	entries := func(r *csi.ListVolumesResponse) map[string][]string {
		out := map[string][]string{}
		for _, e := range r.GetEntries() {
			out[e.GetVolume().GetVolumeId()] = e.GetStatus().GetPublishedNodeIds()
		}
		return out
	}

	t.Run("AllWithPublishedNodes", func(t *testing.T) {
		resp, err := newDriver(t, volumes, nil).ListVolumes(context.Background(), &csi.ListVolumesRequest{})
		require.NoError(t, err)
		assert.Equal(t, volumes, entries(resp))
		assert.Empty(t, resp.GetNextToken())
	})

	t.Run("Pagination", func(t *testing.T) {
		d := newDriver(t, volumes, nil)
		page1, err := d.ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: 2})
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{"vol-a": nil, "vol-b": {"vm-1"}}, entries(page1))
		require.Equal(t, "2", page1.GetNextToken())

		page2, err := d.ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: 2, StartingToken: page1.GetNextToken()})
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{"vol-c": {"vm-2"}}, entries(page2))
		assert.Empty(t, page2.GetNextToken())
	})

	for _, token := range []string{"x", "-1", "4"} {
		t.Run("BadToken_"+token, func(t *testing.T) {
			_, err := newDriver(t, volumes, nil).ListVolumes(context.Background(), &csi.ListVolumesRequest{StartingToken: token})
			assert.Equal(t, codes.Aborted, status.Code(err))
		})
	}

	t.Run("NegativeMaxEntries", func(t *testing.T) {
		_, err := newDriver(t, volumes, nil).ListVolumes(context.Background(), &csi.ListVolumesRequest{MaxEntries: -1})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("XOErrorIsNotPartial", func(t *testing.T) {
		resp, err := newDriver(t, nil, errors.New("boom")).ListVolumes(context.Background(), &csi.ListVolumesRequest{})
		assert.Nil(t, resp)
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("CapabilitiesAdvertised", func(t *testing.T) {
		resp, err := newDriver(t, nil, nil).ControllerGetCapabilities(context.Background(), &csi.ControllerGetCapabilitiesRequest{})
		require.NoError(t, err)
		var got []csi.ControllerServiceCapability_RPC_Type
		for _, c := range resp.GetCapabilities() {
			got = append(got, c.GetRpc().GetType())
		}
		assert.Contains(t, got, csi.ControllerServiceCapability_RPC_LIST_VOLUMES)
		assert.Contains(t, got, csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES)
	})
}

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
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// NodeGetVolumeStats
// ---------------------------------------------------------------------------

const testVolumeID = "vol-1"

func TestNodeGetVolumeStats(t *testing.T) {
	driver := &xenorchestraCSIDriver{}

	t.Run("usage of the filesystem of the volume path", func(t *testing.T) {
		resp, err := driver.NodeGetVolumeStats(t.Context(), &csi.NodeGetVolumeStatsRequest{
			VolumeId:   testVolumeID,
			VolumePath: t.TempDir(),
		})
		require.NoError(t, err)
		require.Len(t, resp.GetUsage(), 2)

		for _, usage := range resp.GetUsage() {
			assert.Positive(t, usage.GetTotal(), usage.GetUnit().String())
			assert.GreaterOrEqual(t, usage.GetUsed(), int64(0), usage.GetUnit().String())
			assert.GreaterOrEqual(t, usage.GetAvailable(), int64(0), usage.GetUnit().String())
			assert.LessOrEqual(t, usage.GetUsed(), usage.GetTotal(), usage.GetUnit().String())
			assert.LessOrEqual(t, usage.GetAvailable(), usage.GetTotal(), usage.GetUnit().String())
		}
		assert.Equal(t, csi.VolumeUsage_BYTES, resp.GetUsage()[0].GetUnit())
		assert.Equal(t, csi.VolumeUsage_INODES, resp.GetUsage()[1].GetUnit())
	})

	tests := []struct {
		name string
		req  *csi.NodeGetVolumeStatsRequest
		code codes.Code
	}{
		{
			name: "volume ID missing",
			req:  &csi.NodeGetVolumeStatsRequest{VolumePath: "/tmp"},
			code: codes.InvalidArgument,
		},
		{
			name: "volume path missing",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: testVolumeID},
			code: codes.InvalidArgument,
		},
		{
			name: "volume path not found",
			req: &csi.NodeGetVolumeStatsRequest{
				VolumeId:   testVolumeID,
				VolumePath: filepath.Join(t.TempDir(), "missing"),
			},
			code: codes.NotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := driver.NodeGetVolumeStats(t.Context(), tt.req)
			assert.Equal(t, tt.code, status.Code(err))
		})
	}
}

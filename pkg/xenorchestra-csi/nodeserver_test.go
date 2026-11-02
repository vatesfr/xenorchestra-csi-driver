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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
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

// ---------------------------------------------------------------------------
// NodeGetVolumeHealth
// ---------------------------------------------------------------------------

// fakeMounter answers IsMountPoint from a set of mount points; the other methods are not used.
type fakeMounter struct {
	clients.Mounter
	mountPoints map[string]bool
	err         error
}

func (m *fakeMounter) IsMountPoint(target string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if _, err := os.Stat(target); err != nil {
		return false, err
	}
	return m.mountPoints[target], nil
}

func TestNodeGetVolumeHealth(t *testing.T) {
	staging := t.TempDir()
	publish := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	health := func(t *testing.T, mounter *fakeMounter, req *csi.NodeGetVolumeHealthRequest) []*csi.VolumeHealth_VolumeHealthEntry {
		t.Helper()
		driver := &xenorchestraCSIDriver{mounter: mounter}
		resp, err := driver.NodeGetVolumeHealth(t.Context(), req)
		require.NoError(t, err)
		assert.Equal(t, testVolumeID, resp.GetVolumeHealth().GetVolumeId())
		return resp.GetVolumeHealth().GetHealthStatuses()
	}
	bothMounted := &fakeMounter{mountPoints: map[string]bool{staging: true, publish: true}}

	t.Run("healthy: both paths mounted, their filesystem answers", func(t *testing.T) {
		statuses := health(t, bothMounted, &csi.NodeGetVolumeHealthRequest{
			VolumeId:          testVolumeID,
			StagingTargetPath: staging,
			VolumePublishPath: publish,
		})
		assert.Empty(t, statuses)
	})

	t.Run("no path: nothing known about the volume on the node", func(t *testing.T) {
		assert.Empty(t, health(t, bothMounted, &csi.NodeGetVolumeHealthRequest{VolumeId: testVolumeID}))
	})

	tests := []struct {
		name    string
		mounter *fakeMounter
		req     *csi.NodeGetVolumeHealthRequest
		reason  string
		message string
	}{
		{
			name:    "publish path not mounted",
			mounter: &fakeMounter{mountPoints: map[string]bool{staging: true}},
			req: &csi.NodeGetVolumeHealthRequest{
				VolumeId:          testVolumeID,
				StagingTargetPath: staging,
				VolumePublishPath: publish,
			},
			reason:  VolumeHealthReasonNotMounted,
			message: publish + " is not a mount point",
		},
		{
			name:    "staging path not mounted: reported once, before the publish path",
			mounter: &fakeMounter{mountPoints: map[string]bool{}},
			req: &csi.NodeGetVolumeHealthRequest{
				VolumeId:          testVolumeID,
				StagingTargetPath: staging,
				VolumePublishPath: publish,
			},
			reason:  VolumeHealthReasonNotMounted,
			message: staging + " is not a mount point",
		},
		{
			name:    "path that does not exist",
			mounter: bothMounted,
			req:     &csi.NodeGetVolumeHealthRequest{VolumeId: testVolumeID, VolumePublishPath: missing},
			reason:  VolumeHealthReasonNotMounted,
			message: missing + " does not exist",
		},
		{
			name:    "mount point check failing",
			mounter: &fakeMounter{err: errors.New("input/output error")},
			req:     &csi.NodeGetVolumeHealthRequest{VolumeId: testVolumeID, VolumePublishPath: publish},
			reason:  VolumeHealthReasonFilesystemError,
			message: "failed to check the mount point " + publish + ": input/output error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statuses := health(t, tt.mounter, tt.req)
			require.Len(t, statuses, 1)
			assert.Equal(t, csi.VolumeHealthErrorType_INACCESSIBLE, statuses[0].GetStatus())
			assert.Equal(t, tt.reason, statuses[0].GetReason())
			assert.Equal(t, tt.message, statuses[0].GetMessage())
		})
	}

	t.Run("volume ID missing", func(t *testing.T) {
		driver := &xenorchestraCSIDriver{mounter: bothMounted}
		_, err := driver.NodeGetVolumeHealth(t.Context(), &csi.NodeGetVolumeHealthRequest{VolumePublishPath: publish})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestNodeGetCapabilitiesVolumeHealth(t *testing.T) {
	resp, err := (&xenorchestraCSIDriver{}).NodeGetCapabilities(t.Context(), &csi.NodeGetCapabilitiesRequest{})
	require.NoError(t, err)

	var types []csi.NodeServiceCapability_RPC_Type
	for _, c := range resp.GetCapabilities() {
		types = append(types, c.GetRpc().GetType())
	}
	assert.Contains(t, types, csi.NodeServiceCapability_RPC_GET_VOLUME_STATS)
	assert.Contains(t, types, csi.NodeServiceCapability_RPC_GET_VOLUME_HEALTH)
}

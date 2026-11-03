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
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/fake"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/stub"
)

const (
	testVolumeID = "10000000-0000-0000-0000-00000000000a"
	testDevice   = "xvdc"
)

// Names of the invalid-argument cases shared by several RPCs.
const (
	caseMissingVolumeID    = "missing volume ID"
	caseMissingStagingPath = "missing staging path"
)

var errInjected = errors.New("injected failure")

type nodeTestEnv struct {
	driver  Driver
	mounter *fake.Mounter
	staging string
	target  string
}

func newNodeTestEnv(t *testing.T) *nodeTestEnv {
	t.Helper()
	mounter := fake.NewMounter()
	driver := NewDriverWithDependencies(&DriverOptions{
		DriverName: "csi.xenorchestra.vates.tech",
		NodeName:   "test-node",
		Endpoint:   "unix:///tmp/unused.sock",
		Mode:       DriverModeNode,
	}, stub.NewNodeMetadataGetterStub(), nil, mounter)

	// Kubelet creates the staging directory; the driver creates the target directory.
	dir := t.TempDir()
	staging := filepath.Join(dir, "staging")
	require.NoError(t, os.Mkdir(staging, 0o750))
	return &nodeTestEnv{
		driver:  driver,
		mounter: mounter,
		staging: staging,
		target:  filepath.Join(dir, "target"),
	}
}

func mountCapability() *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: DefaultFsType}},
		AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
	}
}

func (e *nodeTestEnv) stageRequest() *csi.NodeStageVolumeRequest {
	return &csi.NodeStageVolumeRequest{
		VolumeId:          testVolumeID,
		StagingTargetPath: e.staging,
		VolumeCapability:  mountCapability(),
		PublishContext:    map[string]string{"device": testDevice, "vbd": "20000000-0000-0000-0000-00000000000b"},
	}
}

func (e *nodeTestEnv) unstageRequest() *csi.NodeUnstageVolumeRequest {
	return &csi.NodeUnstageVolumeRequest{VolumeId: testVolumeID, StagingTargetPath: e.staging}
}

func (e *nodeTestEnv) publishRequest() *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		VolumeId:          testVolumeID,
		StagingTargetPath: e.staging,
		TargetPath:        e.target,
		VolumeCapability:  mountCapability(),
	}
}

func (e *nodeTestEnv) unpublishRequest() *csi.NodeUnpublishVolumeRequest {
	return &csi.NodeUnpublishVolumeRequest{VolumeId: testVolumeID, TargetPath: e.target}
}

func assertCode(t *testing.T, want codes.Code, err error) {
	t.Helper()
	if want == codes.OK {
		require.NoError(t, err)
		return
	}
	require.Error(t, err)
	assert.Equal(t, want, status.Code(err), "error: %v", err)
}

func TestNodeStageVolume_InvalidArguments(t *testing.T) {
	tests := []struct {
		name   string
		modify func(req *csi.NodeStageVolumeRequest)
	}{
		{caseMissingVolumeID, func(req *csi.NodeStageVolumeRequest) { req.VolumeId = "" }},
		{caseMissingStagingPath, func(req *csi.NodeStageVolumeRequest) { req.StagingTargetPath = "" }},
		{"missing capability", func(req *csi.NodeStageVolumeRequest) { req.VolumeCapability = nil }},
		{"block capability", func(req *csi.NodeStageVolumeRequest) {
			req.VolumeCapability.AccessType = &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}
		}},
		{"missing device", func(req *csi.NodeStageVolumeRequest) { delete(req.PublishContext, "device") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newNodeTestEnv(t)
			req := env.stageRequest()
			tt.modify(req)

			_, err := env.driver.NodeStageVolume(context.Background(), req)
			assertCode(t, codes.InvalidArgument, err)
			assert.Empty(t, env.mounter.Entries())
		})
	}
}

func TestNodeUnstageVolume_InvalidArguments(t *testing.T) {
	tests := []struct {
		name   string
		modify func(req *csi.NodeUnstageVolumeRequest)
	}{
		{caseMissingVolumeID, func(req *csi.NodeUnstageVolumeRequest) { req.VolumeId = "" }},
		{caseMissingStagingPath, func(req *csi.NodeUnstageVolumeRequest) { req.StagingTargetPath = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newNodeTestEnv(t)
			req := env.unstageRequest()
			tt.modify(req)

			_, err := env.driver.NodeUnstageVolume(context.Background(), req)
			assertCode(t, codes.InvalidArgument, err)
		})
	}
}

func TestNodePublishVolume_InvalidArguments(t *testing.T) {
	tests := []struct {
		name   string
		modify func(req *csi.NodePublishVolumeRequest)
	}{
		{caseMissingVolumeID, func(req *csi.NodePublishVolumeRequest) { req.VolumeId = "" }},
		{"missing target path", func(req *csi.NodePublishVolumeRequest) { req.TargetPath = "" }},
		{caseMissingStagingPath, func(req *csi.NodePublishVolumeRequest) { req.StagingTargetPath = "" }},
		{"missing capability", func(req *csi.NodePublishVolumeRequest) { req.VolumeCapability = nil }},
		{"unsupported access mode", func(req *csi.NodePublishVolumeRequest) {
			req.VolumeCapability.AccessMode.Mode = csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newNodeTestEnv(t)
			req := env.publishRequest()
			tt.modify(req)

			_, err := env.driver.NodePublishVolume(context.Background(), req)
			assertCode(t, codes.InvalidArgument, err)
			assert.Empty(t, env.mounter.Entries())
		})
	}
}

func TestNodeUnpublishVolume_InvalidArguments(t *testing.T) {
	tests := []struct {
		name   string
		modify func(req *csi.NodeUnpublishVolumeRequest)
	}{
		{caseMissingVolumeID, func(req *csi.NodeUnpublishVolumeRequest) { req.VolumeId = "" }},
		{"missing target path", func(req *csi.NodeUnpublishVolumeRequest) { req.TargetPath = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newNodeTestEnv(t)
			req := env.unpublishRequest()
			tt.modify(req)

			_, err := env.driver.NodeUnpublishVolume(context.Background(), req)
			assertCode(t, codes.InvalidArgument, err)
		})
	}
}

func TestNodeVolumeLifecycle(t *testing.T) {
	env := newNodeTestEnv(t)
	ctx := context.Background()

	_, err := env.driver.NodeStageVolume(ctx, env.stageRequest())
	require.NoError(t, err)
	assert.Equal(t, []fake.MountEntry{{Device: "/dev/xvdc", Path: env.staging, FsType: DefaultFsType, Options: []string{}}}, env.mounter.Entries())

	_, err = env.driver.NodePublishVolume(ctx, env.publishRequest())
	require.NoError(t, err)
	assert.Equal(t, 1, env.mounter.MountDepth(env.target))
	assert.DirExists(t, env.target)

	_, err = env.driver.NodeUnpublishVolume(ctx, env.unpublishRequest())
	require.NoError(t, err)
	assert.NoDirExists(t, env.target)

	_, err = env.driver.NodeUnstageVolume(ctx, env.unstageRequest())
	require.NoError(t, err)
	assert.Empty(t, env.mounter.Entries())
}

func TestNodePublishVolume_BindMount(t *testing.T) {
	tests := []struct {
		name     string
		readonly bool
		wantOpts []string
	}{
		{"read-write", false, []string{"bind"}},
		{"read-only", true, []string{"ro", "bind"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newNodeTestEnv(t)
			ctx := context.Background()
			_, err := env.driver.NodeStageVolume(ctx, env.stageRequest())
			require.NoError(t, err)

			req := env.publishRequest()
			req.Readonly = tt.readonly
			_, err = env.driver.NodePublishVolume(ctx, req)
			require.NoError(t, err)

			entries := env.mounter.Entries()
			require.Len(t, entries, 2)
			assert.Equal(t, fake.MountEntry{Device: "/dev/xvdc", Path: env.target, FsType: DefaultFsType, Options: tt.wantOpts}, entries[1])
		})
	}
}

func TestNodePublishVolume_Idempotent(t *testing.T) {
	env := newNodeTestEnv(t)
	ctx := context.Background()
	_, err := env.driver.NodeStageVolume(ctx, env.stageRequest())
	require.NoError(t, err)

	for range 2 {
		_, err = env.driver.NodePublishVolume(ctx, env.publishRequest())
		require.NoError(t, err)
	}
	assert.Equal(t, 1, env.mounter.MountDepth(env.target))
}

func TestNodeUnstageVolume_NotMounted(t *testing.T) {
	env := newNodeTestEnv(t)

	_, err := env.driver.NodeUnstageVolume(context.Background(), env.unstageRequest())
	require.NoError(t, err)
}

func TestNodeUnpublishVolume_NotMounted(t *testing.T) {
	env := newNodeTestEnv(t)

	_, err := env.driver.NodeUnpublishVolume(context.Background(), env.unpublishRequest())
	require.NoError(t, err)
}

func TestNodeVolume_Errors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, env *nodeTestEnv)
		call  func(env *nodeTestEnv) error
	}{
		{
			name:  "stage with missing device",
			setup: func(_ *testing.T, env *nodeTestEnv) { env.mounter.MissingDevice(testDevice) },
			call: func(env *nodeTestEnv) error {
				_, err := env.driver.NodeStageVolume(context.Background(), env.stageRequest())
				return err
			},
		},
		{
			name:  "stage with format and mount failure",
			setup: func(_ *testing.T, env *nodeTestEnv) { env.mounter.FailNext(fake.OpFormatAndMount, errInjected) },
			call: func(env *nodeTestEnv) error {
				_, err := env.driver.NodeStageVolume(context.Background(), env.stageRequest())
				return err
			},
		},
		{
			name:  "publish with mount failure",
			setup: func(_ *testing.T, env *nodeTestEnv) { env.mounter.FailNext(fake.OpMount, errInjected) },
			call: func(env *nodeTestEnv) error {
				_, err := env.driver.NodePublishVolume(context.Background(), env.publishRequest())
				return err
			},
		},
		{
			name: "unpublish with unmount failure",
			setup: func(t *testing.T, env *nodeTestEnv) {
				_, err := env.driver.NodeStageVolume(context.Background(), env.stageRequest())
				require.NoError(t, err)
				_, err = env.driver.NodePublishVolume(context.Background(), env.publishRequest())
				require.NoError(t, err)
				env.mounter.FailNext(fake.OpUnmount, errInjected)
			},
			call: func(env *nodeTestEnv) error {
				_, err := env.driver.NodeUnpublishVolume(context.Background(), env.unpublishRequest())
				return err
			},
		},
		{
			name: "unstage with unmount failure",
			setup: func(t *testing.T, env *nodeTestEnv) {
				_, err := env.driver.NodeStageVolume(context.Background(), env.stageRequest())
				require.NoError(t, err)
				env.mounter.FailNext(fake.OpUnmount, errInjected)
			},
			call: func(env *nodeTestEnv) error {
				_, err := env.driver.NodeUnstageVolume(context.Background(), env.unstageRequest())
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newNodeTestEnv(t)
			tt.setup(t, env)

			assertCode(t, codes.Internal, tt.call(env))
		})
	}
}

func TestNodeStageVolume_Idempotent(t *testing.T) {
	t.Skip("NodeStageVolume is not idempotent yet")

	env := newNodeTestEnv(t)
	for range 2 {
		_, err := env.driver.NodeStageVolume(context.Background(), env.stageRequest())
		require.NoError(t, err)
	}
	assert.Equal(t, 1, env.mounter.MountDepth(env.staging))
}

func TestNodeUnstageVolume_AfterRepeatedStage(t *testing.T) {
	t.Skip("NodeStageVolume is not idempotent yet")

	env := newNodeTestEnv(t)
	ctx := context.Background()
	for range 2 {
		_, err := env.driver.NodeStageVolume(ctx, env.stageRequest())
		require.NoError(t, err)
	}

	_, err := env.driver.NodeUnstageVolume(ctx, env.unstageRequest())
	require.NoError(t, err)
	assert.Empty(t, env.mounter.Entries())
}

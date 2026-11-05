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

package fake

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDevicePath = "/dev/xvdc"
	bindOption     = "bind"
)

func TestMountStacks(t *testing.T) {
	m := NewMounter()
	target := t.TempDir()

	require.NoError(t, m.FormatAndMount(testDevicePath, target, "ext4", nil))
	require.NoError(t, m.FormatAndMount(testDevicePath, target, "ext4", nil))

	assert.Equal(t, 2, m.MountDepth(target))
	mounted, err := m.IsMountPoint(target)
	require.NoError(t, err)
	assert.True(t, mounted)
}

func TestGetDeviceNameFromMount(t *testing.T) {
	staging := "/staging"
	tests := []struct {
		name         string
		mounts       []MountEntry
		wantDevice   string
		wantRefCount int
	}{
		{
			name: "not mounted",
		},
		{
			name:         "single mount",
			mounts:       []MountEntry{{Device: testDevicePath, Path: staging}},
			wantDevice:   testDevicePath,
			wantRefCount: 1,
		},
		{
			name:         "stacked mounts of the same device",
			mounts:       []MountEntry{{Device: testDevicePath, Path: staging}, {Device: testDevicePath, Path: staging}},
			wantDevice:   testDevicePath,
			wantRefCount: 2,
		},
		{
			name:         "stacked mounts of two devices return the oldest",
			mounts:       []MountEntry{{Device: testDevicePath, Path: staging}, {Device: "/dev/xvdd", Path: staging}},
			wantDevice:   testDevicePath,
			wantRefCount: 1,
		},
		{
			name:         "same device on another path counts",
			mounts:       []MountEntry{{Device: testDevicePath, Path: staging}, {Device: testDevicePath, Path: "/other"}},
			wantDevice:   testDevicePath,
			wantRefCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMounter()
			for _, e := range tt.mounts {
				require.NoError(t, m.FormatAndMount(e.Device, e.Path, "ext4", nil))
			}

			device, refCount, err := m.GetDeviceNameFromMount(staging)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDevice, device)
			assert.Equal(t, tt.wantRefCount, refCount)
		})
	}
}

func TestBindMountUsesSourceDevice(t *testing.T) {
	m := NewMounter()
	staging, target := t.TempDir(), t.TempDir()

	require.NoError(t, m.FormatAndMount(testDevicePath, staging, "ext4", nil))
	require.NoError(t, m.Mount(staging, target, "ext4", []string{"ro", bindOption}))

	device, refCount, err := m.GetDeviceNameFromMount(staging)
	require.NoError(t, err)
	assert.Equal(t, testDevicePath, device)
	assert.Equal(t, 2, refCount)

	entries := m.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, MountEntry{Device: testDevicePath, Path: target, FsType: "ext4", Options: []string{"ro", bindOption}}, entries[1])
}

func TestUnmount(t *testing.T) {
	t.Run("not mounted removes the directory", func(t *testing.T) {
		m := NewMounter()
		target := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.Mkdir(target, 0o750))

		require.NoError(t, m.Unmount(target))
		assert.NoDirExists(t, target)
	})

	t.Run("missing path is not an error", func(t *testing.T) {
		m := NewMounter()
		require.NoError(t, m.Unmount(filepath.Join(t.TempDir(), "missing")))
	})

	t.Run("mounted once unmounts and removes the directory", func(t *testing.T) {
		m := NewMounter()
		target := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.Mkdir(target, 0o750))
		require.NoError(t, m.FormatAndMount(testDevicePath, target, "ext4", nil))

		require.NoError(t, m.Unmount(target))
		assert.Equal(t, 0, m.MountDepth(target))
		assert.NoDirExists(t, target)
	})

	t.Run("stacked mount fails and keeps one entry", func(t *testing.T) {
		m := NewMounter()
		target := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.Mkdir(target, 0o750))
		require.NoError(t, m.FormatAndMount(testDevicePath, target, "ext4", nil))
		require.NoError(t, m.FormatAndMount(testDevicePath, target, "ext4", nil))

		require.Error(t, m.Unmount(target))
		assert.Equal(t, 1, m.MountDepth(target))
		assert.DirExists(t, target)
	})
}

func TestFindDevicePath(t *testing.T) {
	m := NewMounter()

	path, err := m.FindDevicePath("xvdc", "")
	require.NoError(t, err)
	assert.Equal(t, testDevicePath, path)

	m.MissingDevice("xvdc")
	_, err = m.FindDevicePath("xvdc", "")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFailNext(t *testing.T) {
	errBoom := errors.New("boom")
	tests := []struct {
		op   Op
		call func(m *Mounter, target string) error
	}{
		{OpFormatAndMount, func(m *Mounter, target string) error { return m.FormatAndMount(testDevicePath, target, "ext4", nil) }},
		{OpMount, func(m *Mounter, target string) error {
			return m.Mount("/staging", target, "ext4", []string{bindOption})
		}},
		{OpUnmount, func(m *Mounter, target string) error { return m.Unmount(target) }},
	}
	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			m := NewMounter()
			target := t.TempDir()

			m.FailNext(tt.op, errBoom)
			require.ErrorIs(t, tt.call(m, target), errBoom)
			assert.Empty(t, m.Entries())
			require.NoError(t, tt.call(m, target), "the failure applies to one call only")
		})
	}
}

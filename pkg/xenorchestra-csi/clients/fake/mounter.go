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

// Package fake provides in-memory implementations of the clients interfaces
// that behave like the real ones, for unit and sanity tests.
package fake

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
)

// Op names a Mounter operation, used to inject failures with FailNext.
type Op string

const (
	OpFormatAndMount Op = "FormatAndMount"
	OpMount          Op = "Mount"
	OpUnmount        Op = "Unmount"
)

// MountEntry is one line of the fake /proc/mounts.
type MountEntry struct {
	Device  string
	Path    string
	FsType  string
	Options []string
}

// Mounter is an in-memory mount table that follows the semantics of
// k8s.io/mount-utils on Linux:
//   - mounting on an already mounted path stacks a new entry, it never fails;
//   - a bind mount records the device of its source, as /proc/mounts does;
//   - GetDeviceNameFromMount returns the device of the first entry for a path
//     and the number of entries, on any path, that use this device;
//   - Unmount behaves like mountutils.CleanupMountPoint: it removes the
//     directory of a path that is not mounted, unmounts one entry otherwise,
//     and fails if the path is still mounted afterwards.
//
// Directories are real: only the mount table is faked.
type Mounter struct {
	mu             sync.Mutex
	entries        []MountEntry
	missingDevices map[string]bool
	failNext       map[Op]error
}

func NewMounter() *Mounter {
	return &Mounter{
		missingDevices: make(map[string]bool),
		failNext:       make(map[Op]error),
	}
}

// FailNext makes the next call to op return err.
func (m *Mounter) FailNext(op Op, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext[op] = err
}

// MissingDevice makes FindDevicePath report the device as absent from the host.
func (m *Mounter) MissingDevice(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.missingDevices[name] = true
}

// MountDepth returns the number of mounts stacked on path.
func (m *Mounter) MountDepth(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	depth := 0
	for _, e := range m.entries {
		if e.Path == path {
			depth++
		}
	}
	return depth
}

// Entries returns a copy of the mount table, oldest entry first.
func (m *Mounter) Entries() []MountEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.entries)
}

func (m *Mounter) FormatAndMount(source, target, fstype string, options []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.takeFailure(OpFormatAndMount); err != nil {
		return err
	}
	m.entries = append(m.entries, MountEntry{Device: source, Path: target, FsType: fstype, Options: slices.Clone(options)})
	return nil
}

func (m *Mounter) Mount(source, target, fstype string, options []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.takeFailure(OpMount); err != nil {
		return err
	}
	device := source
	if slices.Contains(options, "bind") {
		if e, ok := m.firstEntry(source); ok {
			device = e.Device
		}
	}
	m.entries = append(m.entries, MountEntry{Device: device, Path: target, FsType: fstype, Options: slices.Clone(options)})
	return nil
}

func (m *Mounter) Unmount(target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.takeFailure(OpUnmount); err != nil {
		return err
	}
	last := -1
	for i, e := range m.entries {
		if e.Path == target {
			last = i
		}
	}
	if last >= 0 {
		m.entries = slices.Delete(m.entries, last, last+1)
		if _, ok := m.firstEntry(target); ok {
			return fmt.Errorf("failed to cleanup mount point %v", target)
		}
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Mounter) FindDevicePath(deviceName string, vbdUUID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	devicePath := filepath.Join("/dev", deviceName)
	if m.missingDevices[deviceName] {
		return "", fmt.Errorf("device %s: %w", devicePath, os.ErrNotExist)
	}
	return devicePath, nil
}

func (m *Mounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.firstEntry(mountPath)
	if !ok {
		return "", 0, nil
	}
	refCount := 0
	for _, other := range m.entries {
		if other.Device == e.Device {
			refCount++
		}
	}
	return e.Device, refCount, nil
}

func (m *Mounter) IsMountPoint(target string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.firstEntry(target)
	return ok, nil
}

// firstEntry returns the oldest entry for path. The caller holds m.mu.
func (m *Mounter) firstEntry(path string) (MountEntry, bool) {
	for _, e := range m.entries {
		if e.Path == path {
			return e, true
		}
	}
	return MountEntry{}, false
}

// takeFailure returns and clears the failure injected for op. The caller holds m.mu.
func (m *Mounter) takeFailure(op Op) error {
	err := m.failNext[op]
	delete(m.failNext, op)
	return err
}

// Compile time check to ensure Mounter implements the Mounter interface
var _ clients.Mounter = &Mounter{}

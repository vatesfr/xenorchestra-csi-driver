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

package stub

import (
	"sync"

	csisanity "github.com/kubernetes-csi/csi-test/v5/pkg/sanity"

	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"
)

// StubMounter simulates filesystem operations in memory.
// dirs tracks which target paths are currently "mounted".
type StubMounter struct {
	mu   sync.Mutex
	dirs map[string]bool
}

func NewStubMounter() *StubMounter {
	return &StubMounter{
		dirs: make(map[string]bool),
	}
}

// FormatAndMount simulates a format+mount by recording target as mounted.
func (s *StubMounter) FormatAndMount(source, target, fstype string, options []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirs[target] = true
	return nil
}

// Mount simulates a mount by recording target as mounted.
func (s *StubMounter) Mount(source, target, fstype string, options []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirs[target] = true
	return nil
}

// Unmount simulates an unmount by removing target from the in-memory map.
func (s *StubMounter) Unmount(target string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.dirs, target)
	return nil
}

// FindDevicePath returns a fake device path, mirroring the real SafeMounter behavior.
func (s *StubMounter) FindDevicePath(deviceName string, vbdUUID string) (string, error) {
	return "/dev/" + deviceName, nil
}

// GetDeviceNameFromMount returns a non-empty device name
func (s *StubMounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	return "/dev/xvdc", 0, nil
}

// IsMountPoint reports whether target is currently tracked as mounted.
func (s *StubMounter) IsMountPoint(target string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mounted, ok := s.dirs[target]; ok {
		return mounted, nil
	}
	return false, nil
}

// CheckPath checks if a path exists in the mounted directories.
func (s *StubMounter) CheckPath(path string) (csisanity.PathKind, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.dirs[path]; ok {
		return csisanity.PathIsDir, nil
	}
	return csisanity.PathIsNotFound, nil
}

// Compile time check to ensure StubMounter implements the Mounter interface
var _ clients.Mounter = &StubMounter{}

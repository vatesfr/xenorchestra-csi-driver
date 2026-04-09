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

import "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients"

type StubMounter struct{}

func NewStubMounter() *StubMounter {
	return &StubMounter{}
}

func (s *StubMounter) FormatAndMount(source, target, fstype string, options []string) error {
	return nil
}

func (s *StubMounter) Unmount(target string) error {
	return nil
}

func (s *StubMounter) Mount(source, target, fstype string, options []string) error {
	return nil
}

func (s *StubMounter) FindDevicePath(deviceName string, vbdUUID string) (string, error) {
	return "", nil
}

func (s *StubMounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	return "", 0, nil
}

func (s *StubMounter) IsMountPoint(target string) (bool, error) {
	return false, nil
}

// Compile time check to ensure StubMounter implements the Mounter interface
var _ clients.Mounter = &StubMounter{}

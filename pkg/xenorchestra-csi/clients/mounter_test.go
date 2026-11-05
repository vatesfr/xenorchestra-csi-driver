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

package clients

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeMounterFindDevicePath(t *testing.T) {
	m := NewSafeMounter()

	// /dev/null exists on every Linux host.
	path, err := m.FindDevicePath("null", "")
	require.NoError(t, err)
	assert.Equal(t, "/dev/null", path)

	_, err = m.FindDevicePath("xvd-not-present", "")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

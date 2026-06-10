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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
)

// ---------------------------------------------------------------------------
// BuildTag
// ---------------------------------------------------------------------------

func TestBuildTag(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{
			name:  "volume ID tag",
			key:   VDITagKeyVolumeId,
			value: "aaaaaaab-0000-0000-0000-000000000001",
			want:  "k8s:volumeId:aaaaaaab-0000-0000-0000-000000000001",
		},
		{
			name:  "PV name tag",
			key:   VDITagKeyPVName,
			value: "pvc-my-pv",
			want:  "k8s:pvName:pvc-my-pv",
		},
		{
			name:  "managed-by tag",
			key:   VDITagKeyManagedBy,
			value: "csi.xenorchestra.vates.tech@0.4.0",
			want:  "k8s:managedBy:csi.xenorchestra.vates.tech@0.4.0",
		},
		{
			name:  "value with colons is preserved verbatim",
			key:   "someKey1",
			value: "a:b:c",
			want:  "k8s:someKey1:a:b:c",
		},
		{
			name:  "empty value",
			key:   VDITagKeyVolumeId,
			value: "",
			want:  "k8s:volumeId:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTag(tt.key, tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ParseTagValue
// ---------------------------------------------------------------------------

func TestParseTagValue(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		key  string
		want string
	}{
		{
			name: "matching tag first",
			tags: []string{"k8s:volumeId:abc-123"},
			key:  VDITagKeyVolumeId,
			want: "abc-123",
		},
		{
			name: "matching tag among several",
			tags: []string{
				"k8s:pvName:pvc-x",
				"k8s:volumeId:abc-456",
				"k8s:managedBy:csi@0.4.0",
				"some-other-tag",
			},
			key:  VDITagKeyVolumeId,
			want: "abc-456",
		},
		{
			name: "key not present returns empty string",
			tags: []string{"k8s:pvName:pvc-x", "k8s:managedBy:csi@0.4.0"},
			key:  VDITagKeyVolumeId,
			want: "",
		},
		{
			name: "nil tags returns empty string",
			tags: nil,
			key:  VDITagKeyVolumeId,
			want: "",
		},
		{
			name: "empty tags slice returns empty string",
			tags: []string{},
			key:  VDITagKeyVolumeId,
			want: "",
		},
		{
			name: "value containing colons is returned intact",
			tags: []string{"k8s:someKey:a:b:c"},
			key:  "someKey",
			want: "a:b:c",
		},
		{
			name: "tag with correct prefix but wrong key is not matched",
			tags: []string{"k8s:pvNameExtra:pvc-x"},
			key:  VDITagKeyPVName,
			want: "",
		},
		{
			name: "empty value is returned correctly",
			tags: []string{"k8s:volumeId:"},
			key:  VDITagKeyVolumeId,
			want: "",
		},
		{
			name: "returns first match when key appears more than once",
			tags: []string{"k8s:volumeId:first", "k8s:volumeId:second"},
			key:  VDITagKeyVolumeId,
			want: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTagValue(tt.tags, tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// BuildTagFilter
// ---------------------------------------------------------------------------

func TestBuildTagFilter(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{
			name:  "plain UUID value — hyphens are not regex metacharacters, no escaping needed",
			key:   VDITagKeyVolumeId,
			value: "aaaaaaaa-0000-0000-0000-000000000001",
			want:  `tags:/^k8s:volumeId:aaaaaaaa-0000-0000-0000-000000000001$/`,
		},
		{
			name:  "PV name with hyphens — hyphens are not regex metacharacters, no escaping needed",
			key:   VDITagKeyPVName,
			value: "pvc-my-volume",
			want:  `tags:/^k8s:pvName:pvc-my-volume$/`,
		},
		{
			name:  "managed-by value with dot and at is escaped",
			key:   VDITagKeyManagedBy,
			value: "csi.xenorchestra.vates.tech@0.4.1",
			want:  `tags:/^k8s:managedBy:csi\.xenorchestra\.vates\.tech@0\.4\.1$/`,
		},
		{
			name:  "value with regex special chars is fully escaped",
			key:   "key",
			value: "a.b*c+d?e[f]g",
			want:  `tags:/^k8s:key:a\.b\*c\+d\?e\[f\]g$/`,
		},
		{
			name:  "empty value",
			key:   VDITagKeyVolumeId,
			value: "",
			want:  `tags:/^k8s:volumeId:$/`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTagFilter(tt.key, tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Round-trip: BuildTag / ParseTagValue
// ---------------------------------------------------------------------------

func TestBuildTagParseTagValueRoundTrip(t *testing.T) {
	pairs := []struct {
		key   string
		value string
	}{
		{VDITagKeyVolumeId, "aaaaaaaa-0000-0000-0000-000000000001"},
		{VDITagKeyPVName, "pvc-some-persistent-volume"},
		{VDITagKeyManagedBy, "csi.xenorchestra.vates.tech@0.4.2"},
		{"someKey", "value:with:colons"},
	}

	for _, p := range pairs {
		t.Run(p.key+"="+p.value, func(t *testing.T) {
			tag := BuildTag(p.key, p.value)
			got := ParseTagValue([]string{tag}, p.key)
			assert.Equal(t, p.value, got, "round-trip failed for key=%q value=%q tag=%q", p.key, p.value, tag)
		})
	}
}

// ---------------------------------------------------------------------------
// RecoverVolumeNameFromVDI
// ---------------------------------------------------------------------------

func TestParseVolumeNameFromVDINameDescription(t *testing.T) {
	tests := []struct {
		name            string
		nameDescription string
		want            string
	}{
		{
			name:            "ExactFormat",
			nameDescription: "VDI managed by the Kubernetes CSI; pv-name=pvc-from-description1",
			want:            "pvc-from-description1",
		},
		{
			name:            "StopsAtSpaceDelimiter",
			nameDescription: "VDI managed by the Kubernetes CSI; pv-name=pvc-from-description2 manual-edit",
			want:            "pvc-from-description2",
		},
		{
			name:            "StopsAtSemicolonDelimiter",
			nameDescription: "VDI managed by the Kubernetes CSI; pv-name=pvc-from-description3; manual-edit",
			want:            "pvc-from-description3",
		},
		{
			name:            "MissingPVName",
			nameDescription: "VDI managed by the Kubernetes CSI",
			want:            "",
		},
		{
			name:            "EmptyPVName",
			nameDescription: "VDI managed by the Kubernetes CSI; pv-name=",
			want:            "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVolumeNameFromVDINameDescription(tt.nameDescription)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRecoverVolumeNameFromVDI(t *testing.T) {
	t.Run("FromNameDescription", func(t *testing.T) {
		vdi := &payloads.VDI{
			NameDescription: "VDI managed by the Kubernetes CSI; pv-name=pvc-from-description",
			NameLabel:       "csi-aaaaaaaa-0000-0000-0000-000000000001-pvc-from-label",
		}

		got := recoverVolumeNameFromVDI(vdi, "aaaaaaaa-0000-0000-0000-000000000001")
		assert.Equal(t, "pvc-from-description", got)
	})

	t.Run("FromNameDescriptionWithManualEditSuffix", func(t *testing.T) {
		vdi := &payloads.VDI{
			NameDescription: "VDI managed by the Kubernetes CSI; pv-name=pvc-from-description manual-edit",
			NameLabel:       "csi-aaaaaaaa-0000-0000-0000-000000000001-pvc-from-label",
		}

		got := recoverVolumeNameFromVDI(vdi, "aaaaaaaa-0000-0000-0000-000000000001")
		assert.Equal(t, "pvc-from-description", got)
	})

	t.Run("FromNameLabelFallback", func(t *testing.T) {
		vdi := &payloads.VDI{
			NameLabel: "csi-aaaaaaaa-0000-0000-0000-000000000002-pvc-from-label",
		}

		got := recoverVolumeNameFromVDI(vdi, "aaaaaaaa-0000-0000-0000-000000000002")
		assert.Equal(t, "pvc-from-label", got)
	})

	t.Run("RejectsNameLabelWithSpace", func(t *testing.T) {
		vdi := &payloads.VDI{
			NameLabel: "csi-aaaaaaaa-0000-0000-0000-000000000002-pvc from label",
		}

		got := recoverVolumeNameFromVDI(vdi, "aaaaaaaa-0000-0000-0000-000000000002")
		assert.Empty(t, got)
	})

	t.Run("EmptyWhenNotRecoverable", func(t *testing.T) {
		vdi := &payloads.VDI{
			NameDescription: "custom description",
			NameLabel:       "custom-label",
		}

		got := recoverVolumeNameFromVDI(vdi, "aaaaaaaa-0000-0000-0000-000000000003")
		assert.Empty(t, got)
	})
}

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
	"fmt"
	"regexp"
	"strings"

	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
)

const vdiNameDescriptionPVNameMarker = "pv-name="

const (
	dns1123LabelFmt     = `[a-z0-9](?:[-a-z0-9]*[a-z0-9])?`
	dns1123SubdomainFmt = dns1123LabelFmt + `(?:\.` + dns1123LabelFmt + `)*`
)

var vdiNameDescriptionPVNameRe = regexp.MustCompile(`(?:^|;)\s*pv-name=(` + dns1123SubdomainFmt + `)(?:[\s;]|$)`)

// BuildVDINameLabel constructs the VDI.name_label for a new volume.
// The format is "<prefix><volumeId>-<volumeName>" (e.g. "csi-12345678-90ab-cdef-pvc-xyz").
// The volumeId is embedded so that GetVDIByVolumeId can fall back to searching
// by name_label when tags have been erased.
func BuildVDINameLabel(prefix, volumeId, volumeName string) string {
	return fmt.Sprintf("%s%s-%s", prefix, volumeId, volumeName)
}

// BuildVDINameDescription constructs the VDI.name_description for a new volume.
// It appends "; pv-name=<volumeName>" to the standard description so operators
// can identify the backing Kubernetes PV in the Xen Orchestra UI.
// This field is not used for lookups.
func BuildVDINameDescription(volumeName string) string {
	return "VDI managed by the Kubernetes CSI; " + vdiNameDescriptionPVNameMarker + volumeName
}

// BuildTag encodes a key-value pair as a VDI tag string using the format
// "k8s:<key>:<value>".
func BuildTag(key, value string) string {
	return fmt.Sprintf("%s:%s:%s", tagPrefix, key, value)
}

// ParseTagValue returns the value associated with key from a tags slice.
// It scans for a tag with the prefix "k8s:<key>:" and returns the remainder.
// Returns an empty string if no matching tag is found.
func ParseTagValue(tags []string, key string) string {
	prefix := fmt.Sprintf("%s:%s:", tagPrefix, key)
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}

// BuildTagFilter builds an XO REST API filter string that matches VDIs
// whose tags contain exactly "k8s:<key>:<value>".
// The value is regex-escaped for safety.
func BuildTagFilter(key, value string) string {
	return fmt.Sprintf("tags:/^%s$/", regexp.QuoteMeta(BuildTag(key, value)))
}

// recoverVolumeNameFromVDI tries to recover the Kubernetes PV name for a VDI.
// It first reads the VDI name_description and falls back to parsing name_label.
func recoverVolumeNameFromVDI(vdi *payloads.VDI, volumeId string) string {
	if vdi == nil {
		return ""
	}

	if volumeName := parseVolumeNameFromVDINameDescription(vdi.NameDescription); volumeName != "" {
		return volumeName
	}

	return parseVolumeNameFromVDINameLabel(vdi.NameLabel, volumeId)
}

func parseVolumeNameFromVDINameDescription(nameDescription string) string {
	matches := vdiNameDescriptionPVNameRe.FindStringSubmatch(nameDescription)
	if len(matches) != 2 {
		return ""
	}

	return matches[1]
}

func parseVolumeNameFromVDINameLabel(nameLabel, volumeId string) string {
	if volumeId == "" {
		return ""
	}

	nameLabelRe := regexp.MustCompile(fmt.Sprintf(`%s-(%s)$`, regexp.QuoteMeta(volumeId), dns1123SubdomainFmt))
	matches := nameLabelRe.FindStringSubmatch(nameLabel)
	if len(matches) != 2 {
		return ""
	}

	return matches[1]
}

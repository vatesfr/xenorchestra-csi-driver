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

// Package topology provides helpers for resolving and validating XenOrchestra
// pool placement from CSI accessibility requirements.
package topology

import (
	"context"
	"errors"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gofrs/uuid"

	"github.com/vatesfr/xenorchestra-go-sdk/pkg/payloads"
	"github.com/vatesfr/xenorchestra-go-sdk/pkg/services/library"
	xok8s "github.com/vatesfr/xenorchestra-k8s-common"
)

// ErrNoPoolInTopology is returned when accessibility requirements carry no
// pool topology segment at all.
var ErrNoPoolInTopology = fmt.Errorf("accessibility_requirements contain no %q segment", xok8s.XOLabelTopologyPoolID)

// OrderedPoolIDs returns a deduplicated, ordered list of pool UUIDs derived
// from the accessibility requirements, following the CSI spec ordering:
//
//  1. Preferred topologies, in order (SP MUST attempt these first).
//  2. Requisite topologies not already covered by preferred (fallback).
//
// Returns ErrNoPoolInTopology if neither preferred nor requisite carry any
// pool topology segment.
func OrderedPoolIDs(ar *csi.TopologyRequirement) ([]uuid.UUID, error) {
	if ar == nil {
		return nil, ErrNoPoolInTopology
	}

	seen := make(map[uuid.UUID]struct{})
	var result []uuid.UUID

	appendPool := func(v string) error {
		id, err := uuid.FromString(v)
		if err != nil || id == uuid.Nil {
			return fmt.Errorf("invalid pool UUID %q in topology: %w", v, err)
		}
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			result = append(result, id)
		}
		return nil
	}

	// 1. Preferred first (CSI spec: MUST attempt in order).
	for _, topo := range ar.GetPreferred() {
		v, ok := topo.GetSegments()[xok8s.XOLabelTopologyPoolID]
		if !ok || v == "" {
			continue
		}
		if err := appendPool(v); err != nil {
			return nil, err
		}
	}

	// 2. Requisite as fallback (any not already added from preferred).
	for _, topo := range ar.GetRequisite() {
		v, ok := topo.GetSegments()[xok8s.XOLabelTopologyPoolID]
		if !ok || v == "" {
			continue
		}
		if err := appendPool(v); err != nil {
			return nil, err
		}
	}

	if len(result) == 0 {
		return nil, ErrNoPoolInTopology
	}
	return result, nil
}

// ValidatePoolIDAgainstRequisite checks that the given poolID appears in at
// least one requisite topology segment. Returns a descriptive error if not.
//
// If ar is nil or has no requisite entries the check is skipped (returns nil).
func ValidatePoolIDAgainstRequisite(ar *csi.TopologyRequirement, poolID uuid.UUID) error {
	if ar == nil || len(ar.GetRequisite()) == 0 {
		return nil
	}
	poolIDStr := poolID.String()
	for _, topo := range ar.GetRequisite() {
		if v, ok := topo.GetSegments()[xok8s.XOLabelTopologyPoolID]; ok && v == poolIDStr {
			return nil
		}
	}
	return fmt.Errorf("poolId %q does not match any requisite accessibility_requirements topology", poolIDStr)
}

// ErrPoolNotViable is returned when a pool's default SR is not accessible.
var ErrPoolNotViable = errors.New("pool not viable")

// SelectPool iterates the ordered pool UUIDs (as returned by OrderedPoolIDs)
// and returns the first pool whose default SR exists and is accessible.
//
// Per the CSI spec the preferred topologies are tried first (they come first
// in the orderedPoolIDs list), then the requisite topologies as fallback.
// If no viable pool is found, a RESOURCE_EXHAUSTED error is returned.
func SelectPool(ctx context.Context, srClient library.SR, poolClient library.Pool, orderedPoolIDs []uuid.UUID) (*payloads.Pool, error) {
	var lastErr error

	for _, poolID := range orderedPoolIDs {
		pool, err := poolClient.Get(ctx, poolID)
		if err != nil {
			lastErr = fmt.Errorf("pool %q not found or inaccessible: %w", poolID, err)
			continue
		}

		if pool.DefaultSR == uuid.Nil {
			lastErr = fmt.Errorf("pool %q has no default SR configured", poolID)
			continue
		}

		sr, err := srClient.Get(ctx, pool.DefaultSR)
		if err != nil {
			lastErr = fmt.Errorf("pool %q default SR %q not found or inaccessible: %w", poolID, pool.DefaultSR, err)
			continue
		}

		if sr.InMaintenanceMode {
			lastErr = fmt.Errorf("pool %q default SR %q is in maintenance mode", poolID, pool.DefaultSR)
			continue
		}

		// TODO: check that the SR has enough free space to create the requested VDI
		// (sr.Size - sr.Usage >= capacityBytes).

		return pool, nil
	}

	return nil, fmt.Errorf("%w: no viable pool found among candidates: %v", ErrPoolNotViable, lastErr)
}

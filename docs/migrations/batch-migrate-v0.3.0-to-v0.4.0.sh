#!/usr/bin/env bash
set -euo pipefail

PREFIX="${PREFIX:-csi-}"
DRY_RUN="${DRY_RUN:-true}"

migrated=0
skipped=0
errors=0

# Extract a key-value from other-config line (semicolon-separated).
# Usage: xe_get_other_config "output" "kubernetes_pv_name"
xe_get_other_config() {
    local output="$1"
    local key="$2"
    local line
    line=$(echo "$output" | { grep -E "^\s+other-config" || true; } | head -1)
    [[ -z "$line" ]] && return 0
    echo "$line" | tr ';' '\n' | { grep -E "^\s*${key}:" || true; } | head -1 | sed "s/^[[:space:]]*${key}:[[:space:]]*//"
}

# Extract a top-level param value (e.g. name-label, uuid).
xe_get_param() {
    local output="$1"
    local param="$2"
    echo "$output" | { grep -E "^${param} \(" || true; } | head -1 | sed "s/^${param} ([^(]*):[[:space:]]*//"
}

echo "=== v0.3.0 → v0.4.0 batch migration ==="
echo "PREFIX : ${PREFIX}"
echo "DRY_RUN: ${DRY_RUN}"
echo ""

# xe vdi-list --minimal returns comma-separated UUIDs
VDI_LIST=$(xe vdi-list --minimal)
IFS=',' read -ra VDI_ARRAY <<< "$VDI_LIST"

echo "Processing ${#VDI_ARRAY[@]} VDIs..."

for VDI_UUID in "${VDI_ARRAY[@]}"; do
    VDI_UUID=$(echo "$VDI_UUID" | tr -d '[:space:]')
    [[ -z "$VDI_UUID" ]] && continue

    # echo "Processing VDI ${VDI_UUID}..."
    PARAM_OUTPUT=$(xe vdi-param-list uuid="$VDI_UUID" 2>/dev/null || true)

    if [[ -z "$PARAM_OUTPUT" ]]; then
        continue
    fi

    PV_NAME=$(xe_get_other_config "$PARAM_OUTPUT" "kubernetes_pv_name")

    if [[ -z "$PV_NAME" ]]; then
        continue
    fi

    VOLUME_HANDLE=$(xe_get_other_config "$PARAM_OUTPUT" "kubernetes_volume_id")
    CURRENT_NAME=$(xe_get_param "$PARAM_OUTPUT" "name-label")

    if [[ -z "$VOLUME_HANDLE" ]]; then
        echo "[SKIPPED] VDI ${VDI_UUID}: missing kubernetes_volume_id"
        skipped=$((skipped + 1))
        continue
    fi

    NEW_NAME_LABEL="${PREFIX}${VOLUME_HANDLE}-${PV_NAME}"
    VOLUME_HANDLE_TAG="k8s:volumeId:${VOLUME_HANDLE}"
    PV_NAME_TAG="k8s:pvName:${PV_NAME}"

    if [[ "$DRY_RUN" == "true" ]]; then
        actions=()
        if [[ "$CURRENT_NAME" != "$NEW_NAME_LABEL" ]]; then
            actions+=("rename '${CURRENT_NAME}' → '${NEW_NAME_LABEL}'")
        fi
        actions+=("add tag '${VOLUME_HANDLE_TAG}'")
        actions+=("add tag '${PV_NAME_TAG}'")
        echo "[DRY-RUN] VDI ${VDI_UUID}: $(IFS='; '; echo "${actions[*]}")"
        migrated=$((migrated + 1))
        continue
    fi

    action_count=0

    if [[ "$CURRENT_NAME" != "$NEW_NAME_LABEL" ]]; then
        if ! xe vdi-param-set uuid="$VDI_UUID" name-label="$NEW_NAME_LABEL" 2>/dev/null; then
            echo "[ERROR] VDI ${VDI_UUID}: failed to set name-label"
            errors=$((errors + 1))
            continue
        fi
        action_count=$((action_count + 1))
    fi

    if ! xe vdi-param-add uuid="$VDI_UUID" param-name=tags param-key="$VOLUME_HANDLE_TAG" 2>/dev/null; then
        echo "[ERROR] VDI ${VDI_UUID}: failed to add tag '${VOLUME_HANDLE_TAG}'"
        errors=$((errors + 1))
        continue
    fi
    action_count=$((action_count + 1))

    if ! xe vdi-param-add uuid="$VDI_UUID" param-name=tags param-key="$PV_NAME_TAG" 2>/dev/null; then
        echo "[ERROR] VDI ${VDI_UUID}: failed to add tag '${PV_NAME_TAG}'"
        errors=$((errors + 1))
        continue
    fi
    action_count=$((action_count + 1))

    echo "[MIGRATED] VDI ${VDI_UUID} (${action_count} change(s))"
    migrated=$((migrated + 1))
done

echo ""
echo "=== Summary ==="
echo "migrated: ${migrated}"
echo "skipped : ${skipped}"
echo "errors  : ${errors}"

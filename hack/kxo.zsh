#!/usr/bin/env zsh
# kxo - kubectl shorthand for xenorchestra-csi manifests
# Sourced automatically by .autoenv.zsh

# Store dir paths relative to this script (resolved at source time)
_KXO_DEPLOY_DIR="${${(%):-%x}:A:h}/../deploy"
_KXO_EXAMPLES_DIR="${${(%):-%x}:A:h}/../examples"

# Discover manifests dynamically at each call
_kxo_load_manifests() {
  typeset -gA K_MANIFESTS=()
  for f in "${_KXO_DEPLOY_DIR}"/*.yaml(N); do
    local filename="${f:t}"
    local key="${${filename%.yaml}//csi-xenorchestra-/}"
    K_MANIFESTS[$key]="deploy/$filename"
  done
  for f in "${_KXO_EXAMPLES_DIR}"/*.yaml(N); do
    local filename="${f:t}"
    local key="${filename%.yaml}"
    K_MANIFESTS[$key]="examples/$filename"
  done
}

kxo() {
  local cmd="$1"
  shift

  # Refresh manifest list on each call
  _kxo_load_manifests

  # If no arguments, show help
  if [[ -z "$cmd" ]]; then
    echo "Usage: kxo [apply|a|delete|d|get|describe] <manifest-key> [manifest-key...]"
    echo "       kxo create-secret [config-file]"
    echo "       kxo delete-secret"
    echo ""
    echo "Environment variables:"
    echo "  IMAGE        Override the driver container image on apply"
    echo "               e.g. IMAGE=<node-ip>:32000/vatesfr/xenorchestra-csi-driver:dev kxo apply node"
    echo "  CLUSTER_TAG  Override the --cluster-tag arg (default: \$USER)"
    echo "               e.g. CLUSTER_TAG=k8s-prod kxo apply controller"
    echo ""
    echo "Available manifests:"
    for key in "${(@k)K_MANIFESTS}"; do
      echo "  $key → ./${K_MANIFESTS[$key]}"
    done
    return 0
  fi

  # Special commands
  case "$cmd" in
    create-secret)
      local config_file="${1:-xo-config.yaml}"
      if [[ ! -f "$config_file" ]]; then
        echo "Error: $config_file not found"
        return 1
      fi
      kubectl -n kube-system delete secret xenorchestra-cloud-controller-manager --ignore-not-found
      kubectl -n kube-system create secret generic xenorchestra-cloud-controller-manager \
        --from-file=config.yaml="$config_file"
      return $?
      ;;
    delete-secret)
      kubectl -n kube-system delete secret xenorchestra-cloud-controller-manager
      return $?
      ;;
  esac

  # If no manifest keys, show usage
  if [[ $# -eq 0 ]]; then
    echo "Usage: kxo [apply|a|delete|d|get|describe] <manifest-key> [manifest-key...]"
    local -a keys=("${(@k)K_MANIFESTS}")
    echo "Available: ${(j:, :)keys}"
    return 1
  fi

  # Process each manifest key
  local rc=0
  for manifest_key in "$@"; do
    # Check if manifest key exists
    if [[ -z "${K_MANIFESTS[$manifest_key]}" ]]; then
      echo "Unknown manifest: $manifest_key"
      local -a keys=("${(@k)K_MANIFESTS}")
      echo "Available: ${(j:, :)keys}"
      rc=1
      continue
    fi

    local manifest_file="./${K_MANIFESTS[$manifest_key]}"

    # Check if file exists
    if [[ ! -f "$manifest_file" ]]; then
      echo "Error: File not found: $manifest_file"
      rc=1
      continue
    fi

    # Execute kubectl command
    case "$cmd" in
      apply|a)
        local _cluster_tag="${CLUSTER_TAG:-k8s-managed-$USER}"
        sed \
          -e "s|image: .*xenorchestra-csi-driver.*|image: ${IMAGE:-&}|g" \
          -e "s|\"--cluster-tag=[^\"]*\"|\"--cluster-tag=${_cluster_tag}\"|g" \
          "$manifest_file" | kubectl apply -f -
        ;;
      delete|d)
        kubectl delete -f "$manifest_file"
        ;;
      get|describe|edit)
        kubectl "$cmd" -f "$manifest_file"
        ;;
      *)
        kubectl "$cmd" -f "$manifest_file"
        ;;
    esac
    (( $? != 0 )) && rc=$?
  done
  return $rc
}

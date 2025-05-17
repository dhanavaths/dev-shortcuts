#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
source "${SCRIPTDIR}/../shared/colors.sh"

# # # k - get nodes -l "kubernetes.azure.com/agentpool=gen05,scheduler.clusterfleet.io/ready=false"
# reset namespace
namespace="${1}" && [ "$namespace" = "-" ] && namespace="" || namespace="-n $namespace"
resource_type=$2
glob_pattern="${3}" && [ "$glob_pattern" = "-" ] && glob_pattern=""
node_filter=$4

if [ -z "$resource_type" ]; then
  echo "Resource type is required."
  exit 1
fi

function _main() {

  if [ -n "$node_filter" ]; then
    resource_list=$(kubectl $kube_config $namespace get $resource_type --field-selector "spec.nodeName=${node_filter}"  -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n')
  else
    resource_list=$(kubectl $kube_config $namespace get $resource_type -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n')
  fi

  if [ -n "$glob_pattern" ]; then
    resource_list=$(echo "$resource_list" | grep -i -- "$glob_pattern" | nl -v 0)
  else
    resource_list=$(echo "$resource_list" | nl -v 0)
  fi

  echo
  echo "Select the number of the '$resource_type' resource to DELETE:"
  select_item_from_resource_list

  # Check if branch exists and switch to it
  if [ -n "$RESOURCE_LIST_SELECTED_NAME" ]; then

      if [ -z "$kube_config" ]; then
        echo "$RESOURCE_LIST_SELECTED_NAME" | xclip -selection clipboard
      fi

      echo -e -n "${YELLOW}Enter name of the resource to DELETE:${DEFAULTCOLOR} "
      read confirmation_resource_name
      echo "$RESOURCE_LIST_SELECTED_NAME"
      if [ "$RESOURCE_LIST_SELECTED_NAME" = "$confirmation_resource_name" ]; then
        kubectl $kube_config $namespace delete $resource_type $confirmation_resource_name
      else
        echo "Invalid resource name entered to confirm the deletion."
      fi
  else
    echo "Invalid input."
  fi
}

_main $@
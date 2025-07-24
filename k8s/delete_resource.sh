#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
source "${SCRIPTDIR}/../shared/colors.sh"

# k - delete <resource-type> resource
set_common_options "$@"
set -- "${POST_PARSE_ARGS[@]}"
resource_type=$3

if [ -z "$resource_type" ]; then
  echo "Resource type is required."
  exit 1
fi

function _main() {

  if [ -n "${OPT_NODE_NAME_SELECTOR}" ]; then
    resource_list=$(kubectl $kube_config $OPT_NAMESPACE get $resource_type ${OPT_NODE_NAME_SELECTOR} -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n')
  elif [ -n "${OPT_RESOURCE_NAME}" ]; then
    resource_list=$(kubectl $kube_config $OPT_NAMESPACE get $resource_type ${OPT_RESOURCE_NAME} -o jsonpath='{.metadata.name}' | tr ' ' '\n')
  else
    resource_list=$(kubectl $kube_config $OPT_NAMESPACE get $resource_type -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n')
  fi

  if [ -n "${OPT_RESOURCE_NAME_PATTERN}" ]; then
    resource_list=$(echo "$resource_list" | grep -i -- "$OPT_RESOURCE_NAME_PATTERN" | nl -v 0)
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
        kubectl $kube_config $OPT_NAMESPACE delete $resource_type $confirmation_resource_name
      else
        echo "Invalid resource name entered to confirm the deletion."
      fi
  else
    echo "Invalid input."
  fi
}

_main $@
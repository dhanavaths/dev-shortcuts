#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
set_common_options "$@"
set -- "${POST_PARSE_ARGS[@]}"
# namespace="${1}" && [ "$namespace" = "-" ] && namespace="" || namespace="-n $namespace"
resource_verb=$2
resource_kind=$3
#resource_name=$4
var_arguments=${@:5}
resource_list=""

if [ "$resource_verb" != "get" ]; then
  OPT_OUTPUT_FORMAT=""
fi

function _fetch_object() {
  if [ "$1" = "" ]; then
      echo "Invalid input/no objects found."
      exit 0
  fi
  object_name=$1
  echo "kubectl $kube_config $OPT_NAMESPACE $resource_verb $resource_kind $object_name $var_arguments ${OPT_OUTPUT_FORMAT}" >&2
  if [ "${OPT_LESS_MODE}" = "less" ]; then
    kubectl $kube_config $OPT_NAMESPACE $resource_verb $resource_kind $object_name $var_arguments ${OPT_OUTPUT_FORMAT} | less
  elif [ "${OPT_VSCODE_MODE}" = "code" ]; then
    kubectl $kube_config $OPT_NAMESPACE $resource_verb $resource_kind $object_name $var_arguments ${OPT_OUTPUT_FORMAT} | code -
  else
    kubectl $kube_config $OPT_NAMESPACE $resource_verb $resource_kind $object_name $var_arguments ${OPT_OUTPUT_FORMAT}
  fi
  echo
}



function _handle_user_input() {
    if [ -n "${OPT_RESOURCE_NAME_PATTERN}" ]; then
      if [ "${OPT_OUTPUT_FORMAT}" == "-owide" ]; then
        exit_on_empty_or_single_item_in_resource_list
      fi
    fi

    exit_on_empty_resource_list
    select_item_from_resource_list $resource_kind
    _fetch_object $RESOURCE_LIST_SELECTED_NAME
    exit_on_empty_or_single_item_in_resource_list
}

function _main() {
  if [ -n "${OPT_ALL_NAMESPACES}" ]; then
    kubectl $kube_config get $resource_kind ${OPT_ALL_NAMESPACES} ${OPT_LABEL_SELECTOR} -owide
    if [ "${OPT_OUTPUT_FORMAT}" == "-oyaml" ]; then
      kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_ALL_NAMESPACES} ${OPT_LABEL_SELECTOR} -oyaml | code -
    fi
    return
  elif [[ -n "${OPT_RESOURCE_NAME_PATTERN}" ]]; then
      echo "kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide" >&2
      local _output=$(kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide | awk 'NR==1; /'"${OPT_RESOURCE_NAME_PATTERN}"'/')
      echo "$_output"
      resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)
  elif [[ -n "${OPT_RESOURCE_NAME}" ]]; then
      if [ -z "${OPT_OUTPUT_FORMAT}" ]; then
        echo "kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_RESOURCE_NAME} ${OPT_SORT_BY} -owide" >&2
        kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_RESOURCE_NAME} ${OPT_SORT_BY} -owide
      fi
      resource_list=$(echo "${OPT_RESOURCE_NAME}" | nl -v 0)
  else
    echo "kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide" >&2
    local _output=$(kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide)
    echo "$_output"
    resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)
    # resource_list=$(kubectl $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | nl -v 0)
  fi

  while true; do
    _handle_user_input
  done
}

_main
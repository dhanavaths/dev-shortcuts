#!/bin/bash
# klocal <ns> <resource_kind:pod|svc> [-/<pod/svc_name>] <port>
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
set_common_options "$@"
set -- "${POST_PARSE_ARGS[@]}"

resource_kind="$3"
port_to_forward=${5:-$4}

function _port_forward() {
  if [ "$1" = "" ]; then
      echo "Invalid input/no objects found."
      exit 0
  fi
  object_name=$1
  echo "$object_name" | xclip -selection clipboard
  echo "kubectl $kube_context $kube_config $OPT_NAMESPACE port-forward $resource_kind/$object_name $port_to_forward:$port_to_forward" >&2
  kubectl $kube_context $kube_config $OPT_NAMESPACE port-forward $resource_kind/$object_name $port_to_forward:$port_to_forward
}



function _handle_user_input() {
    exit_on_empty_resource_list
    select_item_from_resource_list $resource_kind
    _port_forward $RESOURCE_LIST_SELECTED_NAME
    exit 0
}

function _main() {
    if [ -n "${OPT_RESOURCE_NAME}" ]; then
        _port_forward "${OPT_RESOURCE_NAME}"
        exit 0
    fi
    if [ -n "${OPT_RESOURCE_NAME_PATTERN}" ]; then
      echo "kubectl $kube_context $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide" >&2
      local _output=$(kubectl $kube_context $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide | awk 'NR==1; /'"${OPT_RESOURCE_NAME_PATTERN}"'/')
      echo "$_output"
      resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0) 
    else
      echo "kubectl $kube_context $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide" >&2
      local _output=$(kubectl $kube_context $kube_config $OPT_NAMESPACE get $resource_kind ${OPT_NODE_NAME_SELECTOR} ${OPT_LABEL_SELECTOR} ${OPT_SORT_BY} -owide)
      echo "$_output"
      resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0) 
    fi
    _handle_user_input
}

_main
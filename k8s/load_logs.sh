#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"

namespace="${1}" && [ "$namespace" = "-" ] && namespace="" || namespace="-n $namespace"
pod_name="${2}" && [ "$pod_name" = "-" ] && pod_name=""
search_pattern=""
var_arguments=""
resource_list=""

_log_format='"\u001b[32m[\(.time)]\u001b[0m\u001b[34m[\(.level)]\u001b[0m \u001b[37m\(.msg)\u001b[0m \u001b[33m[\(.file)]\u001b[0m \u001b[90m[\(.func)]\u001b[0m"'

function _fetch_pod_logs() {
  if [ "$1" = "" ]; then
      exit 0
  fi
  echo "kubectl $kube_config $namespace logs $1 $var_arguments" >&2

  if [ -n "$search_pattern" ]; then
    kubectl $kube_config $namespace logs $1 $var_arguments | jq -r "$_log_format" | grep -i -E "$search_pattern" 2>/dev/null
  else
    kubectl $kube_config $namespace logs $1 $var_arguments | jq -r "$_log_format" 2>/dev/null
  fi

  if [ $? -ne 0 ]; then
    kubectl $kube_config $namespace logs $1 $var_arguments
  fi
  echo
}

function _handle_user_input() {
    select_item_from_resource_list "POD"
    _fetch_pod_logs $RESOURCE_LIST_SELECTED_NAME
    exit_on_single_item_in_resource_list
}


function _main()
{
  local idx=3 
  if [ -z "$pod_name" ]; then
    resource_list=$(kubectl $kube_config get pod $namespace -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | nl -v 0)
  else
    if  [[ "$pod_name" == \** ]]; then
      pod_name=$(echo $pod_name | sed 's/\*//g')
      echo "kubectl $kube_config $namespace get pod  -owide | awk 'NR==1; /'\"$pod_name\"'/'" >&2
      local _output=$(kubectl $kube_config $namespace get pod -owide | awk 'NR==1; /'"$pod_name"'/')
      echo "$_output"
      resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)
    else
      resource_list=$(echo "$pod_name" | nl -v 0)
    fi
  fi

  exit_on_empty_resource_list

  local _next_arg="${@:$idx:1}"
  if [ ! "$_next_arg" = "-f" ]; then
    search_pattern="${@:$idx:1}"
    idx=$((idx + 1))
  fi

  var_arguments=${@:$idx}

  while true; do
    if [ -z "$kube_config" ]; then
      kubectl $namespace get lease 2>/dev/null
    fi
    _handle_user_input
  done
}

_main "$@"

#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
set_common_options "$@"
set -- "${POST_PARSE_ARGS[@]}"
for arg in "$@"; do
  echo "arg: $arg"
done
# namespace="${1}" && [ "$namespace" = "-" ] && namespace="" || namespace="-n $namespace"
var_arguments="${@:5}"
echo "var_arguments: $var_arguments"
resource_list=""

_go_runtime_log_format='try "\u001b[32m[\(.time)]\u001b[0m\u001b[34m[\(.level)]\u001b[0m \u001b[37m\(.msg)\u001b[0m \u001b[33m[\(.file)]\u001b[0m \u001b[90;2m[\(.func | split("/")[-1])]\u001b[0m" catch empty'

function _fetch_pod_logs() {
  if [ "$1" = "" ]; then
      exit 0
  fi
  echo "kubectl $kube_config $OPT_NAMESPACE logs $1 $var_arguments" >&2

  if [ -n "$OPT_SEARCH_PATTERN" ]; then
    if [ "$OPT_LANGUAGE_RUNTIME" = "go" ]; then
      kubectl $kube_config $OPT_NAMESPACE logs $1 $var_arguments | tail -n +2 | jq -r "$_go_runtime_log_format" | grep -i -E -- "$OPT_SEARCH_PATTERN" 2>/dev/null
    else
      kubectl $kube_config $OPT_NAMESPACE logs $1 $var_arguments | grep -i -E -- "$OPT_SEARCH_PATTERN" | sed 's/\(Message\)/\x1b[1;36m\1\x1b[0m/g' 2>/dev/null
    fi
  else
    if [ "$OPT_LANGUAGE_RUNTIME" = "go" ]; then
      kubectl $kube_config $OPT_NAMESPACE logs $1 $var_arguments | jq -r "$_go_runtime_log_format" 2>/dev/null
    elif [ "$OPT_LANGUAGE_RUNTIME" = "dotnet" ]; then
      kubectl $kube_config $OPT_NAMESPACE logs $1 $var_arguments | sed 's/\(Message\)/\x1b[1;34m\1\x1b[0m/g' 
    else
      kubectl $kube_config $OPT_NAMESPACE logs $1 $var_arguments 2>/dev/null
    fi
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
  echo "OPT_RESOURCE_NAME_PATTERN: $OPT_RESOURCE_NAME_PATTERN"
  if  [[ -n "${OPT_RESOURCE_NAME_PATTERN}" ]]; then
    echo "kubectl $kube_config $OPT_NAMESPACE get pod  -owide | awk 'NR==1; /'\"${OPT_RESOURCE_NAME_PATTERN}\"'/'" >&2
    local _output=$(kubectl $kube_config $OPT_NAMESPACE get pod -owide | awk 'NR==1; /'"${OPT_RESOURCE_NAME_PATTERN}"'/')
    echo "$_output"
    resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)
  elif [ -n "${OPT_RESOURCE_NAME}" ]; then
      resource_list=$(echo "${OPT_RESOURCE_NAME}" | nl -v 0)
  else
    echo "kubectl $kube_config $OPT_NAMESPACE get pod  -owide" >&2
    local _output=$(kubectl $kube_config $OPT_NAMESPACE get pod -owide )
    echo "$_output"
    resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)

  fi

  exit_on_empty_resource_list

  while true; do
    if [ -z "$kube_config" ]; then
      kubectl $OPT_NAMESPACE get lease 2>/dev/null
    fi
    _handle_user_input
  done
}

_main "$@"

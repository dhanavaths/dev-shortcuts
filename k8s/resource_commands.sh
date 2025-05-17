#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
namespace="${1}" && [ "$namespace" = "-" ] && namespace="" || namespace="-n $namespace"
resource_verb=$2
resource_kind=$3
resource_glob="${4}" && [ "$resource_glob" = "-" ] && resource_glob=""

node_name_selector=""
idx=6
if [[ "$5" == aksgen* ]] || [[ "$5" == aks-gen* ]] || [[ "$5" == aks-sys* ]] ; then
  node_name_selector=" --field-selector spec.nodeName=$5"
  output_format="${6}" && [ "$output_format" = "-" ] && output_format=""
  idx=7
else
  output_format="${5}" && [ "$output_format" = "-" ] && output_format=""
fi

output_mode=""
all_namespaces=$(has_all_namespace_option $@)

var_arguments=${@:$idx}
resource_list=""

all_output_formats="-owide -oyaml -ojson -less -oless -code -ocode"

declare -A extended_format_mapping
extended_format_mapping["-oless"]="less"
extended_format_mapping["-less"]="less"
extended_format_mapping["-ocode"]="code"
extended_format_mapping["-code"]="code"

function _fetch_object() {
  if [ "$1" = "" ]; then
      echo "Invalid input/no objects found."
      exit 0
  fi
  object_name=$1
  echo "kubectl $kube_config $namespace $resource_verb $resource_kind $object_name $var_arguments $output_format" >&2
  if [ "$output_mode" = "less" ]; then
    kubectl $kube_config $namespace $resource_verb $resource_kind $object_name $var_arguments $output_format | less
  elif [ "$output_mode" = "code" ]; then
    kubectl $kube_config $namespace $resource_verb $resource_kind $object_name $var_arguments $output_format | code -
  else
    kubectl $kube_config $namespace $resource_verb $resource_kind $object_name $var_arguments $output_format
  fi
  echo
}



function _handle_user_input() {
    exit_if_futher_object_lookup_not_required
    select_item_from_resource_list $resource_kind
    _fetch_object $RESOURCE_LIST_SELECTED_NAME
    exit_on_single_item_in_resource_list
}

function exit_if_futher_object_lookup_not_required() {
  exit_on_empty_resource_list
  if [ "$resource_verb" = "get" ] && [ -z "$output_format" ]; then
    exit 0
  fi

  if [ "$output_format" == "-owide" ]; then
    exit 0
  fi
}

function _main() {
  for om in $all_output_formats; do
    if [[ "$om" = "$resource_glob" ]]; then
      output_format="$resource_glob"
      resource_glob=""
      break
    fi
  done


  if [[ -v extended_format_mapping["$output_format"] ]]; then
    output_mode="${extended_format_mapping[$output_format]}"
    if [ "$resource_verb" == "describe" ]; then
      output_format=""
    else
      output_format="-oyaml"
    fi
  fi

  if [ "$resource_glob" == "-A" ]; then
    resource_glob=""
  fi

  if [ -n "$all_namespaces" ]; then
    kubectl $kube_config $namespace get $resource_kind $all_namespaces -owide
    if [ "$output_format" == "-oyaml" ]; then
      kubectl $kube_config $namespace get $resource_kind $all_namespaces -oyaml | code -
    fi
    return
  elif [[ -z "$resource_glob" ]]; then
    echo "kubectl $kube_config $namespace get $resource_kind $node_name_selector -owide" >&2
    local _output=$(kubectl $kube_config $namespace get $resource_kind $node_name_selector -owide)
    echo "$_output" >&2
    resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)
    resource_list=$(kubectl $kube_config $namespace get $resource_kind $node_name_selector -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | nl -v 0)
  else
    if  [[ "$resource_glob" == \** ]]; then
      resource_glob=$(echo $resource_glob | sed 's/\*//g')
      echo "kubectl $kube_config $namespace get $resource_kind $node_name_selector -owide" >&2
      local _output=$(kubectl $kube_config $namespace get $resource_kind $node_name_selector -owide | awk 'NR==1; /'"$resource_glob"'/')
      echo "$_output" >&2
      resource_list=$(echo "$_output" | awk 'NR > 1 {print $1}' | nl -v 0)      
    else
      if [ -z "$output_format" ]; then
        echo "kubectl $kube_config $namespace get $resource_kind $resource_glob -owide" >&2
        kubectl $kube_config $namespace get $resource_kind $resource_glob -owide
      fi
      resource_list=$(echo "$resource_glob" | nl -v 0)
    fi
  fi

  while true; do
    _handle_user_input
  done
}

_main
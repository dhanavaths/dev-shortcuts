#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"
source "${SCRIPTDIR}/../shared/colors.sh"

namespace=$1
action=$2
filter=$3
display_items=""

function _main() {
  # if no filter is provided for prod kube contexts, just print all contexts and exit.
  if [ -z "$filter" ] && [ -n "$kube_config" ]; then
    kubectl $kube_config config get-contexts
    exit 0
  fi

  if [ ${#filter} -le 6 ]; then
    filter="$filter-"
  fi

  cur_context=$(kubectl $kube_config config current-context)
  if [ -z "$filter" ]; then
    items=$(kubectl $kube_config config get-contexts | grep -v CURRENT | sed 's/^[ \*\t]*//' | cut -d' ' -f1 | sort)
  else
    items=$(kubectl $kube_config config get-contexts  | grep -- "$filter" | sed 's/^[ \*\t]*//' | cut -d' ' -f1 | sort)
  fi 

  if [ -n "$items" ]; then
    display_items=$(echo "${items}" | awk -v cur_context="$cur_context" '{print $0 == cur_context ? "*\t" $0 : "\t" $0}')
  fi

  resource_list=$(echo -n "${items}" | nl -v 0)
  resource_display_list=$(echo -n "${display_items}" | nl -v 0)
  select_item_from_resource_list "Context"
  item_name=$(echo "$RESOURCE_LIST_SELECTED_NAME" | tr '*' ' ')
  if [ -n "$item_name" ]; then
    kubectl $kube_config config $action $item_name
  else
    echo "Invalid input. Please try again."
  fi
  echo
}

_main $@

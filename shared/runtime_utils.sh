#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/colors.sh"
source "${SCRIPTDIR}/../shared/utils.sh"
source "${SCRIPTDIR}/../shared/options_parser.sh"

RESOURCE_LIST_SELECTED_NAME=""
RESOURCE_LIST_COUNT="0"

function exit_on_empty_resource_list() {
    trimmed_resource_list=$(echo "$resource_list" | tr -d '\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    if [ -z "$trimmed_resource_list" ]; then
      echo -e "${YELLOW}No matching resource found!${DEFAULTCOLOR}"
      exit 0
    fi
}

function select_item_from_resource_list() {
  RESOURCE_LIST_SELECTED_NAME=""
  RESOURCE_LIST_COUNT="0"
	exit_on_empty_resource_list
  RESOURCE_LIST_COUNT=$(echo "$resource_list" | wc -l)
	if [ -n "$1" ]; then
	    echo "Select $1"
    fi

  if [ -n "$resource_display_list" ]; then
    echo "$resource_display_list"
  else
    echo "$resource_list"
  fi
	echo

	local resource_number=""

    if [ "$RESOURCE_LIST_COUNT" = "1" ]; then
      resource_number="0"
    else
      # Read user input
      read -p "Enter row number: " resource_number
    fi


    if [ "$resource_number" = "q" ] || [ -z "$resource_number" ]; then
      exit 0
    fi

    # Get branch name from selected number
    RESOURCE_LIST_SELECTED_NAME=$(echo "$resource_list" | tr '*' ' ' | awk -v num="$resource_number" '$1 == num {print $2}')
}

function exit_on_empty_or_single_item_in_resource_list() {
  exit_on_empty_resource_list

  if [ -n "${OPT_WATCH_MODE}" ]; then
    # reset watch mode
    export OPT_WATCH_MODE=""
    return
  fi

  RESOURCE_LIST_COUNT=$(echo "$resource_list" | wc -l)
	if [ "$RESOURCE_LIST_COUNT" = "1" ]; then
	  exit 0
	fi
}

#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/colors.sh"
source "${SCRIPTDIR}/../shared/utils.sh"

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

function exit_on_single_item_in_resource_list() {
	if [ "$RESOURCE_LIST_COUNT" = "1" ]; then
	  exit 0
	fi
}

function has_all_namespace_option() {
  for arg in "$@"; do
    if [ "$arg" = "--all-namespaces" ] || [ "$arg" = "-A" ]; then
      echo "-A"
      break
    fi
  done
  echo ""
}

OPT_NAMESPACE=""
OPT_ALL_NAMESPACES=""
OPT_NODE_NAME_SELECTOR=""
OPT_RESOURCE_NAME=""
OPT_RESOURCE_NAME_PATTERN=""
OPT_SEARCH_PATTERN=""
OPT_OUTPUT_FORMAT=""
OPT_SORT_BY="--sort-by=.metadata.name"
OPT_LESS_MODE=""
OPT_VSCODE_MODE=""
INPUT_COMMAND_ARGS=""
POST_PARSE_ARGS=""
OPT_LABEL_SELECTOR_GIVEN=""

function set_common_options() {
  INPUT_COMMAND_ARGS="$@"
  OPT_NAMESPACE=$(get_namespace_extended "$1")

  args=("$1")
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -s | --sort-by)
        case "$2" in
          a|ts|age)
            OPT_SORT_BY="--sort-by=.metadata.creationTimestamp"
            ;;
          -)
            OPT_SORT_BY=""
            ;;
          *)
            OPT_SORT_BY="--sort-by=$2"
            ;;
        esac
        shift 2
        ;;
      -nn | --node-name)
        OPT_NODE_NAME_SELECTOR="--field-selector spec.nodeName=$2"
        shift 2
        ;;
      -p | -g | -r | --search-pattern | --search)
        OPT_SEARCH_PATTERN="$2"
        shift 2
        ;;
      -A | --all-namespaces)
        OPT_ALL_NAMESPACES="-A"
        shift
        ;;
      -oless | -less)
        OPT_LESS_MODE="less"
        if [ -z "$OPT_OUTPUT_FORMAT" ]; then
          OPT_OUTPUT_FORMAT="-oyaml"
        fi
        shift
        ;;
      -ocode | -code)
        OPT_VSCODE_MODE="code"
        if [ -z "$OPT_OUTPUT_FORMAT" ]; then
          OPT_OUTPUT_FORMAT="-oyaml"
        fi
        shift
        ;;
      -oyaml)
        OPT_OUTPUT_FORMAT="-oyaml"
        shift
        ;;
      -ojson)
        OPT_OUTPUT_FORMAT="-ojson"
        shift
        ;;
      -l)
        args+=("$1")
        args+=("$2")
        shift 2
        OPT_LABEL_SELECTOR_GIVEN="true"
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  if [ -z "${OPT_OUTPUT_FORMAT}" ]; then
    OPT_OUTPUT_FORMAT="-owide"
  fi


  if [ "${#args[@]}" -gt 3 ]; then
    if  [[ "${args[3]}" == "-" ]]; then 
      echo -n ""
    elif [[ -n "$OPT_LABEL_SELECTOR_GIVEN" ]]; then
      echo "name cannot be provided when a selector is specified"
      exit 0
    elif  [[ "${args[3]}" == \** ]]; then
      OPT_RESOURCE_NAME_PATTERN=$(echo "${args[3]}" | sed 's/\*//g')
    else
      OPT_RESOURCE_NAME="${args[3]}"
    fi
  fi

  POST_PARSE_ARGS=("${args[@]}")
  export OPT_NAMESPACE
  export OPT_ALL_NAMESPACES
  export OPT_NODE_NAME_SELECTOR
  export OPT_RESOURCE_NAME
  export OPT_RESOURCE_NAME_PATTERN
  export OPT_SEARCH_PATTERN
  export OPT_OUTPUT_FORMAT
  export OPT_SORT_BY
  export OPT_LESS_MODE
  export OPT_VSCODE_MODE
  export INPUT_COMMAND_ARGS
  export POST_PARSE_ARGS
}

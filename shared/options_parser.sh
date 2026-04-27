SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../bash/config.sh"

OPT_NAMESPACE=""
OPT_NAMESPACE_NAME=""
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
OPT_LABEL_SELECTOR=""
OPT_LANGUAGE_RUNTIME=""
OPT_WATCH_MODE=""
OPT_HAS_DIRECT_COMMAND=""

function set_common_options() {
  if [ "$2" == '--' ]; then
    export POST_PARSE_ARGS="$@"
    return
  fi

  INPUT_COMMAND_ARGS="$@"
  OPT_NAMESPACE=$(get_namespace_extended "$1")
  OPT_NAMESPACE_NAME=$(get_namespace "$1")

  args=("$1")
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --)
        OPT_HAS_DIRECT_COMMAND="true"
        shift
        ;;
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
        OPT_LABEL_SELECTOR="$1 $2"
        shift 2
        OPT_LABEL_SELECTOR_GIVEN="true"
        ;;
      -go)
        OPT_LANGUAGE_RUNTIME="go"
        shift
        ;;
      -net)
        OPT_LANGUAGE_RUNTIME="dotnet"
        shift
        ;;
      -w | --watch)
        OPT_WATCH_MODE="true"
        args+=("$1")
        shift
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
    if [[ "${args[3]}" == "-" ]] || [[ -n "${OPT_HAS_DIRECT_COMMAND}" ]]; then 
        echo -n ""
    elif [[ -n "$OPT_LABEL_SELECTOR_GIVEN" ]]; then
      echo "exiting, name cannot be provided when a selector is specified. If you want to use a name, please pass on - for resource name."
      exit 0
    elif  [[ "${args[3]}" == \** ]]; then
      OPT_RESOURCE_NAME_PATTERN=$(echo "${args[3]}" | sed 's/\*//g')
    else
      OPT_RESOURCE_NAME="${args[3]}"
    fi
  fi

  if [ -n "${OPT_HAS_DIRECT_COMMAND}" ]; then 
    POST_PARSE_ARGS="$@"
  else
    POST_PARSE_ARGS=("${args[@]}")
  fi

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
  export OPT_WATCH_MODE
  export OPT_LABEL_SELECTOR
}

#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/utils.sh"
source "${SCRIPTDIR}/../shared/colors.sh"

function confirm_on_kubectl_crud_operation() {
	local user_input=""
	for arg in $@; do
		if [ "$arg" == "delete" ] || [ "$arg" == "apply" ] || [ "$arg" == "create" ] || [ "$arg" == "edit" ] || [ "$arg" == "patch" ] || [ "$arg" == "replace" ] || [ "$arg" == "restart" ]; then
			if [ -n "$kube_config" ]; then
				echo -e "$YELLOW** kubectl $@ $DEFAULTCOLOR" >&2
				echo -e -n "$RED** Sure to run this action ($arg)?$DEFAULTCOLOR $GREEN(input Y to confirm)$DEFAULTCOLOR: " >&2
				read user_input
				if [ "$user_input" = "Y" ]; then
					return
				else
					echo "kube command is DENIED to put/patch! Edit bashrc to allow kube to do changes!" >&2
					exit 1
				fi
				return
			fi
		fi
	done
}

function kpresource()
{
	confirm_on_kubectl_crud_operation $@
	local idx=4
	local resource=""
	case $3 in
		l)
			resource=lease
			;;
		d)
			resource=deploy
			;;
		p)
			resource=pod
			;;
		s)
			resource=secrets
			;;
		pdb)
			resource=PodDisruptionBudget
			;;
		app)
			local _app_name=$5
			local _attr_path=$6
			if [ "$_attr_path" == "-owide" ]; then
				_attr_path=""
			fi

			if [ "$4" = "status" ]; then				
				echo "kubectl $kube_config -n $1 get app ${_app_name} -ojson" >&2
				kubectl $kube_config -n $1 get app ${_app_name} -ojson > /tmp/_app.json
				python3 ${SCRIPTDIR}/../pyscripts/parse_application_status.py
				return
			elif [ "$4" = "clusters" ]; then
				local _object
				local _clusters
				_object=$(kubectl $kube_config -n $1 get app ${_app_name} -ojson)
				_clusters=$(echo "$_object" | jq -r '.status.clusters[].cluster')
				echo "$_clusters"
				return
			elif [ "$4" = "mw" ]; then
				local _object
				local _uid
				local _clusters		
				
				_object=$(kubectl $kube_config -n $1 get app ${_app_name} -ojson)
				_clusters=$(echo "$_object" | jq -r '.status.clusters[].cluster')
				_uid=$(echo "$_object" | jq -r '.metadata.uid')
				local i=0
				local _output
				if [ -z $_attr_path ]; then
					kubectl $kube_config get mw -A -l "apis.clusterfleet.io/work=$_uid" -owide
				else
					for cl in $_clusters; do
						_output=$(kubectl $kube_config -n std-cluster-$cl get mw "$1.${_app_name}" -ojson | jq -r ".items[] | $_attr_path")
						#echo "$cl $_output"
					done
				fi
				return
			fi
			idx=3
			;;
		*)
			idx=3
			;;
	esac
	if [ "$1" = "-" ]; then
		echo "kubectl $kube_config $2 $resource ${@:$idx}" >&2
		kubectl $kube_config $2 $resource ${@:$idx}
	else
		echo "kubectl $kube_config -n $1 $2 $resource ${@:$idx}" >&2
		kubectl $kube_config -n $1 $2 $resource ${@:$idx}
	fi
}

function is_app_custom_subresource() {
	if [[ "$1" == "status" ]]; then
		return 1
	elif [[ "$1" == "clusters" ]]; then
		return 1
	elif [[ "$1" == "mw" ]]; then
		return 1
	else
		return 0
	fi
}

function kpverb()
{
	local wide_output=""
	local given_namespace=$1
	local _ns="${1}" && [ "$_ns" = "-" ] && _ns="" || _ns="-n $_ns"

	extended_output=$(is_extended_output $@)
	if [[ "$extended_output" != "0" ]]; then
		wide_output="-oyaml"
	elif [[ "$@" != *" -o"* ]]; then
		if [[ "$@" == *" all"* ]]; then
			wide_output="-owide"
		else
			wide_output="-owide --sort-by=.metadata.name"
		fi
	fi

	case $2 in
		--)
			kpresource ${1} ${@:3}
			return
			;;
		namespaced)
			local _crd_types
			_crd_types=$(kubectl $kube_config api-resources --verbs=list --namespaced -o name | grep -v "events.events.k8s.io" | grep -v "events" | sort | uniq)
			_crd_types=$(echo "$_crd_types"  | tr '\n' ' ' | sed 's/^ *//;s/ *$//' | tr ' ' ',')
			kubectl $kube_config -n $1 get $_crd_types
			;;
		get)
			if [ -z "$3" ]; then
				echo "get     : k <ns> get <resource> [*<pattern>] [<nodename>] [-code|-less default => -owide]"
				return
			fi

			is_app_custom_subresource $4
			local _result="$?"
			if [[ "$3" = "app" ]] && [[ "$_result" = "1" ]] ; then
				kpresource $1 get ${@:3}
				return
			fi

			${SCRIPTDIR}/../k8s/resource_commands.sh $1 get ${@:3}
			;;
		describe|desc)
			if [[ -z "$3" ]]; then
				echo "describe: k <ns> [describe|desc] <resource> [*<pattern>] [<nodename>] [-code|-less default => -owide]"
				return
			fi			
			${SCRIPTDIR}/../k8s/resource_commands.sh $1 describe $3 $4
			;;
		logs)
			if [[ -z "$3" ]]; then
				echo "logs    : k <ns> logs <pod-name-pattern> [search-string-pattern] [-f]"
				return
			fi
			${SCRIPTDIR}/../k8s/load_logs.sh $given_namespace ${@:3}
			;;
		exec)
			if [[ -z "$3" ]]; then
				echo "exec    : k <ns> exec <pod-name> [ps|sh| --woth-args]"
				return
			fi
			if [[ -z "$4" ]]; then
				kpresource $1 exec -it $3 -- bash
			elif [[ "$4" = "ps" ]]; then
				kpresource $1 exec -it $3 -- powershell
			elif [[ "$4" = "sh" ]]; then
				kpresource $1 exec -it $3 -- sh
			else
				kpresource $1 exec -it $3 ${@:4}
			fi
			;;
		df)
			${SCRIPTDIR}/../fleet/datafolder_objects.sh $1 $3
			;;
		delete)
			if [ -z "$3" ]; then
				echo "delete  : k <ns> delete <resource> [< node-name|- >] [pattern]"
				return
			fi
			confirm_on_kubectl_crud_operation $@
			${SCRIPTDIR}/../k8s/delete_resource.sh $1 ${@:3}
			;;
		restart)
			if [ -z "$3" ] || [ -z "$4" ] ; then
				echo "restart : k <ns> restart <resource-type> <resource-name>"
				return
			fi
			confirm_on_kubectl_crud_operation $@
			case $3 in
				scheduler)
					if [ -n "$kube_config" ]; then
						echo "Cannot use production context to restart dev components!"
						exit 0
					fi
					kubectl $kube_config $_ns rollout restart deployment scheduler-deployment
					;;
				*)
					kubectl $kube_config $_ns rollout restart $3 $4
					;;
			esac			
			;;
		apply)
			if [ -z "$3" ]; then
				echo "apply   : k <ns> apply -f <file-path>"
				return
			fi
			confirm_on_kubectl_crud_operation $@
			case $3 in
				scheduler)
					if [ -n "$kube_config" ]; then
						echo "Cannot use production context to apply dev components!"
						exit 0
					fi
					kubectl $kube_config $_ns apply -f hack/deployments/kind-scheduler.yaml
					;;
				demo-app-pr|demo-app-ds|demo-app-ss|demo-app)
					if [ -n "$kube_config" ]; then
						echo "Cannot use production context to apply dev components!"
						exit 0
					fi
					echo "kubectl $kube_config $_ns apply -f ~/scripts/demo-apps/yaml/${3}.yaml" >&2
					kubectl $kube_config $_ns apply -f ~/scripts/demo-apps/yaml/${3}.yaml
					;;
				*)
					kpresource $@
					;;
			esac
			;;
		clsub|subscription)
			echo "kubectl $kube_config $_ns get cl -o custom-columns='NAME:.metadata.name,SUBSCRIPTION_ID:.spec.properties.subscriptionId'" >&2
			local _output=$(kubectl $kube_config $_ns get cl -o custom-columns='NAME:.metadata.name,SUBSCRIPTION_ID:.spec.properties.subscriptionId')
			if [ -n "$3" ]; then
				echo "$_output" | grep $3
			else
				echo "$_output"
			fi
			;;
        fdes)
            echo "kubectl $kube_config $_ns get endpointslices -o custom-columns='NAME:.metadata.name, SOURCECLUSTER:.metadata.labels.multicluster\.kubernetes\.io/source-cluster'" >&2
            local _output=$(kubectl $kube_config $_ns get endpointslices -o custom-columns='NAME:.metadata.name, SOURCECLUSTER:.metadata.labels.multicluster\.kubernetes\.io/source-cluster')
            if [ -n "$3" ]; then
                echo "$_output" | grep $3
            else
                echo "$_output"
            fi
            ;;
		help)
			kpverb - get
			kpverb - logs
			kpverb - delete
			kpverb - describe
			kpverb - apply
			kpverb - exec
			kpverb - restart
			echo "cl SubID: k - clsub"
			echo "namespaced objects: k <ns> namespaced"
			;;
		*)
			kpresource $@
			;;
	esac

}

function _kpnamespace()
{
	local _namespace=$(get_namespace $1)
	local _filter=""
	case $_namespace in
		all)
			if [[ -z "$2" ]]; then
				echo "help: k all [pattern]"
				exit 1
			fi

			echo "kubectl ${kube_config} get $2 -A -owide --sort-by=.metadata.name" >&2
			if [ -n "$3" ]; then
				kubectl ${kube_config} get $2 -A -owide --sort-by=.metadata.name | awk {'print $1 " " $2 " " $3 " " $4 " " $5 " " $6 " " $7'} | column -t | { head -n 1; tail -n +2 | grep -- $3; }
			else
				kubectl ${kube_config} get $2 -A -owide --sort-by=.metadata.name | awk {'print $1 " " $2 " " $3 " " $4 " " $5 " " $6 " " $7'} | column -t
			fi
			;;
		staleips)
			echo "namespace: submariner-k8s-broker"
			echo "cluster: $2"
			echo "kubectl ${kube_config} -n submariner-k8s-broker get endpointslices -l \"multicluster.kubernetes.io/source-cluster=$2\"" >&2
			echo
			kubectl ${kube_config} -n submariner-k8s-broker get endpointslices -l "multicluster.kubernetes.io/source-cluster=$2"
			echo
			echo "kubectl ${kube_config} -n submariner-k8s-broker get endpointslices " >&2
			;;
		*)
			kpverb $_namespace ${@:2}
			;;
	esac
}

_kpnamespace $@
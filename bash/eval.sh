#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/config.sh"
source "${SCRIPTDIR}/../shared/utils.sh"

function show_help() {
	if [ -z "$2" ]; then
		echo "commands format: <l|k> <namespace> <verb> <resource> [resource_name] [options]"
		execute_script_with_local_context ${SCRIPTDIR}/../kubectl/commands.sh - help
		exit 0
	fi
}

function set_prod_kube_cofig()
{
	export kube_config="--kubeconfig $prod_kube_config_file_path"
}

function reset_kube_cofig()
{
	export kube_config=""
}

function execute_script()
{
	local script_path=$1
	local namespace=$(get_namespace $2)
    ${script_path} $namespace ${@:3}
}

function execute_script_with_prod_context()
{
    set_prod_kube_cofig
    execute_script $@
    reset_kube_cofig
}

function execute_script_with_local_context()
{
    reset_kube_cofig
    execute_script $@
}


function handle_prod_kube_command()
{
    case $1 in
		kuc)
			execute_script_with_prod_context ${SCRIPTDIR}/../kubectl/kube_context.sh - use-context ${@:2}
			;;
		kgc)
			execute_script_with_prod_context ${SCRIPTDIR}/../kubectl/kube_context.sh - get-contexts ${@:2}
			;;
		kdc)
			execute_script_with_prod_context ${SCRIPTDIR}/../kubectl/kube_context.sh - delete-context ${@:2}
			;;
		dlkube)
			execute_script_with_prod_context ${SCRIPTDIR}/../kubectl/download_kube.sh ${@:2}
			;;
		*)
			show_help $@
			execute_script_with_prod_context ${SCRIPTDIR}/../kubectl/commands.sh $@
			;;
	esac
}

function handle_local_k_command() {
    # safety measure
	reset_kube_cofig

	case $1 in
		kuc)
			execute_script_with_local_context ${SCRIPTDIR}/../kubectl/kube_context.sh - use-context ${@:2}
			;;
		kgc)
			execute_script_with_local_context ${SCRIPTDIR}/../kubectl/kube_context.sh - get-contexts ${@:2}
			;;
		krc)
			kind get clusters | awk '{}{print "kind export kubeconfig --name " $$0}{}' | sh
			cluster_list=$(kind get clusters | awk '{print "kind-" $0}')
			items=$(kubectl $kube_config config get-contexts | grep -v CURRENT | sed 's/^[ \*\t]*//')
			# Iterate over each line in the items
			while IFS= read -r item; do
				# Split the line (assuming space-separated values) into separate variables
				context_name=$(echo $item | awk '{print $1}')
				cluster_name=$(echo $item | awk '{print $2}')

				if echo "$cluster_list" | grep -w -q -- "$cluster_name"; then
					echo "$cluster_name found in the list." >&2
				else
				 	kubectl $kube_config config delete-context $context_name
				fi
			done <<< "$items"
            kubectl $kube_config config get-contexts
			;;


		delfinalizers|df)
			execute_script_with_local_context ${SCRIPTDIR}/../k8s/remove_finalizers.sh
			;;
		load-image|li)
			read -p "Kind cluster name (Enter 'cc' for current cluster): " KIND_CLUSTER_NAME

			if [ "$3" == "-q" ]; then
				cluster_list=$(kind get clusters)
			elif [ -z "$KIND_CLUSTER_NAME" ]; then
				cluster_list=$(kind get clusters)
			else
				cluster_list=$(kubectl config current-context | cut -d'-' -f2-)
			fi

			for cluster in $cluster_list; do
				kind load docker-image $2 --name $cluster
			done
			;;
		*)
			show_help $@
			execute_script_with_local_context ${SCRIPTDIR}/../kubectl/commands.sh $@
			;;
	esac
}

function _main() {
	echo -e "\033[1;33m`date`\033[0m" >&2
	case $1 in
		_git)
			${SCRIPTDIR}/../git/commands.sh ${@:2}
			;;
		_prod_kubectl)
			handle_prod_kube_command ${@:2}
			;;
		_local_kubectl)
			handle_local_k_command ${@:2}
			;;
	esac
}

_main $@

#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/utils.sh"
source "${SCRIPTDIR}/../shared/colors.sh"
source "${SCRIPTDIR}/../bash/dev_config.sh"
source "${SCRIPTDIR}/../shared/runtime_utils.sh"

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
				# Simulating a do-while loop
				while true; do
					# Fetch the current rollout status
					rollout_status=$(kubectl $kube_config -n $1 get app ${_app_name} -ojsonpath='{.status.rolloutStatus}')
					
					# Print the kubectl command for debugging
					echo "kubectl $kube_config -n $1 get app ${_app_name} -ojson" >&2
					
					# Fetch the application details into a temporary JSON file
					kubectl $kube_config -n $1 get app ${_app_name} -ojson > ${DEV_TEMP_DATA_DIR}/_app.json
					
					# Parse the application status with a Python script
					${PYTHON_CMD} ${SCRIPTDIR}/../pyscripts/parse_application_status.py

					# If rollout is completed, break the loop
					if [[ "$rollout_status" == "Completed" ]]; then
						break
					fi
					
					# Sleep for 3 seconds before checking again
					sleep 3
					echo -e "\033[1;33m`date`\033[0m" >&2
				done
				return
			elif [ "$4" = "clusters" ]; then
				local _object
				local _clusters
				_object=$(kubectl $kube_config -n $1 get app ${_app_name} -ojson)
				_clusters=$(echo "$_object" | ${JQ_CMD} -r '.status.clusters[].cluster')
				echo "$_clusters"
				return
			elif [ "$4" = "mw" ]; then
				local _object
				local _uid
				local _clusters		
				
				_object=$(kubectl $kube_config -n $1 get app ${_app_name} -ojson)
				_clusters=$(echo "$_object" | ${JQ_CMD} -r '.status.clusters[].cluster')
				if [ $? -ne 0 ]; then
					echo "Failed to read clusters from app status"
				fi
				_uid=$(echo "$_object" | ${JQ_CMD} -r '.metadata.uid')
				local i=0
				local _output
				if [ -z $_attr_path ]; then
					kubectl $kube_config get mw -A -l "apis.clusterfleet.io/work=$_uid" -owide ${OPT_SORT_BY}
				else
					for cl in $_clusters; do
						_output=$(kubectl $kube_config -n std-cluster-$cl get mw "$1.${_app_name}" -ojson ${OPT_SORT_BY} | ${JQ_CMD} -r ".items[] | $_attr_path")
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
	case $2 in
		test-datapath)
			# kprod xe test-datadir sharedprod-7b4cfd48fb-4snm9
			local dataPath="C:\\pods\\datafolders\\$OPT_NAMESPACE_NAME.$3\\aplusranker-prod\\en-us\\Nets\\Darwin\\BigAnswers\\Dynamic\\ProdV4"
			local dataPath="C:\\pods\\datafolders"
			local nodeName=$(kubectl $kube_config $OPT_NAMESPACE get pod $3 --no-headers -o custom-columns=":.spec.nodeName")
			kubectl $kube_config -n falcon-core get pod --field-selector spec.nodeName=$nodeName
			local dataFetcherPodName=$(kubectl $kube_config -n falcon-core get pod --field-selector spec.nodeName=$nodeName --no-headers -o custom-columns=":metadata.name"|grep datafetcher)
			echo "kubectl $kube_config -n falcon-core exec $dataFetcherPodName -- powershell -Command \"Test-Path '$dataPath'\""
			kubectl $kube_config -n falcon-core exec $dataFetcherPodName -- powershell -Command "Test-Path '$dataPath'"
			;;
		test-datadir)
			# kprod xe test-datadir 801d2297 "C:\datafolderroot\Data\aplusranker-xap-experiments-d7f3a0b8-597031663_container\aplusranker-xap-experiments-d7f3a0b8-597031663\en-us\Nets\Darwin\BigAnswers\Dynamic\ProdV4"
			# kprod xe test-datadir 801d2297 "C:\pods\datafolders\xap-experiments.sharedprod-7b4cfd48fb-4snm9\aplusranker-prod\en-us\Nets\Darwin\BigAnswers\Dynamic\ProdV4"
			local templateHash=$3
			local testPath=$4
			local dataDownloads=$(kubectl $kube_config $OPT_NAMESPACE get dd -l "data.falcon.io/template-hash=$templateHash" --no-headers -o custom-columns=":metadata.name")
			echo "dataDownloads: $dataDownloads"
			# return
			for dd in $dataDownloads; do
				local nodeName="${dd##*-}"
				local dataFetcherPodName=$(kubectl $kube_config -n falcon-core get pod --field-selector spec.nodeName=$nodeName --no-headers -o custom-columns=":metadata.name"|grep datafetcher)
				echo -e "${YELLOW}nodeName: $nodeName, dataFetcherPodName: $dataFetcherPodName, dd: ${dd}${DEFAULTCOLOR}"
				echo "kubectl $kube_config -n falcon-core exec $dataFetcherPodName -- powershell -Command \"Test-Path '$4'\""
				kubectl $kube_config -n falcon-core exec $dataFetcherPodName -- powershell -Command "Test-Path '$4'"
			done
			;;
		chr)
			kubectl $kube_config get chr clusterhealth-report-$3 -oyaml
			;;
		clrkubeconfig)
			local kubeContexts=$(kubectl ${kube_config} config get-contexts -o name | grep -Ev '^(cc|uc)' | tr '\n' ' ')
			# Loop over the contexts and delete them
			for ctx in $kubeContexts; do
				echo "Deleting context: $ctx"
				if [ -n "$3" ]; then
					kubectl $kube_config config delete-context "$ctx"
				fi
			done
			;;
		cl)
			kubectl $kube_config get cl -o json | \
				jq -r '.items[] |
				.metadata.name as $name |
				.spec.properties.subnetResourceId as $id |
				.spec.properties.networkIsland as $networkIsland |
				.spec.properties.subscriptionId as $subId |
				($id | split("/") as $parts |
				[$name, $parts[2], $parts[4], $networkIsland, $subId] | @tsv)' | \
				sort | \
				column -t -s $'\t'
			;;
		clvnet)
			kubectl $kube_config get cl -o json | \
				jq -r '.items[] |
				.metadata.name as $name |
				.spec.properties.subnetResourceId as $id |
				($id | split("/") as $parts |
				[$name, $parts[2]] | @tsv)' | \
				sort | \
				column -t -s $'\t' | cut -d' ' -f3 | sort | uniq
			;;
		clvnetx)
			local kubeContexts=$(kubectl ${kube_config} config get-contexts -o name  | grep ^cc | tr '\n' ' ')
			local vnetSubscriptions=("")
			for ctx in $kubeContexts; do
				echo "kubectl ${kube_config} --context=$ctx get cl -o json | ${JQ_CMD} -r '.items[] | .metadata.name as \$name | .spec.properties.subnetResourceId as \$id | (\$id | split(\"/\") as \$parts | [\$name, \$parts[2]] | @tsv)'" >&2
				local sublist=$(kubectl ${kube_config} --context=$ctx get cl -o json | ${JQ_CMD} -r '.items[] | .metadata.name as $name | .spec.properties.subnetResourceId as $id | ($id | split("/") as $parts | [$name, $parts[2]] | @tsv)' | sort | column -t -s $'\t')
				if [ -n "$sublist" ]; then
					while IFS=$' ' read -r name sub; do
						# echo "Context: $ctx, clname: $name, Subscription: $sub" >&2
						vnetSubscriptions+=("$sub")
					done <<< "$sublist"
				else
					echo "No vnet subscriptions found in context: $ctx"
				fi
			done
			unique_subs=$(printf "%s\n" "${vnetSubscriptions[@]}" | sort -u)
			ps_array='@('
			first=1
			while IFS= read -r sub; do
				if [[ -n "$sub" ]]; then
					if [[ $first -eq 1 ]]; then
						ps_array+="\"$sub\""
						first=0
					else
						ps_array+=", \"$sub\""
					fi
				fi
			done <<< "$unique_subs"
			ps_array+=')'

			# Output the PowerShell array
			echo "$ps_array"
			;;
		clg|clr|clm)
			local version_annotation_suffix="cycle-version"
			if [ "$2" = "clm" ]; then
				version_annotation_suffix="cycle-majorersion"
			fi
			local cycleversion="$3"
			if [ -z "$3" ]; then
				echo "kubectl $kube_config get cl -o=custom-columns=\"NAME:.metadata.name,majorersion:.metadata.labels.microsoft-falcon\.net\/$version_annotation_suffix,CLUSTERDEFINITION:.spec.clusterDefinition\" | cut -d\" \" -f4 | sort | uniq -c" >&2
				local _output=$(kubectl $kube_config get cl -o=custom-columns="NAME:.metadata.name,CYCLEVERSION:.metadata.labels.microsoft-falcon\.net\/$version_annotation_suffix,CLUSTERDEFINITION:.spec.clusterDefinition")
				echo -e "${YELLOW}Current Clusters${DEFAULTCOLOR}" >&2
				echo "$_output" | sort >&2
				echo -e "${YELLOW}Current cycle versions in clusters:${DEFAULTCOLOR}" >&2
				echo "$_output" | cut -d" " -f4 | sort | uniq -c >&2
				resource_list=$(echo "$_output" | cut -d" " -f4 | sort | uniq | nl -v 0)
				select_item_from_resource_list "CYCLEVERSION"
				cycleversion=$(echo "$RESOURCE_LIST_SELECTED_NAME")
			fi

			echo "kubectl $kube_config get cl -l microsoft-falcon.net/$version_annotation_suffix=$cycleversion" >&2
			kubectl $kube_config get cl -l microsoft-falcon.net/$version_annotation_suffix=$cycleversion ${OPT_SORT_BY}
			echo -e "${YELLOW}Cluster Vnet and Subnet IDs:${DEFAULTCOLOR}" >&2
			kubectl $kube_config get cl -l microsoft-falcon.net/$version_annotation_suffix=$cycleversion ${OPT_SORT_BY} -o json | \
				jq -r '.items[] |
				.metadata.name as $name |
				.spec.properties.subscriptionId as $subId |
				.spec.properties.subnetResourceId as $id |
				($id | split("/") as $parts |
				[$parts[4], $parts[-1], $name, $subId] | @tsv)' | \
				sort | \
				column -t -s $'\t'

			;;
		clparse)
			local region=$(echo $3 | cut -d '-' -f2)
			local vnet_subnet_pair=$(${PYTHON_CMD} ~/$CODE_GIT_REPO_NAME/pyscripts/sc_template_parser.py ./fleet-documents/clusters/$region/$3.yaml)
			echo "vnet_subnet_pair: $vnet_subnet_pair"
			;;
		clreplace)
			local region=$(echo $4 | cut -d '-' -f2)
			local vnet_subnet_pair=$(${PYTHON_CMD} ~/$CODE_GIT_REPO_NAME/pyscripts/sc_template_parser.py ./fleet-documents/clusters/$region/$4.yaml)
			${PYTHON_CMD} ~/$CODE_GIT_REPO_NAME/pyscripts/sc_template_parser.py $3 $vnet_subnet_pair
			if [ $? -ne 0 ]; then
				echo "Failed to replace cluster template!"
				return
			fi
			git rm fleet-documents/clusters/$region/$4.yaml
			echo "vnet_subnet_pair used: $vnet_subnet_pair"
			;;
		clnew)
			${PYTHON_CMD} ~/$CODE_GIT_REPO_NAME/pyscripts/sc_template_parser.py ${@:3}
			;;
		cldrain)
			if [ -z "$3" ]; then
				echo "help: k - drain <std-cluster-name>"
				return
			fi

			echo -en "${YELLOW}Enter ${RED} cluster name ${YELLOW} to drain: $DEFAULTCOLOR" >&2
			read _cluster_name
			if [ "$_cluster_name" != "$3" ]; then
				echo "Cluster name mismatch! Expected: $3, got: $_cluster_name"
				return
			fi
			
			echo "kubectl $kube_config patch cl $3 --type merge -p '{\"status\":{\"runtimeStatus\":{\"clusterState\":\"Drain\",\"clusterSubstate\":\"Degraded\",\"clusterStateOverride\":true}}}' --subresource status" >&2
			kubectl $kube_config patch cl $3 --type merge -p '{"status":{"runtimeStatus":{"clusterState":"Drain","clusterSubstate":"Degraded","clusterStateOverride":true}}}' --subresource status
			;;
		clds|cl-drain-status)
			if [ -z "$3" ]; then
				kubectl $kube_config get cl | grep -i drain
				echo "help: k - ds[drain-status] <std-cluster-name>"
				return
			fi

			echo 
			echo -e "${YELLOW}Cluster status: $DEFAULTCOLOR" >&2
			echo "kubectl $kube_config get cl $3" >&2
			kubectl $kube_config get cl $3 ${OPT_SORT_BY}

			echo
			echo -e "${YELLOW}Applications marked for reprocessing: $DEFAULTCOLOR" >&2
			echo "kubectl $kube_config get app -A -o=custom-columns='Namespace:.metadata.namespace,Name:.metadata.name,Annotations:.metadata.annotations.microsoft-falcon\.net/reprocess-application' | grep $3" >&2
			kubectl $kube_config get app -A -o=custom-columns='Namespace:.metadata.namespace,Name:.metadata.name,Annotations:.metadata.annotations.microsoft-falcon\.net/reprocess-application' | grep $3

			echo
			echo -e "${YELLOW}Stale manifestworks: $DEFAULTCOLOR" >&2
			echo "kubectl $kube_config -n std-cluster-$3 get manifestwork -l microsoft-falcon.net/is-stale-manifest=true" >&2
			kubectl $kube_config -n std-cluster-$3 get manifestwork -l microsoft-falcon.net/is-stale-manifest=true ${OPT_SORT_BY}

			echo
			echo -e "${YELLOW}Manifestworks managed by scheduler: $DEFAULTCOLOR" >&2
			echo "kubectl $kube_config -n std-cluster-$3 get manifestwork -l microsoft-falcon.net/managed-by=scheduler | grep -v falcon-core | grep -v clusterfleet" >&2
			local _output=$(kubectl $kube_config -n std-cluster-$3 get manifestwork -l microsoft-falcon.net/managed-by=scheduler ${OPT_SORT_BY} | grep -v falcon-core | grep -v clusterfleet)
			echo "${_output}"

			# echo "${_output}" | head -n 1  |cut -d ' ' -f1 | tr '\n' ' '
			;;
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

			${SCRIPTDIR}/../k8s/resource_commands.sh $INPUT_COMMAND_ARGS
			;;
		describe|desc)
			if [[ -z "$3" ]]; then
				echo "describe: k <ns> [describe|desc] <resource> [*<pattern>] [<nodename>] [-code|-less default => -owide]"
				return
			fi			
			read -ra INPUT_COMMAND_ARGS_LIST <<< "$INPUT_COMMAND_ARGS"
			# echo "${INPUT_COMMAND_ARGS_LIST[@]:0:1} describe ${INPUT_COMMAND_ARGS_LIST[@]:2}"
			${SCRIPTDIR}/../k8s/resource_commands.sh ${INPUT_COMMAND_ARGS_LIST[@]:0:1} describe ${INPUT_COMMAND_ARGS_LIST[@]:2}
			;;
		logs)
			if [[ -z "$3" ]]; then
				echo "logs    : k <ns> logs <pod-name-pattern> [search-string-pattern] [-f]"
				return
			fi
			read -ra INPUT_COMMAND_ARGS_LIST <<< "$INPUT_COMMAND_ARGS"
			# echo "${INPUT_COMMAND_ARGS_LIST[@]:0:1}" get logs "${INPUT_COMMAND_ARGS_LIST[@]:2}"
			${SCRIPTDIR}/../k8s/load_logs.sh ${INPUT_COMMAND_ARGS_LIST[@]:0:1} logs - ${INPUT_COMMAND_ARGS_LIST[@]:2}
			;;
		exec|x)
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
				kpresource $1 exec -it $3 -- ${@:4}
			fi
			;;
		df|datafolder)
			local folder_list=${@:3}
			for folder in $folder_list; do
				${SCRIPTDIR}/../fleet/datafolder_objects.sh $OPT_NAMESPACE $folder
			done
			;;
		fdf|fleetdf|fleetdatafolders)
			case $3 in
			cl)
				folders_names=$(kubectl ${kube_config} -n std-cluster-$4 get fleetdatafolders | grep $5 | cut -d ' ' -f1 | tr '\n' ' ')
				# echo "folders_names: $folders_names"
				# echo "kubectl ${kube_config} -n std-cluster-$4 get fleetdatafolders ${folders_names} -o json"
				kubectl ${kube_config} -n std-cluster-$4 get fleetdatafolders ${folders_names} -o json | \
				{
				echo -e "FolderName\tTemplateHash\tVersion\tPaused\tTotalPods\tUpdatedPods\tAvailablePods\tDeploymentName\tName"
				jq -r '.items |
					sort_by(.status.availablePods) |
					.[]  |
					.spec.template.folderName as $foldername |
					.metadata.annotations["data.falcon.io/template-hash"] as $templatehash |
					.metadata.labels["data.falcon.io/datadeployment-name"] as $deplymentname |
					.spec.template.version as $version |
					.spec.paused as $paused |
					.status.totalPods as $totalpods |
					.status.updatedPods as $updatedpods |
					.status.availablePods as $availablepods |
					.metadata.name as $objName |
					[$foldername, $templatehash, $version, $paused, $totalpods, $updatedpods, $availablepods, $deplymentname, $objName] | @tsv'
				} | sort | column -t -s $'\t'
				;;
			fdn|dn|folder)
				kubectl ${kube_config} get fleetdatafolders -l "data.falcon.io/datadeployment-name=$4" -A -o json | \
				{
				echo -e "ClusterNS\tName\tTemplateHash\tVersion\tPaused\tTotalPods\tUpdatedPods\tAvailablePods\tFolderName"
				jq -r '.items |
					sort_by(.status.availablePods) |
					.[]  |
					.spec.template.folderName as $folderName |
					.metadata.name as $objName |
					.metadata.namespace as $objNamespace |
					.metadata.annotations["data.falcon.io/template-hash"] as $templatehash |
					.spec.template.version as $version |
					.spec.paused as $paused |
					.status.totalPods as $totalpods |
					.status.updatedPods as $updatedpods |
					.status.availablePods as $availablepods |
					[$objNamespace, $objName, $templatehash, $version, $paused, $totalpods, $updatedpods, $availablepods, $folderName] | @tsv'
				} | column -t -s $'\t'
				;;
			esac	

			# kubectl ${kube_config} -n std-cluster-$3 get fleetdatafolders $folders_names -o jsonpath='{.metadata.annotations.data\.falcon\.io\/template-hash} {.spec.template.version} {.spec.template.folderName}' | sort |  column -t -s $'\t'
			# for folder in $folders_names; do
			# 	echo "kubectl ${kube_config} -n std-cluster-$3 get fleetdatafolders $folder"
			# 	kubectl ${kube_config} -n std-cluster-$3 get fleetdatafolders $folder -o jsonpath='{.metadata.annotations.data\.falcon\.io\/template-hash} {.spec.template.version} {.spec.template.folderName}' | sort |  column -t -s $'\t'
			# done
			;;

		delete)
			if [ -z "$3" ]; then
				echo "delete  : k <ns> delete <resource> [< node-name|- >] [pattern]"
				return
			fi
			confirm_on_kubectl_crud_operation $@
			${SCRIPTDIR}/../k8s/delete_resource.sh $INPUT_COMMAND_ARGS
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
					kubectl $kube_config $OPT_NAMESPACE rollout restart deployment scheduler-deployment
					;;
				*)
					kubectl $kube_config $OPT_NAMESPACE rollout restart $3 $4
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
				    # make docker-scheduler ; klocal - apply scheduler
					if [ -n "$kube_config" ]; then
						echo "Cannot use production context to apply dev components!"
						exit 0
					fi
					kind load docker-image scheduler --name $KIND_CONTROL_CLUSTER_NAME
					kubectl $kube_config $OPT_NAMESPACE apply -f hack/deployments/kind-scheduler.yaml
					kubectl $kube_config $OPT_NAMESPACE rollout restart deployment scheduler-deployment
					;;
				syncer)
					if [ -n "$kube_config" ]; then
						echo "Cannot use production context to apply dev components!"
						exit 0
					fi

					kubectl config use-context kind-$KIND_CONTROL_CLUSTER_NAME
					local cluster_list
					cluster_list=$(kubectl get cl -o=custom-columns='NAME:.metadata.name' | grep -v NAME | tr '\n' ' ')

					for cluster in $cluster_list; do
						kubectl config use-context kind-$cluster
						kubectl $kube_config $OPT_NAMESPACE apply -f hack/deployments/kind-standardcluster.yaml
					done
					kubectl config use-context kind-$KIND_CONTROL_CLUSTER_NAME
					;;
				demo-app-pr|demo-app-ds|demo-app-ss|demo-app)
					if [ -n "$kube_config" ]; then
						echo "Cannot use production context to apply dev components!"
						exit 0
					fi
					echo "kubectl $kube_config $OPT_NAMESPACE apply -f ~/$CODE_GIT_REPO_NAME/demo-apps/yaml/${3}.yaml" >&2
					kubectl $kube_config $OPT_NAMESPACE apply -f ~/$CODE_GIT_REPO_NAME/demo-apps/yaml/${3}.yaml
					;;
				*)
					kpresource $@
					;;
			esac
			;;
		clsub|subscription)
			echo "kubectl $kube_config $OPT_NAMESPACE get cl -o json | ${JQ_CMD} -r '.items[] | [\"-n \" + .metadata.name, \"-s \" + .spec.properties.subscriptionId] | @tsv'" >&2
			# local _output=$(kubectl $kube_config $OPT_NAMESPACE get cl -o custom-columns='NAME:.metadata.name,SUBSCRIPTION_ID:.spec.properties.subscriptionId')
			local _output=$(kubectl $kube_config $OPT_NAMESPACE get cl -o json | ${JQ_CMD} -r '.items[] | ["dlkube.ps1 -n " + .metadata.name, "-s " + .spec.properties.subscriptionId, " -p 1 "] | @tsv')

			if [ -n "$3" ]; then
				echo "$_output" | grep $3
			else
				echo "$_output"
			fi
			;;
		b64)
			base64 --decode <<< "$3" 2>/dev/null | ${JQ_CMD} .
			;;
		jwt)
			TOKEN="$3"
			echo "$TOKEN" | awk -F '.' '{print $1 "\n" $2}' | \
			while read part; do
				echo "$part" | base64 --decode 2>/dev/null | ${JQ_CMD} .
			done
			;;
        fdes)
            echo "kubectl $kube_config $OPT_NAMESPACE get endpointslices -o custom-columns='NAME:.metadata.name, SOURCECLUSTER:.metadata.labels.multicluster\.kubernetes\.io/source-cluster'" >&2
            local _output=$(kubectl $kube_config $OPT_NAMESPACE get endpointslices -o custom-columns='NAME:.metadata.name, SOURCECLUSTER:.metadata.labels.multicluster\.kubernetes\.io/source-cluster')
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
	case $1 in
		all)
			if [[ -z "$2" ]]; then
				echo "help: k all <resource> [pattern]"
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
			kpverb $@
			;;
	esac
}

set_common_options "$@"
set -- "${POST_PARSE_ARGS[@]}"
_kpnamespace $@

#!/bin/bash

namespace="$1"
node="${OPT_NODE_NAME_SELECTOR#--field-selector spec.nodeName=}"
if [[ $# -ne 1 || -z "$namespace" || "$namespace" == "-" || -z "$OPT_POD_PATTERN" || -z "$node" || -z "$OPT_SCRIPT_FILE" ]]; then
	echo "Usage: k <ns> run_exec --pod <pod_pattern> --sfile <script_file> -nn <node_name>" >&2
	exit 1
fi
if [[ ! -f "$OPT_SCRIPT_FILE" || ! -r "$OPT_SCRIPT_FILE" ]]; then
	echo "PowerShell script is not a readable file: $OPT_SCRIPT_FILE" >&2
	exit 1
fi
script_bytes=$(wc -c < "$OPT_SCRIPT_FILE") || exit $?
if [[ "$script_bytes" -eq 0 ]]; then
	echo "PowerShell script is empty: $OPT_SCRIPT_FILE" >&2
	exit 1
fi

kubectl_args=(-n "$namespace")
if [[ -n "$kube_context" ]]; then
	kubectl_args+=(--context "${kube_context#--context }")
fi
if [[ -n "$kube_config" ]]; then
	config_path="${kube_config#--kubeconfig }"
	config_path="${config_path//\\//}"
	if [[ "$config_path" != /* && "$config_path" != [a-zA-Z]:/* ]]; then
		config_path="$PWD/$config_path"
	fi
	kubectl_args+=(--kubeconfig "$config_path")
fi
pod_names=$(kubectl "${kubectl_args[@]}" get pods \
	--field-selector "spec.nodeName=$node,status.phase=Running" \
	-o 'jsonpath={range .items[*]}{.metadata.name}{"\n"}{end}') || exit $?
matches=()
while IFS= read -r pod; do
	if [[ -n "$pod" && ( "$pod" == $OPT_POD_PATTERN || "$pod" == *"$OPT_POD_PATTERN"* ) ]]; then
		matches+=("$pod")
	fi
done <<< "$pod_names"
if [[ ${#matches[@]} -ne 1 ]]; then
	echo "Expected one running pod matching '$OPT_POD_PATTERN' on node $node in $namespace; found ${#matches[@]}: ${matches[*]}" >&2
	exit 1
fi
pod="${matches[0]}"

source_path="$OPT_SCRIPT_FILE"
if [[ "$source_path" == [a-zA-Z]:* ]]; then
	source_path="${source_path//\\//}"
fi
source_name="${source_path##*/}"
source_dir="${source_path%/*}"
if [[ "$source_dir" == "$source_path" ]]; then
	source_dir="."
elif [[ -z "$source_dir" ]]; then
	source_dir="/"
fi
remote_file="run_exec_${RANDOM}_${RANDOM}.ps1"

echo "Copying $OPT_SCRIPT_FILE ($script_bytes bytes) to $namespace/$pod on $node" >&2
# A relative source avoids kubectl cp interpreting a Windows drive as a pod name.
(cd -- "$source_dir" && kubectl "${kubectl_args[@]}" cp "./$source_name" "$pod:./$remote_file") || exit $?

echo "Copied to the container's working directory: $remote_file" >&2
echo "Run this command inside the PowerShell session:" >&2
echo "  & '.\\$remote_file'" >&2
echo "The script has NOT been executed. Opening an interactive PowerShell session..." >&2
kubectl "${kubectl_args[@]}" exec -it "$pod" -- powershell -NoProfile -NoLogo

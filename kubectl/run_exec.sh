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

kubectl_args=(-n "$namespace")
if [[ -n "$kube_context" ]]; then
	kubectl_args+=(--context "${kube_context#--context }")
fi
if [[ -n "$kube_config" ]]; then
	kubectl_args+=(--kubeconfig "${kube_config#--kubeconfig }")
fi
pod_names=$(kubectl "${kubectl_args[@]}" get pods \
	--field-selector "spec.nodeName=$node,status.phase=Running" \
	-o 'jsonpath={range .items[*]}{.metadata.name}{"\n"}{end}') || exit $?
matches=()
while IFS= read -r pod; do
	if [[ -n "$pod" && "$pod" == $OPT_POD_PATTERN ]]; then
		matches+=("$pod")
	fi
done <<< "$pod_names"
if [[ ${#matches[@]} -ne 1 ]]; then
	echo "Expected one running pod matching '$OPT_POD_PATTERN' on node $node in $namespace; found ${#matches[@]}: ${matches[*]}" >&2
	exit 1
fi
pod="${matches[0]}"

echo "Copying and running $OPT_SCRIPT_FILE in $namespace/$pod on $node" >&2
kubectl "${kubectl_args[@]}" exec -i "$pod" -- powershell -NoProfile -NonInteractive -Command '
	$ErrorActionPreference = "Stop"
	$scriptPath = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString() + ".ps1")
	try {
		$file = [IO.File]::Create($scriptPath)
		try {
			[Console]::OpenStandardInput().CopyTo($file)
		} finally {
			$file.Dispose()
		}
		$global:LASTEXITCODE = 0
		& $scriptPath
		$succeeded = $?
		if ($LASTEXITCODE -ne 0) {
			exit $LASTEXITCODE
		}
		if (-not $succeeded) {
			exit 1
		}
	} finally {
		Remove-Item -LiteralPath $scriptPath -Force -ErrorAction Stop
	}
' < "$OPT_SCRIPT_FILE"

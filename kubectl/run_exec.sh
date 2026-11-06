#!/bin/bash

set -o pipefail

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
	kubectl_args+=(--kubeconfig "${kube_config#--kubeconfig }")
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

echo "Copying and running $OPT_SCRIPT_FILE ($script_bytes bytes) in $namespace/$pod on $node" >&2
powershell_script='
	$ErrorActionPreference = "Stop"
	Write-Host "[run_exec] PowerShell started; receiving $expectedBytes bytes."
	$scriptPath = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString() + ".ps1")
	try {
		$file = [IO.File]::Create($scriptPath)
		try {
			[Console]::OpenStandardInput().CopyTo($file)
		} finally {
			$file.Dispose()
		}
		$actualBytes = (Get-Item -LiteralPath $scriptPath).Length
		if ($actualBytes -ne $expectedBytes) {
			throw "Script transfer failed: expected $expectedBytes bytes, received $actualBytes."
		}
		Write-Host "[run_exec] Copied $actualBytes bytes to $scriptPath. Starting script."
		$global:LASTEXITCODE = 0
		& $scriptPath
		$succeeded = $?
		if ($LASTEXITCODE -ne 0) {
			exit $LASTEXITCODE
		}
		if (-not $succeeded) {
			exit 1
		}
		Write-Host "[run_exec] Script completed (exit 0). Verify the intended changes separately."
	} finally {
		Remove-Item -LiteralPath $scriptPath -Force -ErrorAction Stop
	}
'
encoded_command=$(printf '%s' "\$expectedBytes = $script_bytes; $powershell_script" |
	iconv -f UTF-8 -t UTF-16LE | base64 | tr -d '\r\n') || exit $?
kubectl "${kubectl_args[@]}" exec -i "$pod" -- powershell -NoProfile -NonInteractive \
	-InputFormat Text -OutputFormat Text -EncodedCommand "$encoded_command" < "$OPT_SCRIPT_FILE"
status=$?
if [[ "$status" -ne 0 ]]; then
	echo "[run_exec] Remote execution failed (exit $status)." >&2
fi
exit "$status"

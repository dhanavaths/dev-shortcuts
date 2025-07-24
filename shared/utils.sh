#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../shared/colors.sh"

function get_namespace(){
	local _namespace=$1
	case $1 in
		ks)
			_namespace="kube-system"
			;;
		fc)
			_namespace="falcon-core"
			;;
		cf)
			_namespace="clusterfleet"
			;;
		xe)
			_namespace=xap-experiments
			;;
		ac)
			_namespace=ads-core
			;;
		mt)
			_namespace=magnetar
			;;
		d)		
			_namespace=default
			;;
	esac
	echo "$_namespace"
}

function get_namespace_extended() {
	local _namespace=$(get_namespace "$1")
	if [ "$_namespace" = "-" ]; then
		echo ""
	else
		echo "-n $_namespace"
	fi
}
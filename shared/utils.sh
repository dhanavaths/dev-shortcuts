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


function is_extended_output() {
	if [[ "$@" ==  *" -less"* ]] || [[ "$@" ==  *" -oless"* ]]; then
		echo "1"
	elif [[ "$@" == *" -code"* ]] || [[ "$@" == *" -ocode"* ]]; then
		echo "2"
	else
		echo "0"
	fi
}

function replace_exteded_output() {
	_final_string=$(echo $@ | sed  's/ -less//g' | sed  's/ -oless//g' | sed 's/ -ocode//g' | sed 's/ -code//g')
	echo $_final_string
}

#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/PS.sh"


function g()
{
	${SCRIPTDIR}/eval.sh _git $@
}

function kube_prod()
{
	${SCRIPTDIR}/eval.sh _prod_kubectl $@
}

function kube_local()
{
	${SCRIPTDIR}/eval.sh _local_kubectl $@
}

function bashrc()
{
    . ~/.bashrc
}


function publishshortcuts() {
	local repoName="dev-shortcuts"
	local _shortcutsAbsPath=$(readlink -f "${SCRIPTDIR}/../../${repoName}")
	_=$(cd ${_shortcutsAbsPath}/../; zip -r /tmp/${repoName}.zip ${repoName})
	rsync -av --exclude='.git' "${_shortcutsAbsPath}" /mnt/q/
	cp /tmp/${repoName}.zip /mnt/q/${repoName}/${repoName}.zip
}

function execute_dev_command() {
	${SCRIPTDIR}/../shared/dev_commands.sh _execute_dev_commands ${@:1}
}

function ws() {
	local _target_path=$(${SCRIPTDIR}/wdir.sh $@)
	if [ -n "$_target_path" ]; then
		cd $_target_path
	fi	
}


export PROD_KUBECTL_SHORTCUT="kube_prod"
export LOCAL_KUBECTL_SHORTCUT="kube_local"

alias k="${PROD_KUBECTL_SHORTCUT}"
alias kube="${PROD_KUBECTL_SHORTCUT}"
alias l="${LOCAL_KUBECTL_SHORTCUT}"
alias kl="${LOCAL_KUBECTL_SHORTCUT}"

# aliases
alias kuc="${PROD_KUBECTL_SHORTCUT} kuc "
alias kgc="${PROD_KUBECTL_SHORTCUT} kgc "
alias dlkube="${PROD_KUBECTL_SHORTCUT} dlkube "

alias _kuc="${LOCAL_KUBECTL_SHORTCUT} kuc "
alias _kgc="${LOCAL_KUBECTL_SHORTCUT} kgc "
alias _krc="${LOCAL_KUBECTL_SHORTCUT} krc "
alias _krd="${LOCAL_KUBECTL_SHORTCUT} krc "

alias x="execute_dev_command "
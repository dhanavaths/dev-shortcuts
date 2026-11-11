#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/config.sh"
 
function k8s_dev_context {
    context=$(kubectl $kube_context config current-context 2>/dev/null)
    local _prod_context=$(kubectl $kube_context --kubeconfig $prod_kube_config_file_path config current-context 2>/dev/null)
    # if [ "$_prod_context" = "$context" ]; then
    #     echo -e "\e[32m[_]\e[0m"
    if [ -n "$context" ]; then
        echo -e "\e[32m[$context]\e[0m"
    else
        echo -e "\e[32m[_]\e[0m"
    fi
}
 
 
function k8s_prod_context {
    context=$(kubectl $kube_context --kubeconfig $prod_kube_config_file_path config current-context 2>/dev/null)
    if [ -n "$context" ]; then
        echo -e "\e[32m[$context]\e[0m"
    fi
}
 
function git_context {
    local context
    if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        context=$(git rev-parse --abbrev-ref HEAD | awk -F/ '{print ((NF>1)?$(NF):$0)}')
        echo -e "\e[90m[git: $context]\e[0m"
    else
		echo -e "\e[90m[git: _]\e[0m"
    fi
}
 
function runtime_env {
    # 1. Check if the OS is Linux
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Check if it is specifically WSL
        if grep -qi microsoft /proc/version; then
            echo -e "\e[90m[WSL]\e[0m"
        else
            echo -e "\e[90m[linux]\e[0m"
        fi

    # Git Bash builds use either the MSYS or Cygwin runtime.
    elif [[ "$OSTYPE" == msys* || "$OSTYPE" == cygwin* ]]; then
        if test -e 'C:\ProgramData\Microsoft\Windows\Start Menu\Programs\SAW Control Panel'; then
            echo -e "\e[90m[SAW]\e[0m"
        else
            echo -e "\e[90m[HOST]\e[0m"
        fi
    else
        echo -e "\e[90m[UNKNOWN]\e[0m"
    fi
}

function _dev_shortcuts_update_prompt {
    _dev_shortcuts_k8s_context=$(k8s_dev_context)
    _dev_shortcuts_runtime_env=$(runtime_env)
    _dev_shortcuts_git_context=$(git_context)
    export PS1='${_dev_shortcuts_k8s_context} ${_dev_shortcuts_runtime_env} \w ${_dev_shortcuts_git_context}\n\$ '
}

# Appending also preserves the remaining elements of an existing hook array.
if [[ ${PROMPT_COMMAND-} != *"_dev_shortcuts_update_prompt"* ]]; then
    PROMPT_COMMAND+=$'\n_dev_shortcuts_update_prompt'
fi
 
if [ -d "${HOME}/workspace" ]; then
  cd ${HOME}/workspace
fi
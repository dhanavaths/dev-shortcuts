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

    # 2. Check if the environment is MINGW64
    elif [ "$MSYSTEM" = "MINGW64" ]; then
        # Check for any drive letters other than C:
        # We look for mount points like /d, /e, etc. 
        local extra_drives=$(mount | grep -E '^.:' | grep -v '^C:' | wc -l)

        if [ "$extra_drives" -gt 0 ]; then
            echo -e "\e[90m[HOST]\e[0m"
        else
            echo -e "\e[90m[SAW]\e[0m"
        fi
    else
        echo -e "\e[90m[UNKNOWN]\e[0m"
    fi
}

if [ "$MSYSTEM" = "MINGW64" ]; then
    PROMPT_COMMAND='export PS1="$(k8s_dev_context) $(runtime_env) \w $(git_context)\n\$ "'
else
    export PS1="\$(k8s_dev_context) \$(runtime_env) \w \$(git_context)\n\$ "
fi
 
if [ -d "${HOME}/workspace" ]; then
  cd ${HOME}/workspace
fi
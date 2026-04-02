#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/config.sh"
 
function k8s_dev_context {
    context=$(kubectl $kube_context config current-context 2>/dev/null)
    local _prod_context=$(kubectl $kube_context --kubeconfig $prod_kube_config_file_path config current-context 2>/dev/null)
    if [ "$_prod_context" = "$context" ]; then
        echo -e "\e[90m[_]\e[0m"
    elif [ -n "$context" ]; then
        echo -e "\e[90m[$context]\e[0m"
    else
        echo -e "\e[90m[_]\e[0m"
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
    if [ "$MSYSTEM" = "MINGW64" ]; then
        local _prod_context=$(kubectl $kube_context --kubeconfig $prod_kube_config_file_path config current-context 2>/dev/null)
        local _dev_context=$(kubectl $kube_context config current-context 2>/dev/null)
        if [ "$_prod_context" = "$_dev_context" ]; then
            echo -e "\e[90m[SAW]\e[0m"
        else
            echo -e "\e[90m[host]\e[0m"
        fi
    else
        echo -e "\e[90m[wsl]\e[0m"
    fi
}
 
if [ "$MSYSTEM" = "MINGW64" ]; then
    PROMPT_COMMAND='export PS1="$(k8s_prod_context) $(runtime_env) \w $(git_context) $(k8s_dev_context)\n\$ "'
else
    export PS1="\$(k8s_prod_context) \$(runtime_env) \w \$(git_context) \$(k8s_dev_context)\n\$ "
fi
 
if [ -d "${HOME}/workspace" ]; then
  cd ${HOME}/workspace
fi
#!/bin/bash
kube_config=""

EXPLORER_COMPATIBLE_WORKSPACE_DIR=""
prod_kube_config_file_path=""
WSL_WORKSPACE_DIR=""
WINDOWS_WORKSPACE_DIR=""
CODE_GIT_REPO_NAME="dev-shortcuts"

if [ "$MSYSTEM" = "MINGW64" ]; then
    if [ -f "/q/kube-config" ]; then
        prod_kube_config_file_path="/q/kube-config"
        WINDOWS_WORKSPACE_DIR="/q/workspace"
        EXPLORER_COMPATIBLE_WORKSPACE_DIR="Q:\\workspace"
    elif [ -f "/e/kube-config" ]; then
        prod_kube_config_file_path="/e/kube-config"
        WINDOWS_WORKSPACE_DIR="/e/workspace"
        EXPLORER_COMPATIBLE_WORKSPACE_DIR="E:\\workspace"
    elif [ -f "$HOME/.kube/config" ]; then
        prod_kube_config_file_path="$HOME/.kube/config"
        WINDOWS_WORKSPACE_DIR="/c/workspace"
        EXPLORER_COMPATIBLE_WORKSPACE_DIR="C:\\workspace"
    fi
else
    if [ -f "/mnt/q/kube-config" ]; then
        prod_kube_config_file_path="/mnt/q/kube-config"
    elif [ -f "/mnt/e/kube-config" ]; then
        prod_kube_config_file_path="/mnt/e/kube-config"
    fi
    WSL_WORKSPACE_DIR="${HOME}/workspace"

fi

if [ "$MSYSTEM" = "MINGW64" ]; then
    export WORKSPACE_DIR=$WINDOWS_WORKSPACE_DIR
else
    export WORKSPACE_DIR=$WSL_WORKSPACE_DIR
fi

GIT_BRANCH_USERNAME_PREFIX="sdhanavath"
#while true; do   nslookup kubernetesservicediscovery.falcon-core.cluster-local.microsoft-falcon.net;   sleep 1; done
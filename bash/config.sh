#!/bin/bash
CONFIG_SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${CONFIG_SCRIPTDIR}/dev-config/customconfig.sh" 2>/dev/null || true

CODE_GIT_REPO_NAME="dev-shortcuts"
JQ_CMD="jq"
PYTHON_CMD="python3"
DEV_TEMP_DATA_DIR="$HOME/dev-temp-data"

if [ "$MSYSTEM" = "MINGW64" ]; then
    # git-bash settings
    if [ -n "$prod_kube_config_file_path" ]; then
        echo -n ""
    elif [ -f "$HOME/.kube/config" ]; then
        prod_kube_config_file_path="$HOME/.kube/config"
        WINDOWS_WORKSPACE_DIR="/c/workspace"
        EXPLORER_COMPATIBLE_WORKSPACE_DIR="C:\\workspace"
    fi
    JQ_CMD="jq-win64.exe"
    export PATH="$PATH:/c/Program Files/SPython/tools"
    PYTHON_CMD="python"
else
    # WSL settings
    if [ -n "$prod_kube_config_file_path" ]; then
        echo -n ""
    else
        prod_kube_config_file_path="$HOME/.kube/config"
    fi
fi

# export variables
if [ "$MSYSTEM" = "MINGW64" ]; then
    export WORKSPACE_DIR=$WINDOWS_WORKSPACE_DIR
else
    export WORKSPACE_DIR=$WSL_WORKSPACE_DIR
fi

export DEV_TEMP_DATA_DIR
export JQ_CMD
export PYTHON_CMD
export prod_kube_config_file_path
export DEVOPS_NEW_BRANCH_USERNAME_PREFIX
export CODE_GIT_REPO_NAME
export EXPLORER_COMPATIBLE_WORKSPACE_DIR
export KUBE_EDITOR="code --wait"
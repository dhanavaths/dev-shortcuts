#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/PS.sh"

function select_item() {
    local prompt=$1
    local input_prompt=$2
    shift 2
	local options=("$@")
	echo >&2
    echo "$prompt" >&2
	local all_items=$(echo "$@" | tr ' ' '\n')
	local items=$(echo "$all_items" | nl -v 0)
    echo "$items" 	>&2
	echo -n "$input_prompt: " >&2
	read selection_choice
	local choice=$(echo "$items" | awk -v num="$selection_choice" '$1 == num {print $2}')
	echo $choice
}

function change_wdir() {
	local _dir_list
	local _repo_directory
	local _service_name
	local _target_path
	local _exp_compatible_target_path
	_dir_list=$(cd ${WORKSPACE_DIR};ls -d */ | sed 's/\/$//' | sed 's/\/$//' | sort -f | tr '\n' ' ')
	_repo_directory=$(select_item "Repository Directories" "Enter a number" "${_dir_list[@]}")

	if [ -z "$_repo_directory" ]; then
		return
	fi

	_target_path="${WORKSPACE_DIR}/$_repo_directory"
	_exp_compatible_target_path="${EXPLORER_COMPATIBLE_WORKSPACE_DIR}\\$_repo_directory"
	local _services_dir=""
	if [ -d "${WORKSPACE_DIR}/$_repo_directory/services" ];then
		_services_dir="services"
	elif [ -d "${WORKSPACE_DIR}/$_repo_directory/tools" ];then
		_services_dir="tools"
	fi

	if [ -n "$_services_dir" ];then
		_dir_list=$(cd ${WORKSPACE_DIR}/$_repo_directory/$_services_dir;ls -d */ | sed 's/\/$//' | sed 's/\/$//' | sort -f | tr '\n' ' ')
		_service_name=$(select_item "Services" "Enter a number" "${_dir_list[@]}")	
	fi

	if [ -n "$_service_name" ]; then
		_exp_compatible_target_path="${EXPLORER_COMPATIBLE_WORKSPACE_DIR}\\${_repo_directory}\\${_services_dir}\\${_service_name}"
		_target_path="${WORKSPACE_DIR}/${_repo_directory}/${_services_dir}/${_service_name}"
	fi

	case $1 in
		code|vs|c)
			cd ${_target_path}
			code ${_target_path}
			;;
		ex|e|explorer)
			echo $_exp_compatible_target_path >&2
			if [ "$MSYSTEM" = "MINGW64" ]; then
				explorer ${_exp_compatible_target_path}
				return
			else
				cd ${_target_path}
			fi
			;;
		*)
			cd ${_target_path}
			;;
	esac
	echo "${_target_path}"
}

change_wdir $@

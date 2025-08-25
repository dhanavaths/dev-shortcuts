#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR}/../bash/config.sh"

function _git_commands()
{
	local option=$1
	case $option in
		s)
			git status
			;;
		b) 
			${SCRIPTDIR}/switch_branch.sh
			;;
		mb) 
			${SCRIPTDIR}/merge_branch.sh
			;;
		h)
			current_branch=$(git rev-parse --abbrev-ref HEAD)
			git push origin $current_branch
			;;

    	l)
			current_branch=$(git rev-parse --abbrev-ref HEAD)
			git pull origin $current_branch
			;;
		n)
			if [ -z "$2" ]; then
				echo "Branch name is required" >&2
				return
			fi
			git checkout -b ${DEVOPS_NEW_BRANCH_USERNAME_PREFIX}/$2
			;;
		d)
			git diff
			;;
		m)
			if git show-ref --quiet refs/heads/main; then
    				git checkout main
			else
				git checkout master
			fi
			;;
		mm)
			if git show-ref --quiet refs/heads/main; then
				git merge main
			else
				git merge master
			fi
			;;
		D)
			${SCRIPTDIR}/delete_branch.sh
			;;
		diff)
			current_branch=$(git rev-parse --abbrev-ref HEAD)
			if git show-ref --quiet refs/heads/main; then
				git diff origin/main $current_branch
			else
				git diff origin/master $current_branch
			fi
			;;
        patch)
            git apply --ignore-space-change --ignore-whitespace ${@:2}
            ;;
		f)
			if [ -z "$2" ]; then
				echo "Branch name is required" >&2
				return
			fi
			git fetch origin $2
			git checkout $2
		;;
	esac
}

_git_commands $@

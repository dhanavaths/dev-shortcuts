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
		cp)
			git cherry-pick ${@:2}
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
		d)
			git diff
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
		D)
			${SCRIPTDIR}/delete_branch.sh
			;;
		f)
			if [ -z "$2" ]; then
				echo "Branch name is required" >&2
				return
			fi
			git fetch origin $2
			git checkout $2
			;;
		rb)

			local rebase_branch=""
			if git show-ref --quiet refs/heads/main; then
				rebase_branch="origin/main"
			else
				rebase_branch="origin/master"
			fi
			if [ -n "$2" ]; then
				rebase_branch=$2
			fi
			git rebase $rebase_branch
			;;
		reset)
			git reset HEAD
			git checkout -- .
			;;
		shortcuts)
			git config --global alias.sorted-branches '!git for-each-ref refs/heads/ --format="%(if:equals=main)%(refname:short)%(then)000%(else)%(if:equals=master)%(refname:short)%(then)001%(else)%(if:equals=release)%(refname:short)%(then)002%(else)999%(end)%(end)%(end) %(committerdate:raw) %(refname:short)" | sort -k1,1 -k2,2rn | awk -v current="$(git rev-parse --abbrev-ref HEAD)" "{ name=\$NF; if (name == current) print \"* \" name; else print \"  \" name; }"'
	esac
}

_git_commands $@

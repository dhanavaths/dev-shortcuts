#!/bin/bash

# List all branches and assign numbers
branches=$(git sorted-branches | nl -v 0)
branches_display=$(echo "$branches" | sed -E "s/^([[:space:]]*[0-9]+[[:space:]]+)\*[[:space:]]+(.+)$/\1* \x1b[0;32m\2\x1b[0m/")
echo "Select a branch to DELETE:" >&2
echo -e "$branches_display" >&2

# Read user input
read -p "Enter branch number: " branch_number

# Get branch name from selected number
branch_name=$(echo "$branches" | tr '*' ' ' | awk -v num="$branch_number" '$1 == num {print $2}')

# Check if branch exists and switch to it
if [ -n "$branch_name" ]; then
	read -p "Are you sure? " -n 1 -r
	echo # (optional) move to a new line
	if [[ $REPLY =~ ^[Y]$ ]]; then
		if git show-ref --quiet refs/heads/main; then
			git checkout -q main
		else
			git checkout -q master
		fi
		git branch -D "$branch_name"
	else
	echo
	echo "No action taken." >&2
	fi
else
	echo "Invalid branch number." >&2
fi

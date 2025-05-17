#!/bin/bash

# List all branches and assign numbers
branches=$(git branch | nl -v 0)
echo "Select a branch to DELETE:" >&2
echo "$branches" >&2

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

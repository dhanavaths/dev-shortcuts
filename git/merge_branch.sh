#!/bin/bash

# List all branches and assign numbers
branches=$(git sorted-branches | nl -v 0)
branches_display=$(echo "$branches" | sed -E "s/^([[:space:]]*[0-9]+[[:space:]]+)\*[[:space:]]+(.+)$/\1* \x1b[0;32m\2\x1b[0m/")
echo "Select the source branch: " >&2
echo -e "$branches_display" >&2

# Read user input
read -p "Enter a number: " branch_number

# Get branch name from selected number
branch_name=$(echo "$branches" | tr '*' ' ' | awk -v num="$branch_number" '$1 == num {print $2}')

# Check if branch exists and switch to it
if [ -n "$branch_name" ]; then
  git merge "$branch_name"
else
  echo "Invalid input." >&2
fi


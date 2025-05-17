#!/bin/bash

# List all branches and assign numbers
branches=$(git branch | nl -v 0)
echo "Select the source branch: " >&2
echo "$branches" >&2

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


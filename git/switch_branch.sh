#!/bin/bash

function _main() {
  # List all branches and assign numbers
  # local _is_working_tree=$(git -q rev-parse --is-inside-work-tree && exit 0 || exit 1)
  # if [ $? -ne 0 ]; then
  #   echo "Not inside a git working tree."
  #   exit 1
  # fi

  branches=$(git branch | nl -v 0)
  if [ -z "$branches" ]; then
    echo "No branches found."
    exit 1
  fi

  echo "Select a branch to switch:"
  echo "$branches"

  # Read user input
  read -p "Enter a number: " branch_number

  # Get branch name from selected number
  branch_name=$(echo "$branches" | tr '*' ' ' | awk -v num="$branch_number" '$1 == num {print $2}')

  # Check if branch exists and switch to it
  if [ -n "$branch_name" ]; then
    git checkout "$branch_name"
  else
    echo "Invalid input."
  fi
}

_main
#!/bin/bash
function push_all_branches()
{
	for branch in $@; do
		git checkout $branch
		git push origin $branch
	done
}

function merge_all_branches() 
{

	#_list="le-clusterhealthcontroller le-ingressprovisioner le-namespaceregistrar le-sc-provisioners le-scheduler-cc le-tenantnamespacecontroller"
	_list="sdhanavath/le-clusterhealth-cc sdhanavath/le-clusterhealth-sc sdhanavath/le-ingressprovisioner sdhanavath/le-namespaceregistrar sdhanavath/le-sc-provisioners sdhanavath/le-scheduler-cc sdhanavath/le-tenantnamespacecontroller"
	for branch_name in $_list; do
		echo $branch_name >&2
		#git checkout $branch_name || return 1
		#git pull origin $branch_name || return 1
		#git merge main
		git push origin $branch_name || return 1
		#git checkout sdhanavath/core-services-leader-election || return 1
		#git merge $branch_name || return 1
		read -p "Continue: " user_input
		if [ "$user_input" != "y" ]; then
			return 0
		fi
	done
}
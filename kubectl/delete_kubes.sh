#!/bin/bash

if [ -z "$kube_config" ]; then
    echo "kube_config is not set!"
    exit 1
fi

declare -A sc_cluster_list
cc_kube_contexts=`kubectl $kube_config config get-contexts -o name | grep '^cc'`

for _cc_context_name in $cc_kube_contexts; do
    echo "Trying to fetch standard clusters using $_cc_context_name" 
    sc_name_list=`kubectl $kube_config --context $_cc_context_name get cl -o=jsonpath="{.items[*].metadata.name}"`
    if [ $? -ne 0 ] || [ -z "$sc_name_list" ]; then
        kubectl $kube_config config delete-context $_cc_context_name
        echo "$_cc_context_name cc cluster does not exist."
        continue
    fi
    for sc_name in $sc_name_list; do
        sc_cluster_list["$sc_name"]="1"
    done
done

echo $sc_cluster_list

sc_kube_contexts=`kubectl $kube_config config get-contexts -o name | grep -v '^cc-' | grep -v '^uc-'`

for sc_context_name in $sc_kube_contexts; do
    sc_cname=`echo $sc_context_name | cut -d'-' -f1-3` 
    echo "sc: $sc_cname"
    if [[ -v sc_cluster_list["$sc_cname"] ]]; then
        echo "$sc_cname still exists."
    else
        echo "$sc_cname does not exist any more. Deleting it from $kube_config."
        kubectl $kube_config config delete-context $sc_cname
        kubectl $kube_config config delete-context $sc_cname-admin
    fi
done



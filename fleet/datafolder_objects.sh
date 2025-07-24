#!/bin/bash
_ns=$1
dfname=$2

kubectl ${kube_config} -n $namespace get df

if [ -z "$dfname" ]; then
    echo "Please input data folder name."
    exit 0
fi

read datafolder_template_hash datafolder_version datafolder_name < <(kubectl ${kube_config} -n $namespace get datafolder $dfname -o jsonpath='{.metadata.annotations.data\.falcon\.io\/template-hash} {.spec.template.version} {.spec.template.folderName}')
echo "DOWNLOAD FORMAT: ${datafolder_name}(fn)-${datafolder_version}(ver)-${datafolder_template_hash}(hash)-<machine_name>" >&2
kubectl ${kube_config} -n $namespace get download -l data.falcon.io/template-hash=$datafolder_template_hash
echo "LINK FORMAT: ${datafolder_name}(fn)-${datafolder_version}(ver)-${datafolder_template_hash}(hash)-<podid>" >&2
kubectl ${kube_config} -n $namespace get links -l data.falcon.io/template-hash=$datafolder_template_hash

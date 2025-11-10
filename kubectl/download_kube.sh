#!/bin/bash
uc=uc-eus-gen
#cc-eus-ppe
region=`echo $1| cut -d '-' -f2`
cc="cc-$region-gen"
if [ -n "$2" ]; then
    cc="$2"
fi
cl=$1
echo "cc: $cc, sc: $cl"

if [ "$MSYSTEM" = "MINGW64" ]; then
    echo "Run dlkube from within wsl"
    exit 1    
fi


if [ -z "$kube_config" ]; then
    echo "kube_config is not set!"
    exit 1
fi

kube_config_path=$(echo "$kube_config" | cut -d' ' -f2)
query="curl http://localhost:7575/kubeconfig --request GET --data '{\"envname\": \"production\", \"controlclustername\": \"${cc}\", \"clustername\": \"${cl}\"}'"
pod=$(kubectl $kube_config -n clusterfleet get pods -l "app=kubeconfig-fetcher" --context $uc-admin -o json | ${JQ_CMD} -r '.items[0].metadata.name')
kubectl $kube_config -n clusterfleet exec -it $pod -c utility --context $uc-admin -- /bin/bash -c "$query"
echo "Copying kubeconfig from $pod for cluster $cl"
kubectl $kube_config cp -c utility clusterfleet/$pod:/tmp/kubeconfigs/clusters/$cl kubeconf --context $uc-admin

if [ ! -f kubeconf ]; then
    echo "Failed to copy kubeconfig from $pod for cluster $cl"
    exit 1
fi

KUBECONFIG=kubeconf:$kube_config_path kubectl config view --flatten > merged-kubeconfig.yaml
cp -a $kube_config_path $kube_config_path-bkp
cp -a merged-kubeconfig.yaml $kube_config_path
rm -rf merged-kubeconfig.yaml
rm -rf kubeconf
kubectl $kube_config config delete-context $cl-admin
kubectl $kube_config config rename-context  $cl $cl-admin

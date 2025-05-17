cc=cc-wus2-gen
mws=$(kubectl  --kubeconfig /mnt/q/kube-config get mw -l "apis.clusterfleet.io/quota-work" -A --context $cc-admin -o json | jq -r '.items[] | .metadata.name + " " + .metadata.namespace')
IFS=$'\n' read -d '' -ra mw_array <<< "$mws"
for mw in "${mw_array[@]}"; do
    IFS=" " read -r -a array <<< "$mw"
    name=${array[0]}
    ns=${array[1]}
    # kubectl -n $ns get mw $name --context $cc-admin
    kubectl  --kubeconfig /mnt/q/kube-config -n $ns patch mw $name  --type=json -p='[{"op": "remove", "path": "/spec/workload/manifests/0/spec/hard/pods"}]' --context $cc-admin
    # echo "Data $name :: $ns"
    # kubectl -n $ns patch mw webxt-llm.resource-quota  --type=json -p='[{"op": "remove", "path": "/spec/workload/manifests/0/spec/hard/pods"}]'
 
done
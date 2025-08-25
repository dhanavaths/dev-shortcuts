#!/bin/bash

# Loop over all namespaces
for _ns in falcon-demo falcon-core clusterfleet default; do
  # Loop over all pods in the namespace
  for resource_type in pod deploy dd dl; do
    for resource_name in $(kubectl get $resource_type -n "$_ns" -o jsonpath='{.items[*].metadata.name}'); do
      echo "Processing $resource_type: $resource_name in namespace: $_ns"
      # Get the pod JSON
      kubectl patch $resource_type $resource_name -n $_ns -p '{"metadata":{"finalizers":null}}' --type=merge
    done
  done
done

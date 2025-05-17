#!/bin/bash

# Loop over all namespaces
for namespace in clusterfleet falcon-core default; do
  # Loop over all pods in the namespace
  for resource_type in pod deploy; do
    for resource_name in $(kubectl get pod -n "$namespace" -o jsonpath='{.items[*].metadata.name}'); do
      echo "Processing $resource_type: $resource_name in namespace: $namespace"
      # Get the pod JSON
      kubectl patch $resource_type $resource_name -n $namespace -p '{"metadata":{"finalizers":null}}' --type=merge
    done
  done
done

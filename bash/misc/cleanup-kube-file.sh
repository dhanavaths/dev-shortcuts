#!/bin/bash

# Backup kubeconfig
CONFIG_FILE="${1:-/mnt/q/kube-config}"
CONFIG_FILE="--kubeconfig $CONFIG_FILE"
BACKUP_FILE="$CONFIG_FILE.bak.$(date +%Y%m%d%H%M%S)"
cp "$CONFIG_FILE" "$BACKUP_FILE"
echo "✅ Backup saved to: $BACKUP_FILE"



# Get referenced clusters and users
referenced_clusters=$(kubectl $kube_context $CONFIG_FILE config view -o jsonpath='{.contexts[*].context.cluster}' | tr ' ' '\n' | sort | uniq)
referenced_users=$(kubectl $kube_context $CONFIG_FILE config view -o jsonpath='{.contexts[*].context.user}' | tr ' ' '\n' | sort | uniq)

echo "referenced_clusters: $referenced_clusters"
echo "referenced_users: $referenced_users"


# Get all clusters and users
all_clusters=$(kubectl $kube_context $CONFIG_FILE config view -o jsonpath='{.clusters[*].name}' | tr ' ' '\n' | sort | uniq)
all_users=$(kubectl $kube_context $CONFIG_FILE config view -o jsonpath='{.users[*].name}' | tr ' ' '\n' | sort | uniq)

echo "all_clusters: $all_clusters"
echo "all_users: $all_users"


# Delete unreferenced clusters
for cluster in $all_clusters; do
    if ! echo "$referenced_clusters" | grep -qx "$cluster"; then
        echo "🗑️  Deleting orphaned cluster: $cluster"
        kubectl $kube_context $CONFIG_FILE config delete-cluster "$cluster"
    fi
done



# Delete unreferenced users
for user in $all_users; do
    if ! echo "$referenced_users" | grep -qx "$user"; then
        echo "🗑️  Deleting orphaned user: $user"
        kubectl $kube_context $CONFIG_FILE config delete-user "$user"
    fi
done



echo "✅ Kubeconfig cleanup complete."
 
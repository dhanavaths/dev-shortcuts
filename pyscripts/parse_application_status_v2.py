from prettytable import PrettyTable


YELLOW_COLOR = "\033[0;33m"
DEFAULT_COLOR = "\033[0m"


def get_status_version(data):
    status = data.get('status') or {}
    if 'aggregatedStatus' in status or 'targetVersionStatus' in status:
        return 2
    return 1


def print_status_tables(data, print_section_header):
    status = data['status']
    print_section_header('Cluster Statuses (v2):')
    cluster_table = PrettyTable()
    cluster_table.field_names = ["Cluster", "Status", "Replicas", "SC Obs.Gen", "Gen Diff", "Rollout Gen", "Upgrading", "Last Status Observed", "Message"]
    upgrading_clusters = set(status.get('clustersUpgrading', []))
    clusters = sorted(status.get('clusters', []), key=lambda cluster: cluster['cluster'])

    for cluster in clusters:
        cluster_name = cluster['cluster']
        ready_condition = next(
            (condition for condition in cluster.get('conditions', []) if condition['type'] == 'Ready'),
            {}
        )
        ready = ready_condition.get('status', 'Unknown')
        observed_generation = cluster.get('observedGeneration')
        generation_diff = (
            observed_generation - data['metadata']['generation']
            if observed_generation is not None else '-'
        )
        message = ''
        if ready != 'True':
            message = ready_condition.get('message', ready_condition.get('reason', ''))

        cluster_table.add_row([
            cluster_name if ready == 'True' else YELLOW_COLOR + cluster_name + DEFAULT_COLOR,
            ready,
            cluster.get('replicas', '-'),
            observed_generation if observed_generation is not None else '-',
            generation_diff,
            cluster.get('inRolloutGeneration', '-'),
            'Yes' if cluster_name in upgrading_clusters else 'No',
            cluster.get('lastManifestStatusObservedTime', '-'),
            message,
        ])

    print(cluster_table)
    print(f"Clusters with status: {len(clusters)}")
    if 'updatedClusters' in status:
        print(f"Updated clusters: {status['updatedClusters']}")
    print()

    print_section_header('Replica Statuses:')
    replica_table = PrettyTable()
    replica_table.field_names = ["Scope", "Desired", "Current", "Updated", "Ready", "Available", "Non.Schedulable"]
    replica_fields = (
        'desiredReplicas', 'replicas', 'updatedReplicas', 'readyReplicas',
        'availableReplicas', 'nonSchedulableReplicas'
    )
    for label, key in (('Aggregated', 'aggregatedStatus'), ('Target Version', 'targetVersionStatus')):
        if key in status:
            replica_status = status[key] or {}
            replica_table.add_row([label] + [replica_status.get(field, '-') for field in replica_fields])
    print(replica_table)
    print()

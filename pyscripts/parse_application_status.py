import json
import sys
import yaml
from prettytable import PrettyTable

framework = 1
daemonset = 2
deployment = 3
statefulset = 4

def _print_err(msg):
    print("\033[0;31m" + msg + "\033[0m")

def _print_as_table(data_list, err_list=None):
    print ("=" * 80)
    col_widths = [max(len(str(item)) for item in column) for column in zip(*data_list)]

    i = 0
    for row in data_list:
        _r = "".join(f"{str(item):<{col_widths[i]}}  " for i, item in enumerate(row))
        print(_r)

    if err_list:
        for row in err_list:
            _r = "".join(f"{str(item):<{col_widths[i]}}  " for i, item in enumerate(row))
            _print_err(_r)

def _get_ready_condition(cond_list):
    if not cond_list:
        return '', ''

    for condition in cond_list:
        if condition['type'] == 'Ready':
            return condition['status'], condition['reason']

    return 'Unknown', 'Unknown'

def _parse_framework(data):
    cluster_data = [('CLUSTER', 'POD_NAME', 'TASK_STATE', 'OBSERVED_GENERATION', 'READY?', 'REASON')]
    cluster_data_errors = []
    state_counts = {}
    total_task_count = 0
    job_tasks_count = 0
    for clobj in data['status']['clusters']:
        clname = clobj['cluster'] 
        status, reason = _get_ready_condition(clobj.get('conditions'))
        _obs_gen = clobj.get('observedGeneration') or '-'
        
        if not clobj.get('manifestStatuses'):
            cluster_data_errors.append((clname, '', '', _obs_gen, status, reason))
            continue

        for msstatus in clobj['manifestStatuses']:
            if msstatus['kind'] != 'Framework':
                continue

            if not msstatus.get('status') or not msstatus['status'].get('attemptStatus') or not msstatus['status']['attemptStatus'].get('taskRoleStatuses'):                
                cluster_data_errors.append((clname, '', '', _obs_gen, status, reason))
                continue

            for task_role_status_list in msstatus['status']['attemptStatus']['taskRoleStatuses']:
                for task in task_role_status_list['taskStatuses']:

                    pod_name = ''
                    if task.get('attemptStatus'):
                        pod_name = task['attemptStatus']['podName']

                    _task_state = task.get('state') or msstatus['status'].get('status') or ''
                    cluster_data.append((clname, pod_name, _task_state, _obs_gen, status, reason))

                    state_counts.setdefault((clname, _task_state), 0)
                    state_counts[(clname, _task_state)] += 1

                    if pod_name.find('taskmanager') != -1:
                        total_task_count += 1
                    
                    if pod_name.find('jobmanager') != -1:
                        job_tasks_count += 1

    _print_as_table(cluster_data, cluster_data_errors)
    print()
    print(f"taskmanager role pods: {total_task_count}")
    print(f"jobmanager  role pods: {job_tasks_count}")

    
    # ask details
    _task_details_data = [('ROLE', 'COUNTS', 'RESOURCES')]
    for _object in data['spec']['workload']:
        
        kind = _object['manifest']['kind']
        if kind != 'Framework':
            continue

        for task_role in _object['manifest']['spec']['taskRoles']:
            task_name = task_role['name']
            task_count = task_role.get('taskNumber') or 1

            for c in task_role['task']['pod']['spec']['containers']:
                _task_details_data.append((task_name, task_count, str(c['resources'])))
    
    _print_as_table(_task_details_data)

    # task counts
    count_data_list = []
    for key, count in state_counts.items():
        count_data_list.append((key[0], key[1], count))
    count_data_list.sort(key=lambda x: x[0])
    count_data_list = [('CLUSTER', 'TASK_STATE', 'COUNT')] + count_data_list
    _print_as_table(count_data_list)
    
    spec_list = [('NAMESPACE', 'NAME', 'GENERATION', 'OBSERVED_GENERATION', 'APPLICATION_STATE')]
    spec_list.append((data['metadata']['namespace'], data['metadata']['name'], data['metadata']['generation'], data['status']['observedGeneration'], data['status']['applicationState']))
    _print_as_table(spec_list)

def get_app_type(data):
    for _object in data['spec']['workload']:
        kind = _object['manifest']['kind']
        if kind == 'Framework':
            return framework
        elif kind == 'DaemonSet':
            return daemonset
        elif kind == 'Deployment':
            return deployment
        elif kind == 'StatefulSet':
            return statefulset
    return 0 


def __print_deployment_manifest_statuses_table(data):
    print('Manifest Statuses:')
    mw_status_table = PrettyTable()
    mw_status_table.field_names = ["Cluster", "Status", "Desired Replicas", "Replicas", "Avail Replicas(N)", "Non.Sch Replicas(N)", "SC/WL Obs.Gen", "Gen Diff", "Collision Count", "Message"]
    for cluster in data['status'].get('clusters', []):
        cluster_name = cluster["cluster"]
        for manifest in cluster.get('manifestStatuses', []):
            kind = manifest["kind"]
            if kind not in ['Deployment', 'StatefulSet']:
                continue

            status = manifest["status"]
            
            status_available = False
            message = ''
            for condition in manifest["status"].get('conditions', []):
                if condition["type"] == "Available":
                    status_available = condition["status"]
                    message = condition["message"]
            
            _gen_diff = cluster.get('observedGeneration', 0) - data['metadata']['generation']
            mw_status_table.add_row([
                cluster_name,
                status_available,
                status.get('desiredReplicas', 0),
                status.get('replicas', 0),
                status.get('availableNewReplicas', 0),
                status.get('nonSchedulableNewReplicas', 0),
                status.get('observedGeneration', 0),
                _gen_diff,
                status.get('collisionCount', 0),
                message,
            ])
    print(mw_status_table)
    print()

def __print_application_conditions(data):
    print('Application Conditions:')
    conditions_table = PrettyTable()
    conditions_table.field_names = ["Type", 'Status', "Reason", "Message", "Observed Generation"]
    for condition in data['status'].get('conditions', []):
        if condition['type'] == 'Validation':
            continue

        conditions_table.add_row([
            condition['type'],
            condition['status'],
            condition['reason'],
            condition['message'],
            condition.get('observedGeneration', 0)
        ])
    print(conditions_table)
    print()


def __print_application_information(data):
    print('Application Information:')
    info_table = PrettyTable()
    info_table.field_names = ["Version", "Generation", "Observed Generation", "Rollout Status", "Application State", 'LKG Version', 'Images']
    images = set()
    for workload in data['spec']['workload']:
        if workload['manifest']['kind'] not in ['Deployment', 'Daemonset', 'StatefulSet']:
            continue

        for container in workload['manifest']['spec']['template']['spec']['containers']:
            images.add(container['image'])

    info_table.add_row([
        data['spec']['version'],
        data['metadata']['generation'],
        data['status']['observedGeneration'],
        data['status']['rolloutStatus'],
        data['status'].get('applicationState', ''),
        data['status'].get('lastKnownGoodVersion', ''),
        images
    ])
    print(info_table)
    print('rolloutStrategy:\n', data['spec'].get('rolloutStrategy', ''))
    print('rollbackStrategy:\n', data['spec'].get('rollbackStrategy', ''))

def _parse_deployment(data):
    if not data.get('status'):
        _print_err('No status found')
        return
    __print_deployment_manifest_statuses_table(data)
    __print_application_conditions(data)
    __print_application_information(data)


def __print_daemonset_manifest_statuses_table(data):
    cl_counters = 0
    mw_status_table = PrettyTable()
    print('Manifest Statuses:')
    mw_status_table.field_names = ["Cluster", "Status", "Desired", "Scheduled(N)", "Scheduled(Diff)", "Cur Scheduled", "Available", "Ready", "Mis. Scheduled", "SC/WL Obs.Gen", "Gen Diff", "Message"]
    for cluster in data['status'].get('clusters', []):
        cluster_name = cluster["cluster"]
        if not cluster.get('manifestStatuses', []):
            continue
        cl_counters += 1
        for manifest in cluster['manifestStatuses']:
            kind = manifest["kind"]
            if kind not in ['DaemonSet']:
                continue

            status = manifest["status"]
            
            status_available = False
            message = ''
            for condition in manifest["status"].get('conditions', []):
                if condition["type"] == "Available":
                    status_available = condition["status"]
                    message = condition["message"]
            

            _gen_diff = cluster.get('observedGeneration', 0) - data['metadata']['generation']
            mw_status_table.add_row([
                cluster_name,
                status_available,
                status.get('desiredNumberScheduled', 0),
                status.get('updatedNumberScheduled', 0),
                status.get('updatedNumberScheduled', 0) - status.get('desiredNumberScheduled', 0),
                status.get('numberAvailable', 0),
                status.get('numberReady', 0),
                status.get('currentNumberScheduled', 0),
                status.get('numberMisscheduled', 0),
                status.get('observedGeneration', 0),
                _gen_diff,
                message,
            ])
    print(mw_status_table)
    print("Clusters with status: %s", cl_counters)
    print()

def _parse_daemonset(data):
    if not data.get('status'):
        _print_err('No status found')
        return
    __print_daemonset_manifest_statuses_table(data)
    __print_application_conditions(data)
    __print_application_information(data)

def _main():
    try:
        handle = open('/tmp/_app.json', 'rb')
        raw_data = handle.read()
        loaded_data = json.loads(raw_data)
    except Exception as e:
        loaded_data = yaml.safe_load(raw_data)

    if not loaded_data:
        _print_err('Failed to load data, please check the context/application name.')
        sys.exit(0)

    app_type = get_app_type(loaded_data)
    if app_type == framework:
        _parse_framework(loaded_data)
    elif app_type == daemonset:
        _parse_daemonset(loaded_data)
    elif app_type == deployment:
        _parse_deployment(loaded_data)
    elif app_type == statefulset:
        _parse_deployment(loaded_data)
    else:
        print('Unknown application type')
        sys.exit(1)        

if __name__ == '__main__':
    _main()
import json
import os
import sys
import yaml # pip install pyyaml
import pprint
from prettytable import PrettyTable # pip install prettytable

framework = 1
daemonset = 2
deployment = 3
statefulset = 4

RED_COLOR = "\033[0;31m"
YELLOW_COLOR = "\033[0;33m"
BRIGHT_CYAN_COLOR = "\033[1;36m"
DEFAULT_COLOR = "\033[0m" 

def _print_err(msg):
    print(RED_COLOR + msg + DEFAULT_COLOR)

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

def _print_section_header(msg):
    print(BRIGHT_CYAN_COLOR + msg + DEFAULT_COLOR)

def _get_ready_condition(cond_list):
    if not cond_list:
        return '', ''

    for condition in cond_list:
        if condition['type'] == 'Ready':
            return condition['status'], condition['reason']

    return 'Unknown', 'Unknown'

def get_rolling_update_max_unavailable(manifest):
    if manifest.get('spec') and manifest['spec'].get('updateStrategy') and \
       manifest['spec']['updateStrategy'].get('rollingUpdate') and manifest['spec']['updateStrategy'].get('type') == 'RollingUpdate':
        if manifest['spec']['updateStrategy'].get('rollingUpdate') and manifest['spec']['updateStrategy']['rollingUpdate'].get('maxUnavailable'):
            return manifest['spec']['updateStrategy']['rollingUpdate']['maxUnavailable']
    return ''

def _parse_framework(data):
    if not data.get('status'):
        print('Status not found!')
        return

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
    print(f"total pods           : {total_task_count + job_tasks_count}")
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
    total_count = 0
    for key, count in state_counts.items():
        count_data_list.append((key[0], key[1], count))
        total_count += count
    count_data_list.sort(key=lambda x: x[0])
    count_data_list = [('CLUSTER', 'TASK_STATE', 'COUNT')] + count_data_list + [('TOTAL', '', total_count)]
    _print_as_table(count_data_list)
    
    spec_list = [('NAMESPACE', 'NAME', 'GENERATION', 'OBSERVED_GENERATION', 'APPLICATION_STATE')]
    spec_list.append((data['metadata']['namespace'], data['metadata']['name'], data['metadata']['generation'], data['status']['observedGeneration'], data['status']['applicationState']))
    _print_as_table(spec_list)

def get_app_type(data):
    for _object in data['spec']['workload']:
        kind = _object['manifest']['kind'].lower()
        if kind == 'framework':
            return framework
        elif kind == 'daemonset':
            return daemonset
        elif kind == 'deployment':
            return deployment
        elif kind == 'statefulset':
            return statefulset
    return 0 


def __print_deployment_manifest_statuses_table(data):
    _print_section_header('Manifest Statuses:')
    mw_status_table = PrettyTable()
    mw_status_table.field_names = ["Cluster", "Status", "Desired Replicas", "Current Replicas", "Avail Replicas(N)", "AR(N)-DES", "Non.Sch Replicas(N)", "SC/WL Obs.Gen", "Gen Diff", "Collision Count", "Message"]

    status_table_rows = []
    if data['status'].get('clusters', []):
        data['status']['clusters'].sort(key=lambda x: x['cluster'])

    for cluster in data['status'].get('clusters', []):
        cluster_name = cluster["cluster"]

        status_available = False
        message = ''
        for condition in cluster.get('conditions', []):
            if condition["type"] == "Ready":
                status_available = condition["status"]
                if status_available != 'True':
                    message = condition["message"]

        _add_empty_row = len(message) != 0

        for manifest in cluster.get('manifestStatuses', []):
            kind = manifest["kind"]
            if kind not in ['Deployment', 'StatefulSet']:
                continue

            status = manifest["status"]
            
            _gen_diff = cluster.get('observedGeneration', 0) - data['metadata']['generation']

            # if _add_empty_row:
            #     mw_status_table.add_row(['-'] * len(mw_status_table.field_names))    

            status_table_rows.append([
                cluster_name, #0
                cluster_name if status_available == 'True' else (YELLOW_COLOR + cluster_name + DEFAULT_COLOR),
                status_available,
                status.get('desiredReplicas', 0),
                status.get('replicas', 0),
                status.get('availableNewReplicas', 0),
                status.get('availableNewReplicas', 0) - status.get('desiredReplicas', 0), #6
                status.get('nonSchedulableNewReplicas', 0),
                status.get('observedGeneration', 0),
                _gen_diff,
                status.get('collisionCount', 0),
                message,
            ])
            # reset message. it's enough to show it under one OS slice
            message = ''

        # if _add_empty_row:
        #     mw_status_table.add_row(['-'] * len(mw_status_table.field_names))    

    total_desired = 0
    total_current = 0
    total_available = 0
    for row in sorted(status_table_rows, key=lambda x: (-x[6], x[0])):
        row.pop(0)
        total_desired += row[2]
        total_current += row[3]
        total_available += row[4]
        mw_status_table.add_row(row)

    mw_status_table.add_row(['TOTAL', '', total_desired, total_current, total_available, total_available - total_desired, '', '', '', '', ''])
    print(mw_status_table)
    print()

def __print_application_conditions(data):
    _print_section_header('Application Conditions:')
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

def __get_application_spec(data):
    result = {
        'rolloutStrategy': data['spec'].get('rolloutStrategy', {}),
        'rollbackStrategy': data['spec'].get('rollbackStrategy', {}),
        'resources': {},
        'affinities': {},
        'nodeSelector': {},
    }
    for _object in data['spec']['workload']:
        _mainifest = _object['manifest']

        if _mainifest['kind'].lower() not in ['deployment', 'statefulset', 'daemonset']:
            continue

        if _mainifest['spec']['template']['spec'].get('affinity'):
            result['affinities'][_mainifest['metadata']['name']] = _mainifest['spec']['template']['spec']['affinity']

        for container in _mainifest['spec']['template']['spec']['containers']:
            if container.get('resources'):
                result['resources'][container['name']] = container['resources']
            if container.get('nodeSelector'):
                result['nodeSelector'][container['name']] = container['nodeSelector']

    return result

def __print_application_information(data, is_daemonset=False):
    _print_section_header('Application Information:')
    info_table = PrettyTable()
    info_table.field_names = ["Version", "Generation", "Observed Generation", "Rollout Status", "Application State", 'LKG Version', 'Images']
    images = set()
    replicas = 0
    for workload in data['spec']['workload']:
        if workload['manifest']['kind'].lower() not in ['deployment', 'daemonset', 'statefulset']:
            continue
        
        if workload['manifest']['spec'].get('replicas'):
            replicas += int(workload['manifest']['spec']['replicas'])

        for container in workload['manifest']['spec']['template']['spec']['containers']:
            images.add(container['image'].rsplit('/', 1)[-1])

    info_table.add_row([
        data['spec'].get('version', ''),
        data['metadata']['generation'],
        data['status']['observedGeneration'],
        data['status'].get('rolloutStatus', ''),
        data['status'].get('applicationState', ''),
        data['status'].get('lastKnownGoodVersion', ''),
        "\n".join(images)
    ])
    print(info_table)
    
    if is_daemonset:
        replicas = -1

    _print_section_header('Replicas:')
    print(f'{replicas}')
    application_spec = __get_application_spec(data)

    _print_section_header("Container Resources:")
    print(json.dumps(application_spec['resources'], indent=2))

def __print_daemonset_manifest_statuses_table(data):
    cl_counters = 0
    mw_status_table = PrettyTable()
    _print_section_header('Manifest Statuses:')
    mw_status_table.field_names = ["Cluster", "Status", "Desired", "Scheduled(N)", "Scheduled(Diff)", "Cur Scheduled", "Ready", "Available", "Mis. Scheduled", "SC/WL Obs.Gen", "Gen Diff", "Message"]
    if data['status'].get('clusters', []):
        data['status']['clusters'].sort(key=lambda x: x['cluster'])

    status_table_rows = []
    for cluster in data['status'].get('clusters', []):
        cluster_name = cluster["cluster"]
        if not cluster.get('manifestStatuses', []):
            continue

        status_available = False
        message = ''
        for condition in cluster.get('conditions', []):
            if condition["type"] == "Ready":
                status_available = condition["status"]
                if status_available != 'True':
                    message = condition["message"]

        cl_counters += 1
        _add_empty_row = len(message) != 0

        for manifest in cluster['manifestStatuses']:
            kind = manifest["kind"]
            if kind not in ['DaemonSet']:
                continue

            status = manifest["status"]
            

            _gen_diff = cluster.get('observedGeneration', 0) - data['metadata']['generation']
            # if _add_empty_row:
            #     mw_status_table.add_row([' '] * len(mw_status_table.field_names))    

            status_table_rows.append([
                cluster_name, #0
                cluster_name if status_available == 'True' else (YELLOW_COLOR + cluster_name + DEFAULT_COLOR),
                status_available,
                status.get('desiredNumberScheduled', 0),
                status.get('updatedNumberScheduled', 0),
                status.get('updatedNumberScheduled', 0) - status.get('desiredNumberScheduled', 0), #5
                status.get('currentNumberScheduled', 0),
                status.get('numberReady', 0),
                status.get('numberAvailable', 0),
                status.get('numberMisscheduled', 0),
                status.get('observedGeneration', 0),
                _gen_diff,
                message,
            ])

            # reset message. it's enough to show it under one OS slice
            message = ''

        # if _add_empty_row:
        #     mw_status_table.add_row([' '] * len(mw_status_table.field_names))    

    total_desired = 0
    total_ready = 0
    total_available = 0
    for row in sorted(status_table_rows, key=lambda x: (-x[5], x[0])):
        row.pop(0)
        total_desired += row[2]
        total_ready += row[6]
        total_available += row[7]
        mw_status_table.add_row(row)

    mw_status_table.add_row(['TOTAL', '', total_desired, '', '', '', total_ready, total_available, '', '', '', ''])
    print(mw_status_table)
    print(f"Clusters with status: {cl_counters}")
    print()

def __print_application_spec(data):
    application_spec = __get_application_spec(data)
    _print_section_header('Rollout Strategy:')
    print(json.dumps(application_spec['rolloutStrategy'], indent=2))

    _print_section_header('Rollback Strategy:')
    print(json.dumps(application_spec['rollbackStrategy'], indent=2))

    if application_spec.get('affinities'):
        _print_section_header("Pod Affinity:")
        print(json.dumps(application_spec['affinities'], indent=2))
    
    if application_spec.get('nodeSelector'):
        _print_section_header("Node Selector:")
        print(json.dumps(application_spec['nodeSelector'], indent=2))

def _parse_regular_workload(data, is_daemonset=False):
    if not data.get('status'):
        _print_err('Status not found!')
        return
    __print_application_spec(data)
    if is_daemonset:
        __print_daemonset_manifest_statuses_table(data)
    else:
        __print_deployment_manifest_statuses_table(data)
    __print_application_conditions(data)
    __print_application_information(data, is_daemonset)

def _main():
    try:
        home_dir = os.path.expanduser("~")
        handle = open(f'{home_dir}/dev-temp-data/_app.json', 'rb')
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
        _parse_regular_workload(loaded_data, True)
    elif app_type == deployment:
        _parse_regular_workload(loaded_data)
    elif app_type == statefulset:
        _parse_regular_workload(loaded_data)
    else:
        print('Unknown application type')
        sys.exit(1)        

if __name__ == '__main__':
    _main()
import os, sys, json
import pprint, copy
import yaml
from collections import OrderedDict
import random
import string
import ruamel.yaml
import subprocess

# kube - get cl -oyaml | grep "subnet-3" | grep -v "{"

template_path_prefix = './fleet-documents/island-definition'
template_path_prefix = './fleet-documents/cluster-type'
cluster_docs_path_prefix = './fleet-documents/clusters'

ruamel_yaml = ruamel.yaml.YAML()

def _parse_template(file_name):
    
    file_path = file_name if file_name.find('/') != -1 else os.path.join(template_path_prefix, file_name)
    handle = open(file_path, 'r')
    loaded_data = ruamel_yaml.load(handle.read().replace('\t', ''))

    l = []
    for k in loaded_data['spec']['properties']:
        l.append(k)
    # print("Loaded spec.properties keys: %s" % (', '.join(l),))

    klist = ['networkIsland', 'kubernetesVersion', 'subscriptionId', 'resourceGroupName', 'location', 'shortRegion', 'boundary', 'orchestrator', 'cloud', 'disableNodeOSImageAutoUpgrade', 'subnetResourceId', 'useSubnetNatGateway', 'dnsZoneSuffix', 'provisionerMSIObjectID', 'acrResourceId', 'submarinerAgentJobImage', 'kubeletIdentityResourceId', 'excludedCrds', 'agentPoolProfiles']
    for k in klist:
        if k not in loaded_data['spec']['properties']:
            raise ValueError(f"Missing key '{k}' in template {file_name}.")

        if not loaded_data['spec']['properties'][k]:
            raise ValueError(f"Key '{k}' in template {file_name} is invalid.")

    return loaded_data  

def get_family_and_vcores(vm_size):
    # print(vm_size)
    try:
        _, cores_family, ver = vm_size.split('_', 2)
    except ValueError:
        cores_family, ver = vm_size.split('_', 2)

    family = ""
    cores = ""
    for c in cores_family:
        if c.isdigit():
            cores += c
        else:
            family += c
    return family.upper() + ver, int(cores)            


def _parse_template_main(file_name, print_summary=False):
    data = _parse_template(file_name)
    total_pods = 0
    total_instances = 0
    total_cores = 0
    sku_counts = {}
    agent_pool_profiles = json.loads(data['spec']['properties']['agentPoolProfiles'])
    agent_pool_profiles_patch_list = json.loads(data['spec']['properties'].get('agentPoolProfilesPatch', '[]'))
    agent_pool_profiles_patch_map = {e['name']: e for e in agent_pool_profiles_patch_list}
    for profile in agent_pool_profiles:
        total_instances = max(int(profile['count']), agent_pool_profiles_patch_map.get(profile['name'], {}).get('count', 0))
        total_pods += int(total_instances) * int(profile['maxPods'])

        family, cores = get_family_and_vcores(profile['vmSize'])
        sku_counts.setdefault(profile['vmSize'], {'5:maxPods': 0, '4:count': 0, '2:cores': 0, '1:family': family, '3:gens': set()})
        sku_counts[profile['vmSize']]['4:count'] += int(total_instances)
        sku_counts[profile['vmSize']]['5:maxPods'] += int(profile['maxPods'])
        sku_counts[profile['vmSize']]['2:cores'] += int(total_instances) * cores
        sku_counts[profile['vmSize']]['3:gens'].add(profile['name'])
        total_cores += int(total_instances) * cores

    for sku in sku_counts:
        sku_counts[sku]['3:gens'] = ', '.join(sku_counts[sku]['3:gens'])
        sku_counts[sku]['0:size'] = sku
    
    sku_family_order = ['DSv3', 'Dv3', 'DASv4', ]
    sku_counts = [sku_counts[k] for k in sku_counts if sku_counts[k]['2:cores']]
    _printable_data = {
        'totalInstaces': total_instances,
        'totalPods': total_pods,
        'totalCores': total_cores
    }

    for k in ['shortRegion', 'subscriptionId', 'networkIsland']:        
        _printable_data[k] = data['spec']['properties'].get('networkIsland')

    subnet_res_id_comps = data['spec']['properties']['subnetResourceId'].split('/')
    _printable_data['vnet'] = subnet_res_id_comps[4]
    _printable_data['subnet'] = subnet_res_id_comps[-1]
    _printable_data['SKUs'] = sku_counts
    if print_summary:
        print(yaml.dump(_printable_data), file=sys.stderr)

    _parsed_data = {}
    for k in _printable_data:
        _parsed_data[k] = _printable_data[k]

    _parsed_data['object'] = data
    
    print(f'{_printable_data["vnet"][-2:]}:{_printable_data["subnet"][-1]}', file=sys.stdout)
    return _parsed_data

cluster_ids = {}

def gen_cluster_ids(cluster_count):
    command = "~/scripts/kubectl/k_wrapper.sh - get cl | grep -v NAME | grep -v kubectl | cut -d' ' -f1 | cut -d'-' -f3"
    result = subprocess.run(command, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode != 0:
        os._exit(1)
    
    cur_cid_list = result.stdout.decode().split('\n')
    print(cur_cid_list, file=sys.stderr)
    while cluster_count > 0:
        # Generate a random 5-character string with lowercase letters and digits
        for i in range(1000):
            random_string = ''.join(random.choices(string.ascii_lowercase + string.digits, k=5))
        if random_string in cluster_ids or random_string in cur_cid_list or random_string[0].isdigit():
            continue
        cluster_ids[random_string] = 1
        cluster_count -= 1


def _generate_cluster_documents(file_name, subnet_config_list):
    gen_cluster_ids(len(subnet_config_list))

    _gen_cluster = None
    _loaded_data = _parse_template(file_name)
    i = 0
    print(subnet_config_list, file=sys.stderr)
    for cid in cluster_ids:
        cluster_config = copy.deepcopy(_loaded_data)

        cluster_name = cluster_config['metadata']['name'].rsplit('-', 1)[0] + '-' + cid
        cluster_config['metadata']['name'] = cluster_name
        cluster_config['spec']['properties']['resourceGroupName'] = cluster_config['spec']['properties']['resourceGroupName'].rsplit('-', 1)[0] + '-' + cid

        vnet_id, subnet_id = subnet_config_list[i].split(':')
        subnet_res_id_comps = list(cluster_config['spec']['properties']['subnetResourceId'].split('/'))
        subnet_res_id_comps[4] = subnet_res_id_comps[4].rsplit('-', 1)[0] +  '-'  + vnet_id
        subnet_res_id_comps[-1] = subnet_res_id_comps[-1].rsplit('-', 1)[0] + '-' + subnet_id
        cluster_config['spec']['properties']['subnetResourceId'] = '/'.join(subnet_res_id_comps)

        short_region =  cluster_config['spec']['properties']['shortRegion']
        _output_file_name = os.path.join(cluster_docs_path_prefix, short_region, '%s.yaml' % (cluster_name,))
        
        if _gen_cluster is None:
            _input = input("generate cluster?[y] ")
            _gen_cluster = _input == 'y'

        if _gen_cluster:
            print(_output_file_name, file=sys.stderr)
            handle = open(_output_file_name, 'w')
            ruamel_yaml.dump(cluster_config, handle)
            handle.close()
        else:
            print(json.dumps(cluster_config, indent=4), file=sys.stderr)
        i += 1

if __name__ == '__main__':
    print(f'args: {sys.argv}', file=sys.stderr)
    # python3 ~/scripts/pyscripts/sc_template_parser.py EUS2-2.03.000.yaml 03:3
    if len(sys.argv) > 2:
        _generate_cluster_documents(sys.argv[1], sys.argv[2:])
    else:
        _parse_template_main(sys.argv[1], True)

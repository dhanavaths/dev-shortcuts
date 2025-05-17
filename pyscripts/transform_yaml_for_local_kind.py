import os, sys, json
import pprint, copy
import yaml
from collections import OrderedDict
import random
import string
import ruamel.yaml
import subprocess

ruamel_yaml = ruamel.yaml.YAML()

def _parse_template(file_path):
    with open(os.path.join(os.getcwd(), file_path), 'r') as handle:
        loaded_data = list(ruamel_yaml.load_all(handle))
        return loaded_data  

def _dump_template(file_path, data_list):    
    with open(os.path.join(os.getcwd(), file_path), 'w') as handle:
        ruamel_yaml.dump_all(data_list, handle)
        handle.write('\n')

def _main(file_path): 
    doc_list = _parse_template(file_path)
    for doc in doc_list:
        if doc.get('kind') != 'Deployment':
            continue
        pod_spec = doc['spec']['template']['spec']
        if pod_spec.get('volumes') is None:
            pod_spec['volumes'] = []
        pod_spec['volumes'].append({
            'name': 'docker-sock-1',
            'hostPath': {
                'path': '/var/run/docker.sock'
            }
        })

        for container in pod_spec['containers']:
            if container.get('volumeMounts') is None:
                container['volumeMounts'] = []
            container['volumeMounts'].append({
                'name': 'docker-sock-1',
                'mountPath': '/var/run/docker.sock'
            })
            
            if container.get('env') is None:
                container['env'] = []
            container['env'].append({
                'name': 'CF_STANDARD_CLUSTER_NAME',
                'value': 'kind'
            })
            container['env'].append({
                'name': 'CLUSTER_ENVIRONMENT',
                'value': 'testing'
            })
            image_name = container['image'].split(':')[0].split('/')[-1]
            container['image'] = input("Enter the image in place of [%s]: (%s)?" % (container['image'], image_name))
            if not container['image']:
                container['image'] = image_name

    _dump_template(file_path, doc_list)

if __name__ == '__main__':
    print(sys.argv)
    if len(sys.argv) < 2:
        print("Usage: python transform_yaml_for_local_kind.py <file_path>")
        sys.exit(1)
    else:
        _main(sys.argv[1])

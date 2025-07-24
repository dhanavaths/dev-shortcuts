import os
import subprocess
import ruamel.yaml
import re

data_root = f"{os.getenv('HOME')}/work_data"
data_dir = 'wus2_apps'
# data_dir = 'ppe_apps'

def ensure_directories():
    if not os.path.exists(data_root):
        os.makedirs(data_root)

    for dir_path in [f'{data_root}/{data_dir}', f'{data_root}/skip_{data_dir}']:
        if not os.path.exists(dir_path):
            os.makedirs(dir_path)

kctl_prefix = 'kubectl --kubeconfig /mnt/q/kube-config'
def get_app_list():
    _cmd = f'{kctl_prefix} get app -A -o=custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name' 
    result = subprocess.run(_cmd.split(), capture_output=True, text=True)
    apps_list = []
    for line in result.stdout.splitlines():
        if line.startswith("NAMESPACE"):
            continue
        namespace, app_name = line.split()
        apps_list.append((namespace, app_name))
    return apps_list
 
def edit_yaml_content(yaml_content, handle):
    ruamel_yaml = ruamel.yaml.YAML()
    _loaded_content = ruamel_yaml.load(yaml_content)
    _loaded_content.pop('status', None)
    if _loaded_content.get('metadata', {}).get('annotations'):
        _loaded_content['metadata']['annotations'].pop('kubectl.kubernetes.io/last-applied-configuration', None)
    for k in ['finalizers', 'creationTimestamp', 'resourceVersion', 'uid', 'generation']:
        _loaded_content['metadata'].pop(k, None)
    return ruamel_yaml.dump(_loaded_content, handle)

def dump_app_spec(): 
    apps_list = get_app_list()
    for namespace, app_name in apps_list:
        _file_path = f'{data_root}/{data_dir}/{namespace}_{app_name}.yaml'
        _skip_file_path = f'{data_root}/skip_{data_dir}/{namespace}_{app_name}.yaml'

        if os.path.exists(_file_path):
            print(f"File already exists: {_file_path}")
            continue 
        elif os.path.exists(_skip_file_path):
            print(f"Skip file already exists: {_skip_file_path}")
            continue 

        _cmd = f'{kctl_prefix} -n {namespace} get app {app_name} -o yaml'
        result = subprocess.run(_cmd.split(), capture_output=True, text=True)
        if result.returncode == 0:
            yaml_content = result.stdout
            if yaml_content.find('beta.kubernetes.io/os:') != -1:
                yaml_content = yaml_content.replace('beta.kubernetes.io/os:', 'kubernetes.io/os:')
                with open(_file_path, 'w') as f:
                    edit_yaml_content(yaml_content, f)
            else:
                with open(_skip_file_path, 'w') as f:
                    f.write("")
        else:
            print(f"Error dumping app spec for {namespace}/{app_name}: {result.stderr}")
            continue

dry_run_mode = False
def check_app_exists(namespace, app_name):
    if dry_run_mode:
        return True

    _cmd = f'{kctl_prefix} -n {namespace} get app {app_name}'
    result = subprocess.run(_cmd.split(), capture_output=True, text=True)
    return result.returncode == 0

def patch_apps():
    global dry_run_mode
    dry_run = "--dry-run=server"
    dry_run = ''
    if dry_run:
        dry_run_mode = True

    for file_name in os.listdir(f'{data_root}/{data_dir}'):
        c1, _ = file_name.split('.')
        namespace, app_name = c1.split('_')
        _file_path = f'{data_root}/{data_dir}/{namespace}_{app_name}.yaml'
        
        yaml_content = open(_file_path, 'r').read()        
        matches = re.findall('kubernetes.io/os:', yaml_content)
        if len(matches) > 1 and data_dir.find('ppe') == -1:
            print(f"Multiple matches found in {namespace}/{app_name}: {matches}")
            continue

        if check_app_exists(namespace, app_name):
            _cmd = f'{kctl_prefix} -n {namespace} apply -f {_file_path} {dry_run}' 
            print(_cmd)
            result = subprocess.run(_cmd.split(), capture_output=True, text=True)
            if result.returncode != 0:
                print(f"Error patching app {namespace}/{app_name}: {result.stderr}")

if __name__ == "__main__":
    # python3 ~/dev-shortcuts/pyscripts/patch_beta_os_label_nodeselector.py
    # Get the list of applications
    print("Starting script...")
    ensure_directories()
    # dump_app_spec()
    patch_apps()
 
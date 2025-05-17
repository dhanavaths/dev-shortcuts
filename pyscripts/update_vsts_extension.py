import os
import json
import uuid
import sys

_major = 0
_minor = 0
_patch = 0
indent = 4

def edit_task_json(task_json):
    handle = open(task_json, "r")
    json_data = json.load(handle)
    handle.close()
    
    json_data["id"] = str(uuid.uuid4())
    # already used by Arathi for dev
    # json_data["id"] = 'd1f0bf33-db0a-4d6a-8bee-5cb7415f8542'
    json_data["name"] = json_data["name"] + '-Dev'

    for _n in ['instanceNameFormat', 'friendlyName', 'description']:
        json_data[_n] = 'Dev ' + json_data[_n]

    json_data["version"] = {
        "Major": _major,
        "Minor": _minor,
        "Patch": _patch
    }
    
    try:
        json_data["visibility"].remove("Preview")
    except valueError:
        pass

    handle = open(task_json, "w")
    json.dump(json_data, handle, indent=indent)
    handle.close()

def edit_vss_extension_json(vss_extension_json):
    handle = open(vss_extension_json, "r")
    json_data = json.load(handle)
    handle.close()
    json_data['id'] = json_data['id'] + '-dev'
    json_data['name'] = json_data['name'] + ' Dev'
    json_data['description'] = 'Dev ' + json_data['description']
        
    json_data["version"] = "%s.%s.%s" % (_major, _minor, _patch)    
    json_data.pop("galleryFlags", None)
    json_data.pop("preview", None)


    json_data["contributions"][0]["id"] = json_data["contributions"][0]["id"] + '-dev'

    handle = open(vss_extension_json, "w")
    json.dump(json_data, handle, indent=indent)
    handle.close()

def edit_package_json(package_json):
    handle = open(package_json, "r")
    json_data = json.load(handle)
    handle.close()
    json_data["name"] = json_data["name"] + '-dev'
    json_data["version"] = "%s.%s.%s" % (_major, _minor, _patch)
    json_data["description"] = json_data["description"] + ' Dev'

    handle = open(package_json, "w")
    json.dump(json_data, handle, indent=2)
    handle.close()

def _main():
    
    task_json = "buildandreleasetask/task.json"
    vss_extension_json = "vss-extension.json"
    package_json = "package.json"

    edit_task_json(task_json)
    edit_vss_extension_json(vss_extension_json)
    edit_package_json(package_json)

if __name__ == "__main__":
    _major, _minor, _patch = [int(e) for e in sys.argv[1].split('.')]
    _main()
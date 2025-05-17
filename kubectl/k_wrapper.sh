#!/bin/bash
SCRIPTDIR_WRAPPER="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "${SCRIPTDIR_WRAPPER}/../bash/extensions.sh"
kube_prod $@

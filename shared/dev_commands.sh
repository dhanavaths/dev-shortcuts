#!/bin/bash
SCRIPTDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
function _apply_file()
{
    kl - apply -f $1
}

function _execute_dev_commands() {
    local _command=$1
    shift
    case $1 in
        demo-app|demo-app-pr|demo-app-ds|demo-app-ss)
            ${SCRIPTDIR}/../bash/eval.sh _local_kubectl cf apply ${1}
            ;;
        ds)
            ${SCRIPTDIR}/../bash/eval.sh _local_kubectl li scheduler
            kubectl -n default delete deployment scheduler-deployment
            kubectl -n default delete lease scheduler-leaderelection
            ${SCRIPTDIR}/../bash/eval.sh _local_kubectl - apply scheduler
            ;;
        status|s)
            local suffix=$2
            if [ -n "$suffix" ]; then
                suffix="-$suffix"
            fi
            while true; do
                tput clear
                ${SCRIPTDIR}/../bash/eval.sh _local_kubectl cf get app status demo-app$suffix
                sleep 2
            done
            ;;                    
        desc|d)
            ${SCRIPTDIR}/../bash/eval.sh _local_kubectl cf desc app demo-app
            ;;            
        export-kubes)
            kind get clusters | awk '{}{print "kind export kubeconfig --name " $$0}{}' | sh
            ;;
        *)
            echo "Command not found"
            ;;
    esac
}

_execute_dev_commands $@
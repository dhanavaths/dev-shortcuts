#!/bin/bash

# Get the number of processes from the first command line argument, default to 1 if empty
# ~/dev-shortcuts/work/hey_parallel_runs.sh wrk 8081 1000 3 10 20 30
# ~/dev-shortcuts/work/hey_parallel_runs.sh hey 8081 1000 1 10000 100
tool_name=${1:-"wrk"}
service_port=${2:-10001}
file_size=${3:-1000}
num_processes=${4:-1}
endpoint="http://localhost:$service_port/falcon-core.demo-app-rwpr/sync"
# endpoint="http://localhost:$service_port/falcon-core.demo-app-rwpr/dynamic_payload?size=$file_size"
# endpoint="http://localhost:$service_port/dynamic_payload?size=$file_size"

echo "Starting $num_processes parallel processes..."

# Loop to start background processes
for i in $(seq 1 $num_processes); do
    echo "Launching process $i..."
    if [ "$tool_name" == "hey" ]; then
        requests_per_process=${5:-10000}
        concurrency_per_process=${6:-100}
        echo "hey -n $requests_per_process -c $concurrency_per_process -h2 \"$endpoint\"" 
        hey -n $requests_per_process -c $concurrency_per_process -h2 "$endpoint" > "/tmp/result_$i.txt" &
    else
        num_of_threads=${5:-1}
        number_of_open_connections=${6:-100}
        test_duration=${7:-30}
        echo "wrk -t$num_of_threads -c$number_of_open_connections -d${test_duration}s --latency \"$endpoint\""
        wrk -t$num_of_threads -c$number_of_open_connections -d${test_duration}s --latency "$endpoint" > "/tmp/result_$i.txt" &
    fi
done

echo "Load tests running... waiting for completion."

# Wait for all background processes launched by this script to finish
wait

echo -e "\n--- ALL RESULTS ---\n"

# Loop again to output and clean up files
for i in $(seq 1 $num_processes); do
    echo "--- Result from Process $i ---"
    cat "/tmp/result_$i.txt"
    rm "/tmp/result_$i.txt"
    echo -e "\n"
done

echo "All tests completed."
#!/bin/sh

# --- CONFIGURATION & INPUT ---
usage() {
    echo "Usage: $0 -n <namespace> -p <pod-name> -c <container-name>"
}

while getopts "n:p:c:h" opt; do
    case "$opt" in
        n) NAMESPACE=$OPTARG ;;
        p) POD_NAME=$OPTARG ;;
        c) CONTAINER_NAME=$OPTARG ;;
        h)
            usage
            exit 0
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done

if [ -z "$NAMESPACE" ] || [ -z "$POD_NAME" ] || [ -z "$CONTAINER_NAME" ]; then
    usage
    echo "❌ Error: namespace, pod name, and container name are required."
    exit 1
fi

echo "Using Namespace: $NAMESPACE"
echo "Using Pod: $POD_NAME"
echo "Using Container: $CONTAINER_NAME"
echo "------------------------------------------------"
# -----------------------------

echo "====== STARTING SIGTERM PROFILER FOR .NET ======"

# 1. Capture start timestamp with millisecond precision
START_TIME=$(date +%s%3N)
START_DISPLAY=$(date +"%T.%3N")

echo "Sending SIGTERM to container '$CONTAINER_NAME' inside pod '$POD_NAME'..."
echo "Dispatched at: $START_DISPLAY"

START_RESTART_COUNT=$(kubectl get pod -n "$NAMESPACE" "$POD_NAME" -o jsonpath="{.status.containerStatuses[?(@.name=='$CONTAINER_NAME')].restartCount}" 2>/dev/null)
if [ -z "$START_RESTART_COUNT" ]; then
    START_RESTART_COUNT=0
fi
echo "Starting restart count: $START_RESTART_COUNT"

# 2. Fire SIGTERM (Kill -15) directly at PID 1 inside the targeted container
kubectl exec -n "$NAMESPACE" "$POD_NAME" -c "$CONTAINER_NAME" -- kill -15 1 2>/dev/null

if [ $? -ne 0 ]; then
    echo "❌ Error: Failed to send signal. Is the pod name correct and running?"
    exit 1
fi

echo "Monitoring container shutdown state..."

# 3. Poll the pod status until the targeted container transitions out of 'running'
while true; do
    CONTAINER_STATE=$(kubectl get pod -n "$NAMESPACE" "$POD_NAME" -o jsonpath="{.status.containerStatuses[?(@.name=='$CONTAINER_NAME')].state.running}" 2>/dev/null)
    CURRENT_RESTART_COUNT=$(kubectl get pod -n "$NAMESPACE" "$POD_NAME" -o jsonpath="{.status.containerStatuses[?(@.name=='$CONTAINER_NAME')].restartCount}" 2>/dev/null)
    
    # If the JSON string returns empty, the state block is no longer 'running'
    if [ -z "$CONTAINER_STATE" ]; then
        break
    fi
    sleep 0.25
done

# 4. Calculate the precise duration
END_TIME=$(date +%s%3N)
END_DISPLAY=$(date +"%T.%3N")

# Math calculations handled via awk to support floating point numbers safely
DURATION=$(awk -v start="$START_TIME" -v end="$END_TIME" 'BEGIN { print (end - start) / 1000 }')

echo "================================================"
echo "Container stopped at:  $END_DISPLAY"
echo "Total Stop Duration:   $DURATION seconds"
if [ -z "$CURRENT_RESTART_COUNT" ]; then
    CURRENT_RESTART_COUNT=$START_RESTART_COUNT
fi
echo "Restart count:         $CURRENT_RESTART_COUNT (was $START_RESTART_COUNT)"
echo "================================================"

# 5. Evaluate the result boundary
IS_TRAPPED=$(awk -v dur="$DURATION" 'BEGIN { if (dur >= 29.0 && dur <= 32.0) print 1; else print 0 }')

if [ "$IS_TRAPPED" -eq 1 ]; then
    echo "⚠️ WARNING: The container stopped right at the 30-second mark."
    echo "This proves your .NET app is hitting the default 30s 'HostOptions.ShutdownTimeout' wall."
    echo "Active reverse proxy connections are being forcefully aborted!"
else
    echo "✅ SUCCESS: The container took $DURATION seconds to stop."
    echo "It successfully processed its shutdown sequence outside the default 30s trap window."
fi
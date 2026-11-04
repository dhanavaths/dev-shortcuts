#!/usr/bin/env bash
# run-experiment.sh - drive a packet-drop experiment and produce a
# side-by-side summary of baseline vs proposed router behavior.
#
# Phases:
#   1. clean network (60s)  -> establish baseline-good numbers
#   2. moderate loss 10%    (120s)
#   3. heavy loss 30% + 100ms+/-20ms delay (180s)
#   4. clean network        (60s)  -> recovery
#
# Run AFTER `make deploy`.
#
# If the script is interrupted and netem is left applied, clear it
# manually with:
#   kubectl --context kind-dev-control-cluster -n netdrop \
#     exec deploy/service-discovery -c netem -- \
#     sh -c 'tc qdisc del dev eth0 root 2>/dev/null || true'
set -euo pipefail

NS="${NS:-netdrop}"
CTX="${CTX:-kind-dev-control-cluster}"
KCTL=(kubectl --context "$CTX" -n "$NS")

phase() {
  local name="$1" duration="$2" loss="$3" delay="$4" jitter="$5"
  echo
  echo "================================================================"
  echo " PHASE: $name  (loss=$loss delay=$delay jitter=$jitter ${duration}s)"
  echo "================================================================"

  # Build the netem spec and apply LIVE via `tc qdisc replace` inside
  # the running netem sidecar. We avoid `kubectl set env` because it
  # recreates the pod, and under loss/delay the new pod's readiness
  # probe fails -> Service endpoint never flips -> router keeps
  # hitting an unaffected (old) endpoint.
  local spec="netem"
  if [ -n "$delay" ]; then
    spec="$spec delay $delay"
    [ -n "$jitter" ] && spec="$spec $jitter"
  fi
  if [ -n "$loss" ] && [ "$loss" != "0%" ] && [ "$loss" != "0" ]; then
    spec="$spec loss $loss"
  fi

  if [ "$spec" = "netem" ]; then
    # Clean network phase - remove the qdisc entirely.
    "${KCTL[@]}" exec deploy/service-discovery -c netem -- \
      sh -c "tc qdisc del dev eth0 root 2>/dev/null || true"
    echo "[$(date +%T)] applied: <no qdisc>"
  else
    "${KCTL[@]}" exec deploy/service-discovery -c netem -- \
      sh -c "tc qdisc replace dev eth0 root $spec"
    echo "[$(date +%T)] applied: $spec"
  fi

  # Verify it's actually installed (and report it back).
  "${KCTL[@]}" exec deploy/service-discovery -c netem -- \
    tc qdisc show dev eth0 | sed 's/^/   tc> /'

  echo "[$(date +%T)] sleeping ${duration}s..."
  sleep "$duration"
}

snapshot() {
  local label="$1"
  echo "--- $label snapshot ---"
  for v in baseline proposed; do
    pod=$("${KCTL[@]}" get pod -l app=router,variant=$v -o jsonpath='{.items[0].metadata.name}')
    echo "[$v] last 2 WINDOW lines:"
    "${KCTL[@]}" logs "$pod" --tail=200 | grep WINDOW | tail -n 2 || true
  done
}

trap '"${KCTL[@]}" exec deploy/service-discovery -c netem -- sh -c "tc qdisc del dev eth0 root 2>/dev/null || true" >/dev/null 2>&1 || true' EXIT

phase "1-clean"             60 "0%"  ""      ""
snapshot "after clean baseline"
phase "2-pure-delay"        60 "0%"  "600ms" "20ms"
snapshot "after pure 600ms delay (no loss)"
phase "3-moderate"          60 "10%" ""      ""
snapshot "after moderate loss"
phase "4-heavy"             60 "30%" "100ms" "20ms"
snapshot "after heavy loss + 100ms delay"
phase "5-heavy-big-delay"   60 "30%" "600ms" "20ms"
snapshot "after heavy loss + 600ms delay"
phase "6-recovery"          60 "0%"  ""      ""
snapshot "after recovery"

echo
echo "Experiment complete. Full logs:"
echo "  ${KCTL[*]} logs deploy/router-baseline --tail=-1 > baseline.log"
echo "  ${KCTL[*]} logs deploy/router-proposed --tail=-1 > proposed.log"

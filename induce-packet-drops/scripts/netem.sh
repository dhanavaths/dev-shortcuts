#!/bin/sh
# netem.sh - configure tc/netem on a container interface.
#
# Env vars:
#   NETEM_IFACE  - interface (default eth0)
#   NETEM_LOSS   - loss percent, e.g. "30%" (default "0%")
#   NETEM_DELAY  - delay, e.g. "100ms" (default "")
#   NETEM_JITTER - delay jitter, e.g. "20ms" (default "")
#   NETEM_DURATION - if set, sleep this long then clear and exit.
#                    If empty, keep the qdisc and tail -f /dev/null.
#   NETEM_CYCLE  - "on=30s,off=30s" toggles on/off in a loop forever.
#
# Requires NET_ADMIN.
set -eu

IFACE="${NETEM_IFACE:-eth0}"
LOSS="${NETEM_LOSS:-0%}"
DELAY="${NETEM_DELAY:-}"
JITTER="${NETEM_JITTER:-}"

apply() {
  local loss="$1"
  local delay="$2"
  tc qdisc del dev "$IFACE" root 2>/dev/null || true
  local spec="netem"
  if [ -n "$delay" ]; then
    spec="$spec delay $delay"
    [ -n "$JITTER" ] && spec="$spec $JITTER"
  fi
  if [ -n "$loss" ] && [ "$loss" != "0%" ] && [ "$loss" != "0" ]; then
    spec="$spec loss $loss"
  fi
  echo "[netem] tc qdisc add dev $IFACE root $spec"
  tc qdisc add dev "$IFACE" root $spec
}

clear() {
  echo "[netem] clearing qdisc on $IFACE"
  tc qdisc del dev "$IFACE" root 2>/dev/null || true
}

trap clear EXIT INT TERM

if [ -n "${NETEM_CYCLE:-}" ]; then
  # Format: on=30s,off=30s
  ON=$(echo "$NETEM_CYCLE" | sed -n 's/.*on=\([^,]*\).*/\1/p')
  OFF=$(echo "$NETEM_CYCLE" | sed -n 's/.*off=\([^,]*\).*/\1/p')
  : "${ON:=30s}"
  : "${OFF:=30s}"
  echo "[netem] cycle on=$ON off=$OFF loss=$LOSS delay=$DELAY"
  while :; do
    apply "$LOSS" "$DELAY"
    sleep "$ON"
    clear
    sleep "$OFF"
  done
fi

apply "$LOSS" "$DELAY"

if [ -n "${NETEM_DURATION:-}" ]; then
  echo "[netem] sleeping $NETEM_DURATION then clearing"
  sleep "$NETEM_DURATION"
  exit 0
fi

# Keep container alive while qdisc is in place.
tail -f /dev/null &
wait $!

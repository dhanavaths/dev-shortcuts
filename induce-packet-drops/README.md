# induce-packet-drops

A reproducible experiment that proves loosening the router → service-discovery
timeouts (connect `500ms → 5s`, overall `1500ms → 15s`) materially improves
stability under bad-network conditions, without hurting the clean-network path.

## Why the proposed numbers

Linux default TCP SYN retransmit schedule is roughly `1s, 2s, 4s, 8s …`
(see `tcp_syn_retries`). With a **500ms connect timeout** the kernel gives up
*before* it has retransmitted even the first SYN — so a single dropped SYN is
guaranteed to fail the connect. Bumping connect to **5s** lets at least one
SYN retransmit succeed.

The 1500ms overall timeout has the same problem for established connections:
any retransmit (initial RTO ≥200ms, then doubles) eats most of the budget,
and one dropped data segment usually trips the timeout. **15s** lets a normal
TCP retransmit recover within a single request instead of forcing the
application-level retry path — which, with `backoff = 500ms · 2ⁿ`, compounds
a transient blip into minutes of cache staleness (and a liveness-probe restart).

## What's in here

| Path | Purpose |
| --- | --- |
| [cmd/discovery/main.go](cmd/discovery/main.go) | Fake service-discovery HTTP server (`/services`, `/healthz`). |
| [cmd/router/main.go](cmd/router/main.go) | Router load-gen: configurable connect/request timeouts, exponential retry, per-window + cumulative stats, simulated liveness check based on cache staleness. |
| [scripts/netem.sh](scripts/netem.sh) | Sidecar that applies `tc qdisc … netem loss/delay` on the pod's `eth0`. Supports steady-state and on/off cycling. |
| [scripts/run-experiment.sh](scripts/run-experiment.sh) | Drives the 4-phase experiment (clean → moderate → heavy → recovery) and prints per-phase snapshots. |
| [yaml/discovery.yaml](yaml/discovery.yaml) | Discovery `Deployment` + `Service` with the netem sidecar (NET_ADMIN). |
| [yaml/router-baseline.yaml](yaml/router-baseline.yaml) | Router with prod-current timeouts (500ms / 1500ms). |
| [yaml/router-proposed.yaml](yaml/router-proposed.yaml) | Router with proposed timeouts (5s / 15s). |
| [yaml/netem-job.yaml](yaml/netem-job.yaml) | Optional `Job` that drives the netem sidecar env from inside the cluster. |

Both routers point at the **same** discovery service and run **concurrently**,
so they see identical network conditions — any difference in the numbers is
attributable to the timeout policy alone.

## Running it

Requires a kind cluster (default: `dev-control-cluster`). Pods need
`NET_ADMIN`; kind allows this by default.

```sh
cd induce-packet-drops
make deploy                  # build, kind-load, apply all manifests
make logs-baseline           # in one terminal
make logs-proposed           # in another
./scripts/run-experiment.sh  # drives the 4 phases
```

To poke it manually instead of running the script:

```sh
# turn drops on
kubectl -n netdrop set env deploy/service-discovery -c netem \
  NETEM_LOSS=30% NETEM_DELAY=100ms NETEM_JITTER=20ms

# turn drops off
kubectl -n netdrop set env deploy/service-discovery -c netem \
  NETEM_LOSS=0% NETEM_DELAY= NETEM_JITTER=
```

## What to look for in the logs

Each router prints a line every 10s like:

```
[baseline] WINDOW attempts=5 ok=1 retries=8 avgMs=1480 | TOTAL ok=42/120 (35.0%) retries=180 maxLatMs=1502 cacheStale=58210ms DEAD(liveness-would-fail)
[proposed] WINDOW attempts=5 ok=5 retries=2 avgMs= 820 | TOTAL ok=118/120 (98.3%) retries=  7 maxLatMs=4310 cacheStale=  2010ms ALIVE
```

The variables that matter for the manager pushback:

- **`ok=...` per window** — success rate during the loss phase.
- **`retries`** — proposed should retry far less because individual attempts succeed.
- **`maxLatMs`** — proves the proposed path doesn't blow up on the clean network (it's bounded by actual RTT, not the timeout).
- **`cacheStale` + `ALIVE/DEAD`** — baseline routinely crosses the
  liveness-probe threshold (→ container restart in prod). Proposed stays ALIVE.

## Expected results (rule-of-thumb)

| Phase           | Baseline ok% | Proposed ok% | Baseline restarts | Proposed restarts |
| --------------- | -----------: | -----------: | ----------------: | ----------------: |
| clean           |       ~100%  |       ~100%  |                 0 |                 0 |
| 10% loss        |       ~60–80% |       ~99%  |              0–1  |                 0 |
| 30% loss + delay |      ~10–30% |       ~95%  |              ≥1   |                 0 |
| recovery        |       ~100%  |       ~100%  |                 0 |                 0 |

If proposed *doesn't* clearly win, the experiment has likely produced no
useful loss (check `kubectl exec` → `tc qdisc show dev eth0`).

## Promoting to a stability test

The router's `/stats` endpoint (port 9090) returns JSON; a CI runner can
scrape it after each phase and assert thresholds (e.g.
`successes/attempts > 0.9` for proposed during the heavy-loss phase, and
`cacheStale < livenessMax`). The same image and manifests can be reused;
only `run-experiment.sh` needs to be replaced with a test harness that
fails the build when thresholds are missed.

## Cleanup

```sh
make clean
kubectl --context kind-dev-control-cluster delete ns netdrop
```

# Connection Churn Load Test Report

## Overview

This test compares request handling and connection churn with and without Envoy-level load balancing. Each scenario uses 2,000 prewarmed persistent connections and recycles 10% of the connections every 2 seconds.

- Date: September 4, 2026
- Target endpoint: `https://router.falcon-core.ingress.wus2.microsoft-falcon.net/falcon-core.ahprobe/keepalive`
- Test client: `goscripts/load-test-with-connections-churn.go`
- Connection count: 2,000
- Connection recycle rate: 10% every 2 seconds
- Duration per test: 10 minutes
- Grafana dashboard: [Load test dashboard](https://grafana-falconfleet-gycserhce8guhyeu.wus3.grafana.azure.com/goto/afx61vxvgh534a?orgId=1)

> The timezone for the schedule was not specified. Add it before correlating these windows with Grafana data.

## Test Matrix

| Scenario | Load pattern | Start | Expected end | Request configuration |
|---|---|---:|---:|---|
| Envoy balancing, test 1 | Continuous | 00:03:30 | 00:13:30 | 2,000 requests in flight |
| Envoy balancing, test 2 | Random burst | 00:15:30 | 00:25:30 | 1,000-2,000 requests/s |
| No Envoy balancing, test 1 | Continuous | 00:29:00 | 00:39:00 | 2,000 requests in flight |
| No Envoy balancing, test 2 | Random burst | 00:42:00 | 00:52:00 | 1,000-2,000 requests/s |

## Test Commands

### Envoy-Level Load Balancing

#### Test 1: Continuous traffic

- Start: September 4, 2026 at 00:03:30
- Traffic: continuously maintain 2,000 in-flight requests

```bash
go run goscripts/load-test-with-connections-churn.go \
  -c 2000 \
  -rcpct 10 \
  -cri 2s \
  -d 10m \
  -f 2000
```

#### Test 2: Random burst traffic

- Start: September 4, 2026 at 00:15:30
- Traffic: randomly select a target of 1,000-2,000 requests each second
- Maximum concurrency: 2,500 requests, from the current `-f` default

```bash
go run goscripts/load-test-with-connections-churn.go \
  -c 2000 \
  -rcpct 10 \
  -cri 2s \
  -d 10m \
  -min-rps 1000 \
  -max-rps 2000
```

### Without Envoy-Level Load Balancing

#### Test 1: Continuous traffic

- Start: September 4, 2026 at 00:29:00
- Traffic: continuously maintain 2,000 in-flight requests

```bash
go run goscripts/load-test-with-connections-churn.go \
  -c 2000 \
  -rcpct 10 \
  -cri 2s \
  -d 10m \
  -f 2000
```

#### Test 2: Random burst traffic

- Start: September 4, 2026 at 00:42:00
- Traffic: randomly select a target of 1,000-2,000 requests each second
- Maximum concurrency: 2,500 requests, from the current `-f` default

```bash
go run goscripts/load-test-with-connections-churn.go \
  -c 2000 \
  -rcpct 10 \
  -cri 2s \
  -d 10m \
  -min-rps 1000 \
  -max-rps 2000
```

## Methodology

1. Apply the required load-balancing configuration before each pair of tests.
2. Confirm the probe endpoint returns `2xx` before starting.
3. Start the client at the scheduled time and verify that all 2,000 connections are prewarmed.
4. Capture the complete client summary after each 10-minute run.
5. Correlate the exact test window with Grafana, including a short baseline before and after the run.
6. Allow the system to stabilize between tests; the schedule provides at least a two-minute gap.

The continuous tests are concurrency-driven and attempt to keep `-f 2000` requests active. The burst tests are arrival-driven: once per second, the client randomly chooses an inclusive request count between 1,000 and 2,000 and dispatches that batch, subject to the default concurrency ceiling of 2,500.

## Results

No terminal summaries or exported Grafana measurements were provided with this report. Record the observed values below after each run.

| Scenario | Requests/s | Success rate | 4xx | 5xx | Errors | Timeouts | Mean latency | Max latency | Data rate |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Envoy, continuous | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending |
| Envoy, burst | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending |
| No Envoy, continuous | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending |
| No Envoy, burst | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending | Pending |

## Grafana Review

For each test window, capture and compare:

- Request rate and response-code distribution
- End-to-end and upstream latency percentiles
- Active, opened, and closed downstream connections
- Connection churn and reconnect rate
- Load distribution across backend instances
- Envoy pending requests, retries, resets, and upstream connection failures
- CPU, memory, and network utilization for Envoy and backend instances
- Saturation or imbalance during the randomized bursts

## Comparison

Use like-for-like comparisons after results are collected:

| Comparison | Primary question |
|---|---|
| Envoy continuous vs. no-Envoy continuous | Does Envoy improve backend distribution without materially increasing latency or errors? |
| Envoy burst vs. no-Envoy burst | Does Envoy absorb randomized bursts more evenly and reduce backend saturation? |
| Continuous vs. burst within each mode | How does each configuration respond to arrival-rate variability? |
| Connection recycle events | Do 10% recycle events every 2 seconds correlate with latency spikes, `5xx`, resets, or uneven load? |

## Conclusions

Pending collection of the four client summaries and Grafana measurements. Conclusions should explicitly cover throughput, latency, error rate, backend distribution, and behavior during connection recycle events.

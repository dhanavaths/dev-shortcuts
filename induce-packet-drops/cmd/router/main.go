// router: a load generator that mimics the prod "router" component.
// It periodically refreshes its discovery cache by calling
// service-discovery. The retry/timeout policy is configurable so the
// same binary can run as either the BASELINE (connect=500ms,
// overall=1500ms) or the PROPOSED (connect=5s, overall=15s) variant.
//
// Retry policy follows production: backoff = base * 2^attempt.
//
// The binary tracks per-window stats and a "cache freshness" gauge:
// time since the last successful refresh. In prod this drives the
// liveness probe; here we just log it so the experiment can show how
// long the cache went stale under packet loss.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type config struct {
	url            string
	connectTimeout time.Duration
	requestTimeout time.Duration
	retryBase      time.Duration
	maxRetries     int
	refreshEvery   time.Duration
	livenessMax    time.Duration
	label          string
}

type stats struct {
	mu              sync.Mutex
	attempts        int64
	successes       int64
	failures        int64
	retries         int64
	totalLatencyMs  int64
	maxLatencyMs    int64
	lastSuccess     time.Time
	maxStalenessMs  int64
	livenessFailed  int64 // windows where cache exceeded livenessMax
}

func (s *stats) recordAttempt(latency time.Duration, ok bool, retries int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	ms := latency.Milliseconds()
	s.totalLatencyMs += ms
	if ms > s.maxLatencyMs {
		s.maxLatencyMs = ms
	}
	s.retries += int64(retries)
	if ok {
		s.successes++
		s.lastSuccess = time.Now()
	} else {
		s.failures++
	}
}

func (s *stats) snapshot(now time.Time) (out struct {
	Attempts, Successes, Failures, Retries int64
	AvgMs, MaxMs                           int64
	StaleMs                                int64
}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out.Attempts = s.attempts
	out.Successes = s.successes
	out.Failures = s.failures
	out.Retries = s.retries
	if s.attempts > 0 {
		out.AvgMs = s.totalLatencyMs / s.attempts
	}
	out.MaxMs = s.maxLatencyMs
	if !s.lastSuccess.IsZero() {
		out.StaleMs = now.Sub(s.lastSuccess).Milliseconds()
	} else {
		out.StaleMs = -1
	}
	if out.StaleMs > s.maxStalenessMs {
		s.maxStalenessMs = out.StaleMs
	}
	return
}

func newClient(cfg config) *http.Client {
	dialer := &net.Dialer{
		Timeout:   cfg.connectTimeout,
		KeepAlive: 30 * time.Second,
	}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   cfg.connectTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		// Force a fresh TCP connection every call so each refresh
		// exercises the connect path - matches the prod scenario
		// where TLS/gRPC connections are dropped under packet loss.
		DisableKeepAlives: true,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   cfg.requestTimeout,
	}
}

// fetch performs a single refresh with retries. Returns total wall
// time (including retries+backoff), retry count, and error.
func fetch(ctx context.Context, client *http.Client, cfg config) (time.Duration, int, error) {
	start := time.Now()
	var lastErr error
	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := cfg.retryBase * (1 << attempt)
			log.Printf("[%s] retry attempt=%d/%d backoff=%s elapsedMs=%d prevErr=%v",
				cfg.label, attempt, cfg.maxRetries, backoff, time.Since(start).Milliseconds(), lastErr)
			select {
			case <-ctx.Done():
				return time.Since(start), attempt, ctx.Err()
			case <-time.After(backoff):
			}
		}
		reqCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, cfg.url, nil)
		if err != nil {
			cancel()
			return time.Since(start), attempt, err
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		cancel()
		if resp.StatusCode/100 != 2 {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			continue
		}
		return time.Since(start), attempt, nil
	}
	if lastErr == nil {
		lastErr = errors.New("exhausted retries")
	}
	return time.Since(start), cfg.maxRetries, lastErr
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.url, "url", "http://service-discovery:8080/services", "discovery URL")
	flag.DurationVar(&cfg.connectTimeout, "connect-timeout", 500*time.Millisecond, "TCP connect timeout")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 1500*time.Millisecond, "overall per-attempt request timeout")
	flag.DurationVar(&cfg.retryBase, "retry-base", 500*time.Millisecond, "base for exponential backoff")
	flag.IntVar(&cfg.maxRetries, "max-retries", 3, "max retry attempts")
	flag.DurationVar(&cfg.refreshEvery, "refresh-every", 2*time.Second, "refresh interval")
	flag.DurationVar(&cfg.livenessMax, "liveness-max-staleness", 30*time.Second, "max cache staleness before liveness would fail")
	flag.StringVar(&cfg.label, "label", os.Getenv("ROUTER_LABEL"), "label to prefix log lines (e.g. baseline/proposed)")
	reportEvery := flag.Duration("report-every", 10*time.Second, "stats report interval")
	flag.Parse()
	if cfg.label == "" {
		cfg.label = "router"
	}

	atomic.StoreInt64(new(int64), 0) // keep atomic import alive if unused

	client := newClient(cfg)
	st := &stats{lastSuccess: time.Now()}

	log.Printf("[%s] starting: connect=%s request=%s retries=%d base=%s url=%s",
		cfg.label, cfg.connectTimeout, cfg.requestTimeout, cfg.maxRetries, cfg.retryBase, cfg.url)

	// Per-window counters reset every report tick.
	var winAttempts, winSuccess, winRetries, winLatMs int64
	var winMu sync.Mutex

	go func() {
		t := time.NewTicker(*reportEvery)
		defer t.Stop()
		for range t.C {
			snap := st.snapshot(time.Now())
			winMu.Lock()
			wa, ws, wr, wl := winAttempts, winSuccess, winRetries, winLatMs
			winAttempts, winSuccess, winRetries, winLatMs = 0, 0, 0, 0
			winMu.Unlock()
			wavg := int64(0)
			if wa > 0 {
				wavg = wl / wa
			}
			cacheStale := "n/a"
			if snap.StaleMs >= 0 {
				cacheStale = fmt.Sprintf("%dms", snap.StaleMs)
			}
			alive := "ALIVE"
			if snap.StaleMs > cfg.livenessMax.Milliseconds() {
				alive = "DEAD(liveness-would-fail)"
				atomic.AddInt64(&st.livenessFailed, 1)
			}
			log.Printf("[%s] WINDOW attempts=%d ok=%d retries=%d avgMs=%d | TOTAL ok=%d/%d (%.1f%%) retries=%d maxLatMs=%d cacheStale=%s %s",
				cfg.label, wa, ws, wr, wavg,
				snap.Successes, snap.Attempts, pct(snap.Successes, snap.Attempts), snap.Retries, snap.MaxMs,
				cacheStale, alive)
		}
	}()

	// Optional /stats endpoint to scrape from the experiment runner.
	if port := os.Getenv("STATS_PORT"); port != "" {
		go func() {
			http.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
				snap := st.snapshot(time.Now())
				_ = json.NewEncoder(w).Encode(snap)
			})
			_ = http.ListenAndServe(":"+port, nil)
		}()
	}

	ctx := context.Background()
	ticker := time.NewTicker(cfg.refreshEvery)
	defer ticker.Stop()
	for {
		<-ticker.C
		lat, retries, err := fetch(ctx, client, cfg)
		ok := err == nil
		st.recordAttempt(lat, ok, retries)
		winMu.Lock()
		winAttempts++
		winRetries += int64(retries)
		winLatMs += lat.Milliseconds()
		if ok {
			winSuccess++
		}
		winMu.Unlock()
		if !ok {
			log.Printf("[%s] FAIL after %dms retries=%d err=%v", cfg.label, lat.Milliseconds(), retries, err)
		}
	}
}

func pct(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100.0 / float64(b)
}

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/bits"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const defaultEndpoint = "https://router.falcon-core.ingress.wus2.microsoft-falcon.net/falcon-core.ahprobe/keepalive"

type generation struct {
	transport *http.Transport

	mu      sync.Mutex
	active  int
	retired bool
}

func (generation *generation) acquire() {
	generation.mu.Lock()
	generation.active++
	generation.mu.Unlock()
}

func (generation *generation) release() {
	generation.mu.Lock()
	generation.active--
	closeIdle := generation.retired && generation.active == 0
	generation.mu.Unlock()
	if closeIdle {
		generation.transport.CloseIdleConnections()
	}
}

func (generation *generation) retire() {
	generation.mu.Lock()
	generation.retired = true
	closeIdle := generation.active == 0
	generation.mu.Unlock()
	if closeIdle {
		generation.transport.CloseIdleConnections()
	}
}

type connectionSlot struct {
	mu         sync.Mutex
	generation *generation
}

func (slot *connectionSlot) acquire() *generation {
	slot.mu.Lock()
	generation := slot.generation
	generation.acquire()
	slot.mu.Unlock()
	return generation
}

func (slot *connectionSlot) rotate() {
	slot.mu.Lock()
	old := slot.generation
	slot.generation = newGeneration()
	old.retire()
	slot.mu.Unlock()
}

func newGeneration() *generation {
	return &generation{transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          1,
		MaxIdleConnsPerHost:   1,
		MaxConnsPerHost:       1,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}}
}

const timingBuckets = 64

type timingSeries struct {
	count            atomic.Uint64
	sumNanos         atomic.Uint64
	sumMillis        atomic.Uint64
	sumMillisSquared atomic.Uint64
	minNanos         atomic.Uint64
	maxNanos         atomic.Uint64
	buckets          [timingBuckets]atomic.Uint64
}

type metrics struct {
	started   atomic.Uint64
	done      atomic.Uint64
	succeeded atomic.Uint64
	failed    atomic.Uint64
	errored   atomic.Uint64
	timeout   atomic.Uint64
	status    [5]atomic.Uint64
	headers   atomic.Uint64
	data      atomic.Uint64
	request   timingSeries
	connect   timingSeries
	firstByte timingSeries

	ratesMu sync.Mutex
	rates   []float64
}

type requestResult struct {
	statusCode        int
	headerBytes       uint64
	dataBytes         uint64
	duration          time.Duration
	connectDuration   time.Duration
	firstByteDuration time.Duration
}

type httpStatusError struct {
	code   int
	status string
}

func (statusError *httpStatusError) Error() string {
	return fmt.Sprintf("unexpected HTTP status %s", statusError.status)
}

/*
	go run goscripts/load-test-with-connections-churn.go \
	  -c 2000 \
	  -rcpct 10 \
	  -cri 5s \
	  -d 3m \
	  -f 2500 \
	  -min-rps 5000 \
	  -max-rps 10000
*/
func main() {
	endpoint := flag.String("url", defaultEndpoint, "HTTP endpoint to test")
	connections := flag.Int("c", 2000, "number of persistent connections")
	connectionRecyclePercent := flag.Float64("rcpct", 10, "percentage of connections to recreate per recycle interval")
	connectionRecycleInterval := flag.Duration("cri", 5*time.Second, "connection recycle interval")
	duration := flag.Duration("d", 3*time.Minute, "test duration")
	inflight := flag.Int("f", 2500, "number of requests kept in flight")
	minRequestsPerSecond := flag.Int("min-rps", 0, "minimum requests in each one-second burst (requires -max-rps)")
	maxRequestsPerSecond := flag.Int("max-rps", 0, "maximum requests in each one-second burst (requires -min-rps)")
	flag.Parse()

	if err := validateOptions(*endpoint, *connections, *connectionRecyclePercent, *connectionRecycleInterval, *duration, *inflight, *minRequestsPerSecond, *maxRequestsPerSecond); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		flag.Usage()
		os.Exit(2)
	}

	slots := make([]connectionSlot, *connections)
	for index := range slots {
		slots[index].generation = newGeneration()
	}
	defer closeSlots(slots)

	fmt.Printf("warming %d connections to %s\n", *connections, *endpoint)
	if err := warmConnections(*endpoint, slots); err != nil {
		fmt.Fprintln(os.Stderr, "warm-up failed:", err)
		os.Exit(1)
	}

	testContext, cancelTest := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelTest()
	testContext, cancelDuration := context.WithTimeout(testContext, *duration)
	defer cancelDuration()

	started := time.Now()
	var stats metrics
	var workers sync.WaitGroup
	if *minRequestsPerSecond > 0 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			runBurstWorkers(testContext, *endpoint, slots, *inflight, *minRequestsPerSecond, *maxRequestsPerSecond, &stats)
		}()
	} else {
		workers.Add(*inflight)
		for workerID := 0; workerID < *inflight; workerID++ {
			go func(offset int) {
				defer workers.Done()
				runWorker(testContext, *endpoint, slots, offset, *inflight, &stats)
			}(workerID)
		}
	}

	rotations := int(math.Ceil(float64(*connections) * *connectionRecyclePercent / 100))
	var churn sync.WaitGroup
	if rotations > 0 {
		churn.Add(1)
		go func() {
			defer churn.Done()
			rotateConnections(testContext, slots, rotations, *connectionRecycleInterval)
		}()
	}

	if *minRequestsPerSecond > 0 {
		fmt.Printf("running for %s with %d connections, %d max in-flight requests, %d-%d requests/s bursts, and %g%% recycled every %s\n",
			*duration, *connections, *inflight, *minRequestsPerSecond, *maxRequestsPerSecond, *connectionRecyclePercent, *connectionRecycleInterval)
	} else {
		fmt.Printf("running for %s with %d connections, %d in-flight requests, and %g%% recycled every %s\n",
			*duration, *connections, *inflight, *connectionRecyclePercent, *connectionRecycleInterval)
	}
	workers.Wait()
	churn.Wait()

	printSummary(time.Since(started), &stats)
}

func validateOptions(endpoint string, connections int, connectionRecyclePercent float64, connectionRecycleInterval, duration time.Duration, inflight, minRequestsPerSecond, maxRequestsPerSecond int) error {
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("-url must be a valid HTTP or HTTPS URL")
	}
	if connections <= 0 {
		return fmt.Errorf("-c must be greater than zero")
	}
	if connectionRecyclePercent < 0 || connectionRecyclePercent > 100 {
		return fmt.Errorf("-rcpct must be between 0 and 100")
	}
	if connectionRecycleInterval <= 0 {
		return fmt.Errorf("-cri must be greater than zero")
	}
	if duration <= 0 {
		return fmt.Errorf("-d must be greater than zero")
	}
	if inflight <= 0 {
		return fmt.Errorf("-f must be greater than zero")
	}
	if (minRequestsPerSecond == 0) != (maxRequestsPerSecond == 0) {
		return fmt.Errorf("-min-rps and -max-rps must be provided together")
	}
	if minRequestsPerSecond < 0 || maxRequestsPerSecond < 0 {
		return fmt.Errorf("-min-rps and -max-rps must be greater than zero")
	}
	if minRequestsPerSecond > maxRequestsPerSecond {
		return fmt.Errorf("-min-rps must be less than or equal to -max-rps")
	}
	return nil
}

func (stats *metrics) record(result requestResult, err error) {
	stats.done.Add(1)
	stats.headers.Add(result.headerBytes)
	stats.data.Add(result.dataBytes)
	stats.request.record(result.duration)
	stats.connect.record(result.connectDuration)
	stats.firstByte.record(result.firstByteDuration)
	if result.statusCode >= 100 && result.statusCode < 600 {
		stats.status[result.statusCode/100-1].Add(1)
	}

	var statusError *httpStatusError
	switch {
	case err == nil:
		stats.succeeded.Add(1)
	case errors.As(err, &statusError):
		stats.failed.Add(1)
	default:
		stats.errored.Add(1)
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			stats.timeout.Add(1)
		}
	}
}

func (series *timingSeries) record(duration time.Duration) {
	if duration <= 0 {
		return
	}
	nanos := uint64(duration)
	millis := uint64(duration / time.Millisecond)
	series.count.Add(1)
	series.sumNanos.Add(nanos)
	series.sumMillis.Add(millis)
	series.sumMillisSquared.Add(millis * millis)
	updateAtomicMin(&series.minNanos, nanos)
	updateAtomicMax(&series.maxNanos, nanos)
	series.buckets[min(bits.Len64(nanos)-1, timingBuckets-1)].Add(1)
}

func updateAtomicMin(value *atomic.Uint64, candidate uint64) {
	for current := value.Load(); current == 0 || candidate < current; current = value.Load() {
		if value.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func updateAtomicMax(value *atomic.Uint64, candidate uint64) {
	for current := value.Load(); candidate > current; current = value.Load() {
		if value.CompareAndSwap(current, candidate) {
			return
		}
	}
}

type distribution struct {
	min                     float64
	max                     float64
	mean                    float64
	standardDeviation       float64
	withinStandardDeviation float64
}

func (series *timingSeries) snapshot() distribution {
	count := series.count.Load()
	if count == 0 {
		return distribution{}
	}
	meanNanos := float64(series.sumNanos.Load()) / float64(count)
	meanMillis := float64(series.sumMillis.Load()) / float64(count)
	meanSquareMillis := float64(series.sumMillisSquared.Load()) / float64(count)
	variance := max(0, meanSquareMillis-meanMillis*meanMillis)
	standardDeviationNanos := math.Sqrt(variance) * float64(time.Millisecond)
	lower := meanNanos - standardDeviationNanos
	upper := meanNanos + standardDeviationNanos
	var within uint64
	for index := range series.buckets {
		midpoint := math.Ldexp(1, index) * 1.5
		if midpoint >= lower && midpoint <= upper {
			within += series.buckets[index].Load()
		}
	}
	return distribution{
		min:                     float64(series.minNanos.Load()),
		max:                     float64(series.maxNanos.Load()),
		mean:                    meanNanos,
		standardDeviation:       standardDeviationNanos,
		withinStandardDeviation: float64(within) * 100 / float64(count),
	}
}

func numericDistribution(values []float64) distribution {
	if len(values) == 0 {
		return distribution{}
	}
	result := distribution{min: values[0], max: values[0]}
	for _, value := range values {
		result.mean += value
		result.min = min(result.min, value)
		result.max = max(result.max, value)
	}
	result.mean /= float64(len(values))
	for _, value := range values {
		difference := value - result.mean
		result.standardDeviation += difference * difference
	}
	result.standardDeviation = math.Sqrt(result.standardDeviation / float64(len(values)))
	for _, value := range values {
		if value >= result.mean-result.standardDeviation && value <= result.mean+result.standardDeviation {
			result.withinStandardDeviation++
		}
	}
	result.withinStandardDeviation = result.withinStandardDeviation * 100 / float64(len(values))
	return result
}

func printSummary(elapsed time.Duration, stats *metrics) {
	done := stats.done.Load()
	headers := stats.headers.Load()
	data := stats.data.Load()
	totalBytes := headers + data
	elapsedSeconds := elapsed.Seconds()

	fmt.Printf("finished in %.2fs, %.2f req/s, %s/s\n", elapsedSeconds, float64(done)/elapsedSeconds, formatBytes(float64(totalBytes)/elapsedSeconds))
	fmt.Printf("requests: %d total, %d started, %d done, %d succeeded, %d failed, %d errored, %d timeout\n",
		stats.started.Load(), stats.started.Load(), done, stats.succeeded.Load(), stats.failed.Load(), stats.errored.Load(), stats.timeout.Load())
	fmt.Printf("status codes: %d 2xx, %d 3xx, %d 4xx, %d 5xx\n",
		stats.status[1].Load(), stats.status[2].Load(), stats.status[3].Load(), stats.status[4].Load())
	fmt.Printf("traffic: %s (%d) total, %s (%d) headers (space savings 0.00%%), %s (%d) data\n",
		formatBytes(float64(totalBytes)), totalBytes, formatBytes(float64(headers)), headers, formatBytes(float64(data)), data)
	fmt.Println("                     min         max         mean           sd      +/- sd")
	printTiming("time for request:", stats.request.snapshot())
	printTiming("time for connect:", stats.connect.snapshot())
	printTiming("time to 1st byte:", stats.firstByte.snapshot())
	stats.ratesMu.Lock()
	rates := append([]float64(nil), stats.rates...)
	stats.ratesMu.Unlock()
	printRate("req/s           :", numericDistribution(rates))
}

func printTiming(label string, values distribution) {
	fmt.Printf("%-20s %11s %11s %11s %11s %9.2f%%\n", label,
		formatDuration(values.min), formatDuration(values.max), formatDuration(values.mean),
		formatDuration(values.standardDeviation), values.withinStandardDeviation)
}

func printRate(label string, values distribution) {
	fmt.Printf("%-20s %11.2f %11.2f %11.2f %11.2f %9.2f%%\n", label,
		values.min, values.max, values.mean, values.standardDeviation, values.withinStandardDeviation)
}

func formatDuration(nanos float64) string {
	if nanos == 0 {
		return "-"
	}
	duration := time.Duration(nanos)
	if duration >= time.Second {
		return fmt.Sprintf("%.2fs", duration.Seconds())
	}
	if duration >= time.Millisecond {
		return fmt.Sprintf("%.2fms", float64(duration)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.2fus", float64(duration)/float64(time.Microsecond))
}

func formatBytes(value float64) string {
	units := [...]string{"B", "KB", "MB", "GB", "TB"}
	unit := 0
	for value >= 1000 && unit < len(units)-1 {
		value /= 1000
		unit++
	}
	return fmt.Sprintf("%.2f%s", value, units[unit])
}

func responseHeaderBytes(response *http.Response) uint64 {
	bytes := len(response.Proto) + 1 + 3 + 1 + len(response.Status) + 2
	for name, values := range response.Header {
		for _, value := range values {
			bytes += len(name) + 2 + len(value) + 2
		}
	}
	return uint64(bytes + 2)
}

func warmConnections(endpoint string, slots []connectionSlot) error {
	context, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	jobs := make(chan int)
	errors := make(chan error, 1)
	workerCount := min(len(slots), max(1, runtime.GOMAXPROCS(0)*4))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				if err := executeRequestWithRetry(context, endpoint, &slots[index], 5); err != nil {
					select {
					case errors <- fmt.Errorf("connection %d: %w", index, err):
					default:
					}
					cancel()
					return
				}
			}
		}()
	}

sendJobs:
	for index := range slots {
		select {
		case jobs <- index:
		case <-context.Done():
			break sendJobs
		}
	}
	close(jobs)
	workers.Wait()
	select {
	case err := <-errors:
		return err
	default:
		return context.Err()
	}
}

func runWorker(context context.Context, endpoint string, slots []connectionSlot, offset, stride int, stats *metrics) {
	started := time.Now()
	completed := uint64(0)
	consecutiveTransientErrors := 0
	defer func() {
		elapsed := time.Since(started).Seconds()
		if elapsed > 0 {
			stats.ratesMu.Lock()
			stats.rates = append(stats.rates, float64(completed)/elapsed)
			stats.ratesMu.Unlock()
		}
	}()
	for sequence := offset; ; sequence += stride {
		if context.Err() != nil {
			return
		}
		stats.started.Add(1)
		result, err := executeRequest(context, endpoint, &slots[sequence%len(slots)])
		if context.Err() != nil && err != nil {
			return
		}
		completed++
		stats.record(result, err)
		if err != nil && context.Err() == nil {
			if isTransientRequestError(err) {
				consecutiveTransientErrors++
				if !waitForRetry(context, retryDelay(consecutiveTransientErrors)) {
					return
				}
			}
		} else {
			consecutiveTransientErrors = 0
		}
	}
}

func runBurstWorkers(context context.Context, endpoint string, slots []connectionSlot, inflight, minRequestsPerSecond, maxRequestsPerSecond int, stats *metrics) {
	jobs := make(chan int, maxRequestsPerSecond)
	var workers sync.WaitGroup
	workers.Add(inflight)
	for range inflight {
		go func() {
			defer workers.Done()
			runBurstWorker(context, endpoint, slots, jobs, stats)
		}()
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	sequence := 0
	dispatch := func() bool {
		count := minRequestsPerSecond + random.Intn(maxRequestsPerSecond-minRequestsPerSecond+1)
		for range count {
			select {
			case jobs <- sequence % len(slots):
				sequence++
			case <-context.Done():
				return false
			}
		}
		return true
	}

	for dispatch() {
		select {
		case <-context.Done():
			close(jobs)
			workers.Wait()
			return
		case <-ticker.C:
		}
	}
	close(jobs)
	workers.Wait()
}

func runBurstWorker(context context.Context, endpoint string, slots []connectionSlot, jobs <-chan int, stats *metrics) {
	started := time.Now()
	completed := uint64(0)
	defer func() {
		elapsed := time.Since(started).Seconds()
		if elapsed > 0 {
			stats.ratesMu.Lock()
			stats.rates = append(stats.rates, float64(completed)/elapsed)
			stats.ratesMu.Unlock()
		}
	}()
	for slotIndex := range jobs {
		if context.Err() != nil {
			return
		}
		stats.started.Add(1)
		result, err := executeRequest(context, endpoint, &slots[slotIndex])
		if context.Err() != nil && err != nil {
			return
		}
		completed++
		stats.record(result, err)
	}
}

func executeRequestWithRetry(context context.Context, endpoint string, slot *connectionSlot, attempts int) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		_, err = executeRequest(context, endpoint, slot)
		if err == nil || !isTransientRequestError(err) || attempt == attempts {
			return err
		}
		if !waitForRetry(context, retryDelay(attempt)) {
			return context.Err()
		}
	}
	return err
}

func isTransientRequestError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var statusError *httpStatusError
	if errors.As(err, &statusError) {
		return statusError.code == http.StatusRequestTimeout || statusError.code == http.StatusTooManyRequests || statusError.code >= 500
	}
	var networkError net.Error
	return errors.As(err, &networkError) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func retryDelay(attempt int) time.Duration {
	delay := 100 * time.Millisecond * time.Duration(1<<min(attempt-1, 4))
	return min(delay, 2*time.Second)
}

func waitForRetry(context context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-context.Done():
		return false
	case <-timer.C:
		return true
	}
}

func executeRequest(context context.Context, endpoint string, slot *connectionSlot) (requestResult, error) {
	var result requestResult
	started := time.Now()
	var connectStarted atomic.Int64
	var connectDuration atomic.Int64
	var firstByteDuration atomic.Int64
	trace := &httptrace.ClientTrace{
		ConnectStart: func(_, _ string) { connectStarted.Store(time.Now().UnixNano()) },
		ConnectDone: func(_, _ string, err error) {
			connectStart := connectStarted.Load()
			if err == nil && connectStart != 0 {
				connectDuration.Store(time.Now().UnixNano() - connectStart)
			}
		},
		GotFirstResponseByte: func() { firstByteDuration.Store(int64(time.Since(started))) },
	}
	context = httptrace.WithClientTrace(context, trace)
	request, err := http.NewRequestWithContext(context, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	request.Header.Set("User-Agent", "connection-churn-load-test/1.0")

	generation := slot.acquire()
	response, err := generation.transport.RoundTrip(request)
	result.connectDuration = time.Duration(connectDuration.Load())
	result.firstByteDuration = time.Duration(firstByteDuration.Load())
	if err != nil {
		generation.release()
		result.duration = time.Since(started)
		return result, err
	}
	written, copyErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	generation.release()
	result.statusCode = response.StatusCode
	result.headerBytes = responseHeaderBytes(response)
	result.dataBytes = uint64(written)
	result.duration = time.Since(started)
	if copyErr != nil {
		return result, copyErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return result, &httpStatusError{code: response.StatusCode, status: response.Status}
	}
	return result, nil
}

func rotateConnections(context context.Context, slots []connectionSlot, count int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	indices := make([]int, len(slots))
	for index := range indices {
		indices[index] = index
	}
	for {
		select {
		case <-context.Done():
			return
		case <-ticker.C:
			for index := range count {
				selected := index + random.Intn(len(indices)-index)
				indices[index], indices[selected] = indices[selected], indices[index]
				slots[indices[index]].rotate()
			}
		}
	}
}

func closeSlots(slots []connectionSlot) {
	for index := range slots {
		slots[index].mu.Lock()
		slots[index].generation.retire()
		slots[index].mu.Unlock()
	}
}

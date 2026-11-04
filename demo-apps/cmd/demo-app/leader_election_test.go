package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	fakecoordination "k8s.io/client-go/kubernetes/typed/coordination/v1/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestLeaderState(t *testing.T) {
	var state leaderState
	if state.isLeader() {
		t.Fatal("a new replica must not be a leader")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state.start(ctx)
	if !state.isLeader() {
		t.Fatal("active leadership context must be a leader")
	}
	canceled, cancelOld := context.WithCancel(context.Background())
	cancelOld()
	state.start(canceled)
	if !state.isLeader() {
		t.Fatal("a delayed, canceled leadership callback must not replace active leadership")
	}
	cancel()
	if state.isLeader() {
		t.Fatal("canceled leadership must stop serving as leader before the stop callback")
	}
	state.stop()
	if state.isLeader() {
		t.Fatal("stopped leadership must not be a leader")
	}
	state.start(canceled)
	if state.isLeader() {
		t.Fatal("a delayed callback must not restore canceled leadership")
	}
}

func configureRetryTest(t *testing.T, electionEnabled, closeNonLeader bool) {
	t.Helper()
	oldElection, oldClose, oldAction := leaderElectionEnabled, closeConnectionFromNonLeader, nonLeaderAction
	leaderElectionEnabled = electionEnabled
	closeConnectionFromNonLeader = closeNonLeader
	nonLeaderAction = nonLeaderActionNone
	podLeadership.stop()
	t.Cleanup(func() {
		leaderElectionEnabled, closeConnectionFromNonLeader, nonLeaderAction = oldElection, oldClose, oldAction
		podLeadership.stop()
	})
}

func TestKeepaliveReturnsOK(t *testing.T) {
	for _, tt := range []struct {
		name     string
		election bool
		close    bool
		leader   bool
	}{
		{name: "default off"},
		{name: "non-leader without fault injection", election: true},
		{name: "leader with fault injection", election: true, close: true, leader: true},
		{name: "leader without fault injection", election: true, leader: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configureRetryTest(t, tt.election, tt.close)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.leader {
				podLeadership.start(ctx)
			}
			recorder := httptest.NewRecorder()
			newRESTMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/keepalive", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", recorder.Code)
			}
			if body := recorder.Body.String(); body != "ok" {
				t.Fatalf("response = %q, want ok", body)
			}
		})
	}
}

type observedRetryConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *observedRetryConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.closed) })
	return err
}

func newRetryTestServer(t *testing.T, protocol string) (*httptest.Server, *http.Client, <-chan *observedRetryConn) {
	t.Helper()
	connections := make(chan *observedRetryConn, 128)
	server := httptest.NewUnstartedServer(newRESTMux())
	server.Config.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
		observed := &observedRetryConn{Conn: conn, closed: make(chan struct{})}
		connections <- observed
		return withRetryConnection(ctx, observed)
	}
	var transport http.RoundTripper
	switch protocol {
	case "http1":
		server.Start()
		tr := &http.Transport{}
		t.Cleanup(tr.CloseIdleConnections)
		transport = tr
	case "h2c":
		server.Config.Handler = h2c.NewHandler(newRESTMux(), &http2.Server{})
		server.Start()
		tr := &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		}
		t.Cleanup(tr.CloseIdleConnections)
		transport = tr
	case "https-http1", "https-http2":
		server.EnableHTTP2 = protocol == "https-http2"
		if server.EnableHTTP2 {
			if err := http2.ConfigureServer(server.Config, &http2.Server{}); err != nil {
				t.Fatal(err)
			}
		}
		server.StartTLS()
		transport = server.Client().Transport
	default:
		t.Fatalf("unsupported test protocol %q", protocol)
	}
	t.Cleanup(server.Close)
	return server, &http.Client{Transport: transport, Timeout: 3 * time.Second}, connections
}

func TestKeepaliveClosesNonLeaderConnections(t *testing.T) {
	for _, protocol := range []string{"http1", "h2c", "https-http1", "https-http2"} {
		t.Run(protocol, func(t *testing.T) {
			configureRetryTest(t, true, true)
			server, client, connections := newRetryTestServer(t, protocol)

			// Start with a leader response to verify the protocol and keep the same connection.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			podLeadership.start(ctx)
			response, err := client.Get(server.URL + "/keepalive")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantProtocol := 1
			if protocol == "h2c" || protocol == "https-http2" {
				wantProtocol = 2
			}
			if response.StatusCode != http.StatusOK || response.ProtoMajor != wantProtocol || string(body) != "ok" {
				t.Fatalf("leader response: status=%d protocol=%s body=%q", response.StatusCode, response.Proto, body)
			}
			conn := <-connections
			select {
			case <-conn.closed:
				t.Fatal("leader request must not close the connection")
			default:
			}

			// Canceling the term must immediately turn the same replica into a non-leader.
			cancel()
			response, err = client.Get(server.URL + "/keepalive")
			if response != nil {
				response.Body.Close()
				t.Fatalf("non-leader returned HTTP %d instead of dropping the connection", response.StatusCode)
			}
			if err == nil {
				t.Fatal("non-leader request must fail at the transport layer")
			}
			select {
			case <-conn.closed:
			case <-time.After(time.Second):
				t.Fatal("underlying connection was not closed (a stream reset alone is insufficient)")
			}

			response, err = client.Get(server.URL + "/liveness")
			if err != nil {
				t.Fatalf("non-leader liveness probe failed: %v", err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("non-leader liveness status = %d, want 200", response.StatusCode)
			}
			response, err = client.Get(server.URL + "/ready")
			if err != nil {
				t.Fatalf("non-leader readiness probe failed: %v", err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("non-leader readiness status = %d, want 200", response.StatusCode)
			}
		})
	}
}

func TestKeepaliveMissingConnectionFailsExplicitly(t *testing.T) {
	configureRetryTest(t, true, true)
	recorder := httptest.NewRecorder()
	keepaliveHandler(recorder, httptest.NewRequest(http.MethodGet, "/keepalive", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestValidateRetryOptions(t *testing.T) {
	for _, tt := range []struct {
		name     string
		election bool
		legacy   bool
		action   string
		wantErr  string
	}{
		{name: "default off", action: nonLeaderActionNone},
		{name: "no fault", election: true, action: nonLeaderActionNone},
		{name: "close", election: true, action: nonLeaderActionClose},
		{name: "503", election: true, action: nonLeaderAction503},
		{name: "mixed", election: true, action: nonLeaderActionMixed},
		{name: "skip binding", election: true, action: nonLeaderActionSkipPortBinding},
		{name: "legacy", election: true, legacy: true, action: nonLeaderActionNone},
		{name: "matching legacy", election: true, legacy: true, action: nonLeaderActionClose},
		{name: "invalid action", election: true, action: "typo", wantErr: "invalid --non-leader-action"},
		{name: "close without election", action: nonLeaderActionClose, wantErr: "requires --leader-election"},
		{name: "503 without election", action: nonLeaderAction503, wantErr: "requires --leader-election"},
		{name: "mixed without election", action: nonLeaderActionMixed, wantErr: "requires --leader-election"},
		{name: "skip binding without election", action: nonLeaderActionSkipPortBinding, wantErr: "requires --leader-election"},
		{name: "legacy without election", legacy: true, action: nonLeaderActionNone, wantErr: "requires --leader-election"},
		{name: "conflicting 503", election: true, legacy: true, action: nonLeaderAction503, wantErr: "conflicts"},
		{name: "conflicting mixed", election: true, legacy: true, action: nonLeaderActionMixed, wantErr: "conflicts"},
		{name: "conflicting skip binding", election: true, legacy: true, action: nonLeaderActionSkipPortBinding, wantErr: "conflicts"},
		{name: "mixed skip and close", election: true, action: "close/skip-port-binding", wantErr: "invalid --non-leader-action"},
		{name: "mixed skip and 503", election: true, action: "skip-port-binding/503", wantErr: "invalid --non-leader-action"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRetryOptions(tt.election, tt.legacy, tt.action)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateRetryOptions() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestChooseNonLeaderAction(t *testing.T) {
	for _, action := range []string{nonLeaderActionNone, nonLeaderActionClose, nonLeaderAction503, nonLeaderActionSkipPortBinding} {
		if got := chooseNonLeaderAction(action); got != action {
			t.Errorf("chooseNonLeaderAction(%q) = %q", action, got)
		}
	}
	const samples = 20000
	counts := make(map[string]int)
	for range samples {
		counts[chooseNonLeaderAction(nonLeaderActionMixed)]++
	}
	if counts[nonLeaderActionClose]+counts[nonLeaderAction503] != samples {
		t.Fatalf("mixed mode selected an unexpected action: %v", counts)
	}
	for action, count := range counts {
		if count < samples*45/100 || count > samples*55/100 {
			t.Fatalf("action %q selected %d/%d times, want approximately 50%%", action, count, samples)
		}
	}
}

func TestKeepaliveLeaderAlwaysSucceeds(t *testing.T) {
	for _, action := range []string{nonLeaderActionClose, nonLeaderAction503, nonLeaderActionMixed, nonLeaderActionSkipPortBinding} {
		t.Run(action, func(t *testing.T) {
			configureRetryTest(t, true, false)
			nonLeaderAction = action
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			podLeadership.start(ctx)
			for range 64 {
				recorder := httptest.NewRecorder()
				newRESTMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/keepalive", nil))
				if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
					t.Fatalf("leader response: status=%d body=%q", recorder.Code, recorder.Body.String())
				}
			}
		})
	}
}

func TestKeepaliveNonLeader503(t *testing.T) {
	configureRetryTest(t, true, false)
	nonLeaderAction = nonLeaderAction503
	recorder := httptest.NewRecorder()
	newRESTMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/keepalive", nil))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "leader=false leader-election=true") {
		t.Fatalf("non-leader response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestKeepaliveNonLeaderActions(t *testing.T) {
	for _, protocol := range []string{"http1", "h2c", "https-http1", "https-http2"} {
		for _, action := range []string{nonLeaderActionClose, nonLeaderAction503, nonLeaderActionMixed} {
			t.Run(protocol+"/"+action, func(t *testing.T) {
				configureRetryTest(t, true, false)
				nonLeaderAction = action
				server, client, connections := newRetryTestServer(t, protocol)
				closeCount, unavailableCount := 0, 0
				for range 64 {
					// Fresh connections prevent transparent client retries from hiding close outcomes.
					client.Transport.(interface{ CloseIdleConnections() }).CloseIdleConnections()
					response, err := client.Get(server.URL + "/keepalive")
					var conn *observedRetryConn
					select {
					case conn = <-connections:
					case <-time.After(time.Second):
						t.Fatal("request did not establish a connection")
					}
					if response == nil {
						if err == nil {
							t.Fatal("missing response without a connection error")
						}
						select {
						case <-conn.closed:
							closeCount++
						case <-time.After(time.Second):
							t.Fatalf("transport error without closing the underlying connection: %v", err)
						}
						continue
					}
					body, readErr := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || readErr != nil {
						t.Fatalf("response errors: request=%v read=%v", err, readErr)
					}
					wantProtocol := 1
					if protocol == "h2c" || protocol == "https-http2" {
						wantProtocol = 2
					}
					if response.StatusCode != http.StatusServiceUnavailable || response.ProtoMajor != wantProtocol || !strings.Contains(string(body), "leader=false leader-election=true") {
						t.Fatalf("non-leader response: status=%d protocol=%s body=%q", response.StatusCode, response.Proto, body)
					}
					unavailableCount++
				}
				if closeCount+unavailableCount != 64 {
					t.Fatalf("outcomes: close=%d 503=%d; non-leaders must never succeed", closeCount, unavailableCount)
				}
				switch action {
				case nonLeaderActionClose:
					if closeCount != 64 {
						t.Fatalf("close mode closed %d/64 connections, want 100%%", closeCount)
					}
				case nonLeaderAction503:
					if unavailableCount != 64 {
						t.Fatalf("503 mode returned 503 for %d/64 requests, want 100%%", unavailableCount)
					}
				case nonLeaderActionMixed:
					if closeCount == 0 || unavailableCount == 0 {
						t.Fatalf("mixed outcomes: close=%d 503=%d; want both failure types", closeCount, unavailableCount)
					}
				}
			})
		}
	}
}

func TestKeepaliveInvalidActionFailsExplicitly(t *testing.T) {
	configureRetryTest(t, true, false)
	nonLeaderAction = "typo"
	recorder := httptest.NewRecorder()
	keepaliveHandler(recorder, httptest.NewRequest(http.MethodGet, "/keepalive", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestHealthRoutes(t *testing.T) {
	mux := newRESTMux()
	for path, wantPattern := range map[string]string{
		"/keepalive":    "/keepalive",
		"/liveness":     "/liveness",
		"/should_retry": "/",
	} {
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, path, nil))
		if pattern != wantPattern {
			t.Errorf("route %s uses pattern %q, want %q", path, pattern, wantPattern)
		}
	}
	if _, exists := handlerCallCounters["shouldRetryHandler"]; exists {
		t.Fatal("removed retry handler must not have a call counter")
	}
}

func TestLivenessIgnoresNonLeaderActions(t *testing.T) {
	for _, action := range []string{nonLeaderActionNone, nonLeaderActionClose, nonLeaderAction503, nonLeaderActionMixed} {
		t.Run(action, func(t *testing.T) {
			configureRetryTest(t, true, false)
			nonLeaderAction = action
			mux := newRESTMux()
			for range 64 {
				recorder := httptest.NewRecorder()
				mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/liveness", nil))
				if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
					t.Fatalf("liveness must ignore %q: status=%d body=%q", action, recorder.Code, recorder.Body.String())
				}
			}
		})
	}
}

func TestLivenessFailuresAreIsolatedFromKeepalive(t *testing.T) {
	oldFail, oldAfter, oldStartup := failLivenessProbe, failProbesAfterSeconds, startupTime
	t.Cleanup(func() {
		failLivenessProbe, failProbesAfterSeconds, startupTime = oldFail, oldAfter, oldStartup
	})
	failLivenessProbe = true
	failProbesAfterSeconds = time.Second
	startupTime = time.Now().Add(-2 * time.Second)
	for _, tt := range []struct {
		name       string
		leader     bool
		wantStatus int
		wantBody   string
	}{
		{name: "non-leader", wantStatus: http.StatusServiceUnavailable, wantBody: "pod=" + hostname + " leader=false leader-election=true\n"},
		{name: "leader", leader: true, wantStatus: http.StatusOK, wantBody: "ok"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configureRetryTest(t, true, false)
			nonLeaderAction = nonLeaderAction503
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.leader {
				podLeadership.start(ctx)
			}
			mux := newRESTMux()
			liveness := httptest.NewRecorder()
			mux.ServeHTTP(liveness, httptest.NewRequest(http.MethodGet, "/liveness", nil))
			if liveness.Code != http.StatusInternalServerError || liveness.Body.String() != "failed\n" {
				t.Fatalf("explicit liveness failure: status=%d body=%q", liveness.Code, liveness.Body.String())
			}
			keepalive := httptest.NewRecorder()
			mux.ServeHTTP(keepalive, httptest.NewRequest(http.MethodGet, "/keepalive", nil))
			if keepalive.Code != tt.wantStatus || keepalive.Body.String() != tt.wantBody {
				t.Fatalf("keepalive must ignore liveness failure: status=%d body=%q", keepalive.Code, keepalive.Body.String())
			}
		})
	}
	failProbesAfterSeconds = time.Hour
	recorder := httptest.NewRecorder()
	newRESTMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/liveness", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("liveness must respect the failure delay: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHealthHandlersHaveSeparateCounters(t *testing.T) {
	configureRetryTest(t, true, false)
	nonLeaderAction = nonLeaderAction503
	oldSetting := logHandlerCallCounts
	keepaliveCounter, livenessCounter := handlerCallCounters["keepaliveHandler"], handlerCallCounters["livenessHandler"]
	if keepaliveCounter == nil || livenessCounter == nil {
		t.Fatal("keepalive and liveness must each have a call counter")
	}
	oldKeepalive, oldLiveness := keepaliveCounter.Load(), livenessCounter.Load()
	t.Cleanup(func() {
		logHandlerCallCounts = oldSetting
		keepaliveCounter.Store(oldKeepalive)
		livenessCounter.Store(oldLiveness)
	})
	logHandlerCallCounts = true
	keepaliveCounter.Store(0)
	livenessCounter.Store(0)
	mux := newRESTMux()
	for _, path := range []string{"/keepalive", "/liveness"} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if keepaliveCounter.Load() != 1 || livenessCounter.Load() != 1 {
		t.Fatalf("handler counts: keepalive=%d liveness=%d; want one call each", keepaliveCounter.Load(), livenessCounter.Load())
	}
}

func TestLeaderElectionFailover(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	tracker := clienttesting.NewObjectTracker(scheme, serializer.NewCodecFactory(scheme).UniversalDecoder())
	client := &fakecoordination.FakeCoordinationV1{Fake: &clienttesting.Fake{}}
	client.AddReactor("*", "*", clienttesting.ObjectReaction(tracker))

	var firstState, secondState leaderState
	first, err := newLeaderElector(client, "test-namespace", "test-lease", "first", &firstState)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newLeaderElector(client, "test-namespace", "test-lease", "second", &secondState)
	if err != nil {
		t.Fatal(err)
	}
	start := func(run func(context.Context)) (context.CancelFunc, <-chan struct{}) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			run(ctx)
		}()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("election did not stop after cancellation")
			}
		})
		return cancel, done
	}
	waitFor := func(description string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if condition() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", description)
	}
	cancelFirst, firstDone := start(func(ctx context.Context) { runLeaderElection(ctx, first) })
	waitFor("first replica to acquire leadership", firstState.isLeader)
	start(func(ctx context.Context) { runLeaderElection(ctx, second) })
	waitFor("second replica to observe the existing leader", func() bool { return second.GetLeader() == "first" })
	if secondState.isLeader() {
		t.Fatal("second replica must not lead while the first holds the lease")
	}
	cancelFirst()
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("first replica did not stop")
	}
	if firstState.isLeader() {
		t.Fatal("first replica remained leader after cancellation")
	}
	waitFor("second replica to acquire the released lease", secondState.isLeader)
}

func TestServeHTTPStopsOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &http.Server{Handler: newRESTMux(), ConnContext: withRetryConnection}
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, server, listener) }()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/keepalive")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveHTTP() = %v, want nil on shutdown", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not stop on cancellation")
	}
	_, err = net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("listener should be closed after shutdown, got %v", err)
	}
}

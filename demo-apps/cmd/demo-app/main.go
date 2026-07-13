package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "demo-app/pkg/grpcdemo"
)

// socketRcvBufBytes matches Envoy's wide transport boundary so the kernel
// receive buffer can absorb large bursts (e.g. 158KB chunks) without stalling.
const socketRcvBufBytes = 4 * 1024 * 1024

// optimizedListenConfig returns a net.ListenConfig whose Control hook forces
// SO_RCVBUF on the listening socket (inherited by accepted sockets on Linux).
// SO_REUSEADDR is also set to keep restarts fast.
func optimizedListenConfig() net.ListenConfig {
	return net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var sockErr error
			if err := c.Control(func(fd uintptr) {
				if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
					sockErr = fmt.Errorf("SO_REUSEADDR: %w", err)
					return
				}
				// SO_RCVBUFFORCE bypasses net.core.rmem_max if the process has CAP_NET_ADMIN.
				if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUFFORCE, socketRcvBufBytes); err != nil {
					// Fall back to SO_RCVBUF (capped by net.core.rmem_max).
					if err2 := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, socketRcvBufBytes); err2 != nil {
						sockErr = fmt.Errorf("SO_RCVBUF: %w", err2)
						return
					}
				}
				// Add parallel send buffer configuration
				if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUFFORCE, socketRcvBufBytes); err != nil {
					_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF, socketRcvBufBytes)
				}
			}); err != nil {
				return err
			}
			return sockErr
		},
	}
}

type noDelayListener struct{ net.Listener }

func (l noDelayListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		// Enable OS-level TCP keepalive so a genuinely dead peer is eventually
		// detected, while a live but idle connection is kept open indefinitely
		// (the server never closes it just for being idle).
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
	return c, nil
}

// listenOptimized binds addr with the tuned ListenConfig and wraps the result
// so accepted sockets get TCP_NODELAY.
func listenOptimized(addr string) (net.Listener, error) {
	lc := optimizedListenConfig()
	lis, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, err
	}
	fmt.Printf("listener tuned on %s: SO_RCVBUF=%d, TCP_NODELAY=on\n", addr, socketRcvBufBytes)
	return noDelayListener{Listener: lis}, nil
}

var httpPort string
var httpSecurePort string
var http1Only bool
var failLivenessProbe bool
var failReadinessProbe bool
var startupDelay = 0 * time.Second
var failProbesAfterSeconds time.Duration
var readTimeout = 120 * time.Second
var writeTimeout = 120 * time.Second
var idleTimeout = 120 * time.Second
var startupTime = time.Now()
var tlsCertPath = "/falcon-managed-secrets/ssl-cert.pem"
var debugHandlerLogs bool

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// This is a utility service used in controlled environments.
		return true
	},
}

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
)

// ANSI color codes used to make log output easier to scan in a terminal.
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorOrange = "\033[38;5;208m"
	colorWhite  = "\033[97m"
)

// hostname is resolved once at startup and prefixed (in green) to every log
// line so logs from multiple pods/hosts can be told apart when aggregated.
var hostname string

// neverIdleTimeout is an effectively-infinite idle timeout (~100 years). It is
// large enough to mean "never close idle connections" for the lifetime of the
// process, while staying well below the int64 overflow boundary so internal
// timer arithmetic (time.Now().Add(...)) stays safe.
const neverIdleTimeout = 100 * 365 * 24 * time.Hour

// handlerCallCounters holds one atomic counter per handler. The map is
// populated once at package init and only read afterwards, so concurrent
// increments touch distinct *atomic.Int64 values without any lock.
var handlerCallCounters = map[string]*atomic.Int64{
	"livenessHandler":       {},
	"readinessHandler":      {},
	"dynamicPayloadHandler": {},
	"syncHandler":           {},
	"websocketHandler":      {},
	"metricsHandler":        {},
}

// countHandlerCall atomically increments the call counter for the named handler.
func countHandlerCall(handler string) {
	if c, ok := handlerCallCounters[handler]; ok {
		c.Add(1)
	}
}

// startHandlerCounterReporter logs a snapshot of all handler call counts every
// 30 seconds. Counters are read atomically; handler names are sorted for a
// stable log line.
func startHandlerCounterReporter() {
	names := make([]string, 0, len(handlerCallCounters))
	for name := range handlerCallCounters {
		names = append(names, name)
	}
	sort.Strings(names)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			parts := make([]string, 0, len(names))
			for _, name := range names {
				parts = append(parts, fmt.Sprintf("%s=%d", name, handlerCallCounters[name].Load()))
			}
			logf("handler call counts: %s", strings.Join(parts, " "))
		}
	}()
}

// logf writes a colored log line in the format:
//
//	<green hostname> <orange timestamp> <white message>
//
// The standard logger's own flags are disabled at startup so it does not emit
// its own (uncolored) timestamp; the timestamp here is formatted manually so it
// can be colored independently.
func logf(format string, args ...interface{}) {
	ts := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	log.Printf("%s[%s]%s %s%s%s %s%s%s", colorGreen, hostname, colorReset, colorOrange, ts, colorReset, colorWhite, msg, colorReset)
}

// debugLog logs a message only when debugHandlerLogs is enabled. All
// per-request and per-connection logging goes through this helper so that
// logging is fully disabled by default for performance.
func debugLog(format string, args ...interface{}) {
	if !debugHandlerLogs {
		return
	}
	logf(format, args...)
}

func logHandlerStart(handler string, r *http.Request) {
	countHandlerCall(handler)
	debugLog("handler=%s method=%s path=%s remote=%s proto=%s", handler, r.Method, r.URL.Path, r.RemoteAddr, r.Proto)
}

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	logHandlerStart("livenessHandler", r)
	if failLivenessProbe {
		if time.Since(startupTime) > failProbesAfterSeconds {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
	}
	fmt.Fprintf(w, "ok")
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	logHandlerStart("readinessHandler", r)
	if failReadinessProbe {
		if time.Since(startupTime) > failProbesAfterSeconds {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
	}
	fmt.Fprintf(w, "ok")
}

func dynamicPayloadHandler(w http.ResponseWriter, r *http.Request) {
	logHandlerStart("dynamicPayloadHandler", r)
	query := r.URL.Query()
	sizeParam := query.Get("size")
	var size int
	if sizeParam == "" {
		size = 1024 // default 1KB
	} else {
		_, err := fmt.Sscanf(sizeParam, "%d", &size)
		if err != nil || size < 0 {
			http.Error(w, "invalid size parameter", http.StatusBadRequest)
			return
		}
	}
	payload := make([]byte, size)
	for i := 0; i < size; i++ {
		payload[i] = 'A' + byte(i%26)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(payload)
}

func syncHandler(w http.ResponseWriter, r *http.Request) {
	logHandlerStart("syncHandler", r)
	// Drain and discard the request payload so the connection can be reused.
	if r.Body != nil {
		n, err := io.Copy(io.Discard, r.Body)
		r.Body.Close()
		switch {
		case err != nil && r.Context().Err() != nil:
			// The client canceled the stream (HTTP/2 RST_STREAM CANCEL) or
			// dropped the connection mid-body. This is a normal client-side
			// abort, not a server fault, and there is no longer anyone to
			// respond to, so stop processing here instead of logging an error.
			debugLog("sync: client canceled request after %d bytes: %v", n, err)
			return
		case err != nil:
			debugLog("sync: error draining request body after %d bytes: %v", n, err)
		case n > 0:
			debugLog("sync: drained %d bytes of request body", n)
		}
	}

	query := r.URL.Query()
	delayParam := query.Get("delay")
	if delayParam == "" {
		fmt.Fprintf(w, "done\n")
		return
	}

	d, err := time.ParseDuration(delayParam)
	if err != nil {
		http.Error(w, "invalid delay parameter: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Sleep, but abort early if the client goes away so we do not hold the
	// handler goroutine for the full delay after a cancellation.
	debugLog("sync: sleeping for %v", d)
	select {
	case <-time.After(d):
		fmt.Fprintf(w, "slept %v\n", d)
	case <-r.Context().Done():
		debugLog("sync: client canceled during sleep after %v: %v", d, r.Context().Err())
	}
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	logHandlerStart("websocketHandler", r)
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		debugLog("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	debugLog("websocket connected: remote=%s", r.RemoteAddr)

	done := make(chan struct{})
	defer close(done)

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(wsWriteWait)); err != nil {
					debugLog("websocket ping write failed: %v", err)
					return
				}
			}
		}
	}()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				debugLog("websocket closed unexpectedly: %v", err)
			} else {
				debugLog("websocket stream closed: %v", err)
			}
			return
		}

		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}

		pong := append([]byte("Pong: "), msg...)
		if err := conn.WriteMessage(msgType, pong); err != nil {
			debugLog("websocket pong write failed: %v", err)
			return
		}
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	logHandlerStart("metricsHandler", r)
	// Read service account token from file
	tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		http.Error(w, "failed to read service account token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare the HTTP request to kubelet metrics endpoint
	req, err := http.NewRequest("GET", "https://localhost:10250"+r.URL.Path, nil)
	if err != nil {
		http.Error(w, "failed to create request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	debugLog("Forwarding request to kubelet metrics endpoint: %s", req.URL.String())
	// Add Authorization header with bearer token
	req.Header.Set("Authorization", "Bearer "+string(token))
	debugLog("Added Authorization header with bearer token %s", string(token))

	// Use a client that skips TLS verification (like curl --insecure)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "failed to call kubelet metrics endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy status code from kubelet response
	w.WriteHeader(resp.StatusCode)

	// Copy response headers from kubelet, except some hop-by-hop headers might be omitted as needed
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Stream the response body back to the caller
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		// Log error but no need to send error because headers/body are already sent
		debugLog("Error copying response body: %v", err)
	}
}

func runWithGrpcSecureServer() {
	cert, err := tls.LoadX509KeyPair(tlsCertPath, tlsCertPath)
	if err != nil {
		fmt.Printf("Failed to load TLS cert for gRPC: %v\n", err)
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}

	// gRPC server
	creds := credentials.NewTLS(tlsConfig)
	grpcServer := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterGrpcDemoServiceServer(grpcServer, pb.NewGrpcDemoServiceImpl())

	// REST mux
	mux := http.NewServeMux()
	mux.HandleFunc("/keepalive", livenessHandler)
	mux.HandleFunc("/ready", readinessHandler)
	mux.HandleFunc("/dynamic_payload", dynamicPayloadHandler)
	mux.HandleFunc("/sync", syncHandler)
	mux.HandleFunc("/ws", websocketHandler)
	mux.HandleFunc("/", metricsHandler)
	// Use a more robust check for gRPC
	multiplexHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for gRPC: HTTP/2 AND Content-Type application/grpc
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		// Fallback to REST for EVERYTHING else
		mux.ServeHTTP(w, r)
	})

	// h2s carries the HTTP/2 server options. IdleTimeout is set to an
	// effectively-infinite value so idle HTTP/2 connections are never closed by
	// the server. This is important because http2.ConfigureServer treats a zero
	// IdleTimeout as "fall back to ReadTimeout", which would otherwise close
	// idle h2 connections after ReadTimeout elapses.
	h2s := &http2.Server{
		IdleTimeout: neverIdleTimeout,
	}
	handlerWithH2C := h2c.NewHandler(multiplexHandler, h2s)
	serverLogger := log.New(os.Stdout, "[HTTP-SERVER] ", log.LstdFlags|log.Lshortfile)
	_ = serverLogger
	httpServer := &http.Server{
		Addr:      fmt.Sprintf("0.0.0.0:%s", httpSecurePort),
		Handler:   handlerWithH2C,
		TLSConfig: tlsConfig,
		// ErrorLog:     serverLogger,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		// IdleTimeout 0 means the server never closes a keep-alive connection
		// just because it has been idle. Combined with TCP keepalive on the
		// listener, connections stay open until the client closes them or the
		// peer is detected as dead.
		IdleTimeout: 0,
	}

	// Because we call Serve on a manual TLS listener (not ServeTLS), Go does not
	// auto-configure HTTP/2. Without this, a client that ALPN-negotiates "h2"
	// (standard HTTP/2 clients and gRPC-over-TLS) would have its connection
	// silently dropped. ConfigureServer wires TLSNextProto["h2"] so those
	// connections are served over HTTP/2. Clients that skip ALPN and send the
	// HTTP/2 preface with prior knowledge are still handled by the h2c wrapper.
	if err := http2.ConfigureServer(httpServer, h2s); err != nil {
		fmt.Printf("Failed to configure HTTP/2 on secure server: %v\n", err)
		return
	}

	lis, err := listenOptimized(fmt.Sprintf("0.0.0.0:%s", httpSecurePort))
	if err != nil {
		fmt.Printf("Failed to listen on secure gRPC port %s: %v\n", httpSecurePort, err)
		return
	}

	tlsListener := tls.NewListener(lis, tlsConfig)
	fmt.Printf("gRPC+REST (TLS) server listening on port %v...\n", httpSecurePort)
	if err := httpServer.Serve(tlsListener); err != nil {
		fmt.Printf("Error starting gRPC+REST (TLS) server: %v\n", err)
	}
}

func runWithGrpcPlainServer() {
	// gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterGrpcDemoServiceServer(grpcServer, pb.NewGrpcDemoServiceImpl())

	// REST mux
	mux := http.NewServeMux()
	mux.HandleFunc("/keepalive", livenessHandler)
	mux.HandleFunc("/ready", readinessHandler)
	mux.HandleFunc("/dynamic_payload", dynamicPayloadHandler)
	mux.HandleFunc("/sync", syncHandler)
	mux.HandleFunc("/ws", websocketHandler)
	mux.HandleFunc("/", metricsHandler)

	// Multiplexed handler: route gRPC by Content-Type, everything else to REST
	multiplexHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			mux.ServeHTTP(w, r)
		}
	})

	serverAddr := fmt.Sprintf("0.0.0.0:%s", httpPort)
	httpServer := &http.Server{Addr: serverAddr}

	if http1Only {
		httpServer.Handler = mux
		fmt.Printf("REST (HTTP/1.1 only) server listening on port %v...\n", httpPort)
	} else {
		h2s := &http2.Server{}
		httpServer.Handler = h2c.NewHandler(multiplexHandler, h2s)
		fmt.Printf("gRPC+REST (H2C) server listening on port %v...\n", httpPort)
	}

	lis, err := listenOptimized(serverAddr)
	if err != nil {
		fmt.Printf("Failed to listen on plain port %s: %v\n", httpPort, err)
		return
	}
	if err := httpServer.Serve(lis); err != nil {
		fmt.Printf("Error starting plain server: %v\n", err)
	}
}

func main() {
	runtime.GOMAXPROCS(2)
	// Resolve the hostname once and disable the standard logger's own flags so
	// it does not emit an uncolored timestamp; logf() composes the colored line
	// (green hostname, orange timestamp, white message) itself.
	hostname, _ = os.Hostname()
	log.SetFlags(0)
	// Define flags (options for the command-line arguments)
	flag.StringVar(&httpPort, "http-port", "12012", "Server port")
	flag.BoolVar(&http1Only, "http1-only", false, "disable HTTP/2 (H2C) on plain server binding")
	flag.StringVar(&httpSecurePort, "https-port", "", "Secure server port")
	flag.BoolVar(&failLivenessProbe, "fail-liveness-probe", false, "should fail liveness probe")
	flag.BoolVar(&failReadinessProbe, "fail-readiness-probe", false, "should fail readiness probe")
	flag.DurationVar(&failProbesAfterSeconds, "fail-probes-after-seconds", 0, "after how many seconds the probes should start failing")
	flag.DurationVar(&startupDelay, "startup-delay", 0*time.Second, "startup delay")
	flag.DurationVar(&readTimeout, "read-timeout", 10*time.Second, "HTTP server read timeout")
	flag.DurationVar(&writeTimeout, "write-timeout", 10*time.Second, "HTTP server write timeout")
	flag.DurationVar(&idleTimeout, "idle-timeout", 120*time.Second, "HTTP server idle timeout")
	flag.BoolVar(&debugHandlerLogs, "debug-handler-logs", false, "enable debug logs at handler entry")

	flag.Parse()

	if startupDelay > 0 {
		fmt.Printf("initial delay: %v\n", startupDelay)
		time.Sleep(startupDelay)
		fmt.Println("initial delay completed")
	}

	fmt.Printf("settings: http-port: %v, http1-only: %v, fail-liveness-probe: %v, fail-readiness-probe: %v\n", httpPort, http1Only, failLivenessProbe, failReadinessProbe)
	startHandlerCounterReporter()
	if httpSecurePort != "" {
		go runWithGrpcSecureServer()
	}
	runWithGrpcPlainServer()
}

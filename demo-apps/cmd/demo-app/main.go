package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "demo-app/pkg/grpcdemo"
)

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

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	if failLivenessProbe {
		if time.Since(startupTime) > failProbesAfterSeconds {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
	}
	fmt.Fprintf(w, "ok")
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if failReadinessProbe {
		if time.Since(startupTime) > failProbesAfterSeconds {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
	}
	fmt.Fprintf(w, "ok")
}

func dynamicPayloadHandler(w http.ResponseWriter, r *http.Request) {
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
	query := r.URL.Query()
	delayParam := query.Get("delay")
	if delayParam != "" {
		d, err := time.ParseDuration(delayParam)
		if err != nil {
			http.Error(w, "invalid delay parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Printf("sync: sleeping for %v\n", d)
		time.Sleep(d)
		fmt.Fprintf(w, "slept for %v\n", d)
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("Forwarding request to kubelet metrics endpoint:", req.URL.String())
	// Add Authorization header with bearer token
	req.Header.Set("Authorization", "Bearer "+string(token))
	fmt.Println("Added Authorization header with bearer token", string(token))

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
		fmt.Println("Error copying response body:", err)
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

	h2s := &http2.Server{}
	handlerWithH2C := h2c.NewHandler(multiplexHandler, h2s)
	serverLogger := log.New(os.Stdout, "[HTTP-SERVER] ", log.LstdFlags|log.Lshortfile)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%s", httpSecurePort),
		Handler:      handlerWithH2C,
		TLSConfig:    tlsConfig,
		ErrorLog:     serverLogger,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", httpSecurePort))
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

	if err := httpServer.ListenAndServe(); err != nil {
		fmt.Printf("Error starting plain server: %v\n", err)
	}
}

func main() {
	runtime.GOMAXPROCS(2)
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

	flag.Parse()

	if startupDelay > 0 {
		fmt.Printf("initial delay: %v\n", startupDelay)
		time.Sleep(startupDelay)
		fmt.Println("initial delay completed")
	}

	fmt.Printf("settings: http-port: %v, http1-only: %v, fail-liveness-probe: %v, fail-readiness-probe: %v\n", httpPort, http1Only, failLivenessProbe, failReadinessProbe)
	if httpSecurePort != "" {
		go runWithGrpcSecureServer()
	}
	runWithGrpcPlainServer()
}

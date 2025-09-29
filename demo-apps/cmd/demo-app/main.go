package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var httpPort string
var failLivenessProbe bool
var failReadinessProbe bool
var startupDelay int
var failProbesAfterSeconds time.Duration
var startupTime = time.Now()

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

func main() {
	// Define flags (options for the command-line arguments)
	flag.StringVar(&httpPort, "http-port", "12012", "Server port")
	flag.BoolVar(&failLivenessProbe, "fail-liveness-probe", false, "should fail liveness probe")
	flag.BoolVar(&failReadinessProbe, "fail-readiness-probe", false, "should fail readiness probe")
	flag.DurationVar(&failProbesAfterSeconds, "fail-probes-after-seconds", 0, "after how many seconds the probes should start failing")
	flag.IntVar(&startupDelay, "startup-delay", 0, "startup delay")

	flag.Parse()

	if startupDelay > 0 {
		fmt.Printf("initial delay: %v\n", startupDelay)
		time.Sleep(time.Duration(startupDelay) * time.Second)
		fmt.Println("initial delay completed")
	}

	fmt.Printf("settings: http-port: %v, fail-liveness-probe: %v, fail-readiness-probe: %v\n", httpPort, failLivenessProbe, failReadinessProbe)
	http.HandleFunc("/keepalive", livenessHandler)
	http.HandleFunc("/ready", readinessHandler)
	http.HandleFunc("/", metricsHandler)

	fmt.Printf("Server is listening on port %v...\n", httpPort)
	err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", httpPort), nil)
	if err != nil {
		fmt.Println("Error starting the server:", err)
	}
}

package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"
)

var httpPort string
var failLivenessProbe bool
var failReadinessProbe bool
var startupDelay int

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	if failLivenessProbe {
		http.Error(w, "failed", http.StatusInternalServerError)
	}

	fmt.Fprintf(w, "ok")
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if failReadinessProbe {
		http.Error(w, "failed", http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "ok")
}

func main() {
	// Define flags (options for the command-line arguments)

	// Register flags with their respective default values and description
	flag.StringVar(&httpPort, "http-port", "12012", "Server port")
	flag.BoolVar(&failLivenessProbe, "fail-liveness-probe", false, "should fail liveness probe")
	flag.BoolVar(&failReadinessProbe, "fail-readiness-probe", false, "should fail readiness probe")
	flag.IntVar(&startupDelay, "startup-delay", 0, "startup delay")

	// Parse the flags from the command-line arguments
	flag.Parse()

	if startupDelay > 0 {
		fmt.Printf("initial delay: %v\n", startupDelay)
		time.Sleep(time.Duration(startupDelay) * time.Second)
		fmt.Println("initial delay completed")
	}

	fmt.Printf("settings: http-port: %v, fail-liveness-probe: %v, fail-readiness-probe: %v\n", httpPort, failLivenessProbe, failReadinessProbe)
	http.HandleFunc("/keepalive", livenessHandler)
	http.HandleFunc("/ready", readinessHandler)

	fmt.Printf("Server is listening on port %v...\n", httpPort)
	err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", httpPort), nil)
	if err != nil {
		fmt.Println("Error starting the server:", err)
	}
}

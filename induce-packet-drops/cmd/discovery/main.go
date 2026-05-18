// service-discovery: a minimal HTTP server that mimics the prod
// "service discovery" component. It returns a static list of fake
// services. Latency and error rate are intentionally low - all
// instability in the experiment is meant to come from the network
// (tc/netem), not from the server itself.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

type service struct {
	Name      string   `json:"name"`
	Cluster   string   `json:"cluster"`
	Endpoints []string `json:"endpoints"`
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	host, _ := os.Hostname()
	var requests uint64

	services := []service{
		{Name: "billing", Cluster: "c1", Endpoints: []string{"10.0.0.1:9000", "10.0.0.2:9000"}},
		{Name: "auth", Cluster: "c1", Endpoints: []string{"10.0.0.3:9000"}},
		{Name: "catalog", Cluster: "c2", Endpoints: []string{"10.0.1.1:9000", "10.0.1.2:9000"}},
		{Name: "search", Cluster: "c2", Endpoints: []string{"10.0.1.3:9000"}},
		{Name: "orders", Cluster: "c3", Endpoints: []string{"10.0.2.1:9000"}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/services", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddUint64(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Server", host)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"server":   host,
			"req":      n,
			"ts":       time.Now().UTC().Format(time.RFC3339Nano),
			"services": services,
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 0,
	}
	log.Printf("service-discovery listening on %s (host=%s)", *addr, host)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

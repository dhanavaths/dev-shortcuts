package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type leaderState struct {
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	changed chan struct{}
}

func (s *leaderState) start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() == nil {
		if s.cancel != nil {
			s.cancel()
		}
		s.ctx, s.cancel = context.WithCancel(ctx)
		if s.changed != nil {
			close(s.changed)
			s.changed = nil
		}
	}
}

func (s *leaderState) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.ctx = nil
}

func (s *leaderState) isLeader() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx != nil && s.ctx.Err() == nil
}

func (s *leaderState) waitForLeadership(ctx context.Context) (context.Context, error) {
	for {
		s.mu.Lock()
		if err := ctx.Err(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if s.ctx != nil && s.ctx.Err() == nil {
			term := s.ctx
			s.mu.Unlock()
			return term, nil
		}
		if s.changed == nil {
			s.changed = make(chan struct{})
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

const (
	nonLeaderActionNone            = "none"
	nonLeaderActionClose           = "close"
	nonLeaderAction503             = "503"
	nonLeaderActionMixed           = "close/503"
	nonLeaderActionSkipPortBinding = "skip-port-binding"
)

var (
	leaderElectionEnabled        bool
	leaderElectionNamespace      string
	leaderElectionLeaseName      string
	closeConnectionFromNonLeader bool
	nonLeaderAction              = nonLeaderActionNone
	podLeadership                leaderState
)

func validateRetryOptions(electionEnabled, closeNonLeader bool, action string) error {
	switch action {
	case nonLeaderActionNone, nonLeaderActionClose, nonLeaderAction503, nonLeaderActionMixed, nonLeaderActionSkipPortBinding:
	default:
		return fmt.Errorf("invalid --non-leader-action %q: must be none, close, 503, close/503, or skip-port-binding", action)
	}
	if closeNonLeader && !electionEnabled {
		return fmt.Errorf("--close-connection-from-non-leader requires --leader-election")
	}
	if action != nonLeaderActionNone && !electionEnabled {
		return fmt.Errorf("--non-leader-action requires --leader-election")
	}
	if closeNonLeader && action != nonLeaderActionNone && action != nonLeaderActionClose {
		return fmt.Errorf("--close-connection-from-non-leader conflicts with --non-leader-action=%s", action)
	}
	return nil
}

func chooseNonLeaderAction(action string) string {
	if action == nonLeaderActionMixed {
		if rand.IntN(2) == 0 {
			return nonLeaderActionClose
		}
		return nonLeaderAction503
	}
	return action
}

func newLeaderElector(client coordinationv1.CoordinationV1Interface, namespace, leaseName, identity string, state *leaderState) (*leaderelection.LeaderElector, error) {
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{Name: leaseName, Namespace: namespace},
		Client:    client,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}
	return leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		ReleaseOnCancel: true,
		Name:            leaseName,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				state.start(ctx)
				if state.isLeader() {
					logf("leader election: acquired lease %s/%s as %s", namespace, leaseName, identity)
				}
			},
			OnStoppedLeading: func() {
				state.stop()
				logf("leader election: stopped leading lease %s/%s", namespace, leaseName)
			},
			OnNewLeader: func(leader string) {
				logf("leader election: lease %s/%s holder=%s", namespace, leaseName, leader)
			},
		},
	})
}

func newInClusterLeaderElector(state *leaderState) (*leaderelection.LeaderElector, error) {
	namespace := leaderElectionNamespace
	if namespace == "" {
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return nil, fmt.Errorf("read leader election namespace: %w", err)
		}
		namespace = strings.TrimSpace(string(data))
	}
	if namespace == "" || leaderElectionLeaseName == "" {
		return nil, fmt.Errorf("leader election namespace and lease name must not be empty")
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster leader election configuration: %w", err)
	}
	client, err := coordinationv1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create leader election client: %w", err)
	}
	identity := os.Getenv("POD__ID")
	if identity == "" {
		identity = hostname
	}
	identity += "_" + string(uuid.NewUUID())
	return newLeaderElector(client, namespace, leaderElectionLeaseName, identity, state)
}

func runLeaderElection(ctx context.Context, elector *leaderelection.LeaderElector) {
	for ctx.Err() == nil {
		elector.Run(ctx)
		if ctx.Err() == nil {
			logf("leader election: lease lost; rejoining election")
		}
	}
}

type retryConnectionContextKey struct{}

func withRetryConnection(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, retryConnectionContextKey{}, conn)
}

func requestNonLeaderAction() string {
	if !leaderElectionEnabled || podLeadership.isLeader() {
		return nonLeaderActionNone
	}
	action := nonLeaderAction
	if closeConnectionFromNonLeader {
		action = nonLeaderActionClose
	}
	return chooseNonLeaderAction(action)
}

func handleNonLeaderFault(w http.ResponseWriter, r *http.Request, action string) bool {
	switch action {
	case nonLeaderActionNone:
		return false
	case nonLeaderAction503:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "pod=%s leader=false leader-election=true\n", hostname)
	case nonLeaderActionClose, nonLeaderActionSkipPortBinding:
		conn, ok := r.Context().Value(retryConnectionContextKey{}).(net.Conn)
		if !ok {
			logf("keepalive: request connection missing from context")
			http.Error(w, "request connection unavailable", http.StatusInternalServerError)
			return true
		}
		debugLog("keepalive: closing non-leader connection remote=%s proto=%s", r.RemoteAddr, r.Proto)
		if err := conn.Close(); err != nil {
			logf("keepalive: close connection: %v", err)
		}
		// Abort without writing headers; HTTP/2 must lose the connection, not just a stream.
		panic(http.ErrAbortHandler)
	default:
		logf("keepalive: invalid non-leader action %q", action)
		http.Error(w, "invalid non-leader action", http.StatusInternalServerError)
	}
	return true
}

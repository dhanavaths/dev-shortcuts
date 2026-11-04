package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

func runApp(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if leaderElectionEnabled {
		elector, err := newInClusterLeaderElector(&podLeadership)
		if err != nil {
			return fmt.Errorf("leader election: %w", err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			runLeaderElection(ctx, elector)
		}()
		defer func() {
			cancel()
			<-done
		}()
	}
	return runServers(ctx)
}

func runServers(ctx context.Context) error {
	if nonLeaderAction == nonLeaderActionSkipPortBinding {
		return serveWhileLeader(ctx, &podLeadership, runBoundServers)
	}
	return runBoundServers(ctx)
}

func runBoundServers(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if ctx.Err() != nil {
		return nil
	}
	results := make(chan error, 2)
	count := 1
	go func() { results <- runWithGrpcPlainServer(ctx) }()
	if httpSecurePort != "" {
		count++
		go func() { results <- runWithGrpcSecureServer(ctx) }()
	}
	var result error
	for range count {
		if err := <-results; err != nil && result == nil {
			result = err
		}
		cancel()
	}
	return result
}

func serveWhileLeader(ctx context.Context, state *leaderState, serve func(context.Context) error) error {
	for ctx.Err() == nil {
		logf("skip-port-binding: waiting for leadership; server ports are unbound")
		term, err := state.waitForLeadership(ctx)
		if err != nil {
			return nil
		}
		serverCtx, cancel := context.WithCancel(term)
		stop := context.AfterFunc(ctx, cancel)
		err = serve(serverCtx)
		stop()
		cancel()
		if err != nil {
			return err
		}
		if ctx.Err() == nil && term.Err() == nil {
			return fmt.Errorf("servers stopped while leadership was still active")
		}
	}
	return nil
}

// Track connections below TLS so shutdown also closes hijacked H2C and WebSocket connections.
type connectionListener struct {
	net.Listener
	mu        sync.Mutex
	conns     map[*serverConnection]struct{}
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func newConnectionListener(listener net.Listener) *connectionListener {
	return &connectionListener{Listener: listener, conns: make(map[*serverConnection]struct{})}
}

func (l *connectionListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, errors.Join(net.ErrClosed, conn.Close())
	}
	tracked := &serverConnection{Conn: conn, listener: l}
	l.conns[tracked] = struct{}{}
	return tracked, nil
}

func (l *connectionListener) Close() error {
	l.closeOnce.Do(func() {
		if err := l.Listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			l.closeErr = err
		}
		l.mu.Lock()
		l.closed = true
		connections := make([]*serverConnection, 0, len(l.conns))
		for conn := range l.conns {
			connections = append(connections, conn)
		}
		l.mu.Unlock()
		for _, conn := range connections {
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				l.closeErr = errors.Join(l.closeErr, err)
			}
		}
	})
	return l.closeErr
}

type serverConnection struct {
	net.Conn
	listener *connectionListener
}

func (c *serverConnection) Close() error {
	err := c.Conn.Close()
	c.listener.mu.Lock()
	delete(c.listener.conns, c)
	c.listener.mu.Unlock()
	return err
}

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

type trackedKeepaliveBody struct {
	io.Reader
	read      int
	readCalls int
	closed    bool
	closeErr  error
}

func (b *trackedKeepaliveBody) Read(p []byte) (int, error) {
	b.readCalls++
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *trackedKeepaliveBody) Close() error {
	b.closed = true
	return b.closeErr
}

type checkedKeepaliveWriter struct {
	*httptest.ResponseRecorder
	beforeWrite func()
}

func (w *checkedKeepaliveWriter) WriteHeader(status int) {
	w.beforeWrite()
	w.ResponseRecorder.WriteHeader(status)
}

func (w *checkedKeepaliveWriter) Write(p []byte) (int, error) {
	w.beforeWrite()
	return w.ResponseRecorder.Write(p)
}

type checkedKeepaliveConn struct {
	net.Conn
	beforeClose func()
}

func (c *checkedKeepaliveConn) Close() error {
	c.beforeClose()
	return c.Conn.Close()
}

func TestKeepaliveBodyHandlingBeforeResponseOrFault(t *testing.T) {
	payload := strings.Repeat("x", 1024*1024)
	for _, tt := range []struct {
		name     string
		election bool
		leader   bool
		legacy   bool
		action   string
		outcomes []int
	}{
		{name: "default", action: nonLeaderActionNone, outcomes: []int{200}},
		{name: "leader", election: true, leader: true, action: nonLeaderActionMixed, outcomes: []int{200}},
		{name: "none", election: true, action: nonLeaderActionNone, outcomes: []int{200}},
		{name: "close", election: true, action: nonLeaderActionClose, outcomes: []int{0}},
		{name: "503", election: true, action: nonLeaderAction503, outcomes: []int{503}},
		{name: "mixed", election: true, action: nonLeaderActionMixed, outcomes: []int{0, 503}},
		{name: "legacy", election: true, legacy: true, action: nonLeaderActionNone, outcomes: []int{0}},
		{name: "stale request after losing binding", election: true, action: nonLeaderActionSkipPortBinding, outcomes: []int{0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configureRetryTest(t, tt.election, tt.legacy)
			nonLeaderAction = tt.action
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.leader {
				podLeadership.start(ctx)
			}
			body := &trackedKeepaliveBody{Reader: strings.NewReader(payload)}
			checkDrained := func() {
				t.Helper()
				if body.read != len(payload) || !body.closed {
					t.Errorf("action ran before body was drained and closed: read=%d/%d closed=%t", body.read, len(payload), body.closed)
				}
			}
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()
			defer clientConn.Close()
			closed := false
			conn := &checkedKeepaliveConn{Conn: serverConn, beforeClose: func() {
				if body.readCalls != 0 || body.closed {
					t.Errorf("close action touched the body: reads=%d closed=%t", body.readCalls, body.closed)
				}
				closed = true
			}}
			request := httptest.NewRequest(http.MethodPost, "/keepalive", strings.NewReader(payload))
			request.Body = body
			request = request.WithContext(withRetryConnection(request.Context(), conn))
			writer := &checkedKeepaliveWriter{ResponseRecorder: httptest.NewRecorder(), beforeWrite: checkDrained}
			var panicValue any
			func() {
				defer func() { panicValue = recover() }()
				newRESTMux().ServeHTTP(writer, request)
			}()
			outcome := writer.Code
			if closed {
				outcome = 0
				if panicValue != http.ErrAbortHandler || writer.Body.Len() != 0 {
					t.Fatalf("close action wrote a response or raised wrong panic: body=%q panic=%v", writer.Body.String(), panicValue)
				}
			} else {
				checkDrained()
				if panicValue != nil {
					t.Fatalf("unexpected panic: %v", panicValue)
				}
			}
			for _, allowed := range tt.outcomes {
				if outcome == allowed {
					return
				}
			}
			t.Fatalf("outcome = %d, want one of %v (0 means connection closed)", outcome, tt.outcomes)
		})
	}
}

func TestKeepaliveBodyErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		readErr  error
		closeErr error
		canceled bool
	}{
		{name: "read error", readErr: errors.New("body read failed")},
		{name: "close error", closeErr: errors.New("body close failed")},
		{name: "client cancellation", readErr: context.Canceled, canceled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configureRetryTest(t, true, false)
			nonLeaderAction = nonLeaderAction503
			var reader io.Reader = strings.NewReader("partial")
			if tt.readErr != nil {
				reader = io.MultiReader(reader, iotest.ErrReader(tt.readErr))
			}
			body := &trackedKeepaliveBody{Reader: reader, closeErr: tt.closeErr}
			request := httptest.NewRequest(http.MethodPost, "/keepalive", nil)
			request.Body = body
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			if tt.canceled {
				cancel()
			}
			request = request.WithContext(ctx)
			wrote := false
			writer := &checkedKeepaliveWriter{ResponseRecorder: httptest.NewRecorder(), beforeWrite: func() {
				wrote = true
				if !body.closed {
					t.Error("response written before closing the body")
				}
			}}
			newRESTMux().ServeHTTP(writer, request)
			if !body.closed {
				t.Fatal("body was not closed after a drain failure")
			}
			if tt.canceled {
				if wrote {
					t.Fatal("canceled request must not receive a response")
				}
			} else if writer.Code != http.StatusBadRequest || !strings.Contains(writer.Body.String(), "failed to read request body") {
				t.Fatalf("body error response: status=%d body=%q", writer.Code, writer.Body.String())
			}
		})
	}
}

func TestKeepaliveWithoutBody(t *testing.T) {
	configureRetryTest(t, false, false)
	for _, body := range []io.ReadCloser{nil, http.NoBody} {
		request := httptest.NewRequest(http.MethodGet, "/keepalive", nil)
		request.Body = body
		writer := httptest.NewRecorder()
		newRESTMux().ServeHTTP(writer, request)
		if writer.Code != http.StatusOK || writer.Body.String() != "ok" {
			t.Fatalf("no-body response: status=%d body=%q", writer.Code, writer.Body.String())
		}
	}
}

func TestKeepaliveClosesBeforeUploadCompletes(t *testing.T) {
	for _, protocol := range []string{"http1", "h2c", "https-http1", "https-http2"} {
		t.Run(protocol, func(t *testing.T) {
			configureRetryTest(t, true, false)
			nonLeaderAction = nonLeaderActionClose
			server, client, connections := newRetryTestServer(t, protocol)
			body, upload := io.Pipe()
			defer body.Close()
			defer upload.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/keepalive", body)
			if err != nil {
				t.Fatal(err)
			}
			type result struct {
				response *http.Response
				err      error
			}
			results := make(chan result, 1)
			go func() {
				response, err := client.Do(request)
				if response != nil {
					response.Body.Close()
				}
				results <- result{response: response, err: err}
			}()
			writeDone := make(chan struct{})
			go func() {
				defer close(writeDone)
				_, _ = upload.Write([]byte("incomplete upload"))
			}()
			var conn *observedRetryConn
			select {
			case conn = <-connections:
			case <-time.After(time.Second):
				t.Fatal("request did not establish a connection")
			}
			select {
			case <-conn.closed:
			case <-time.After(time.Second):
				t.Fatal("server waited for the upload to finish before closing the connection")
			}
			upload.Close()
			select {
			case outcome := <-results:
				if outcome.response != nil || outcome.err == nil {
					t.Fatalf("close action returned a response: response=%v error=%v", outcome.response, outcome.err)
				}
			case <-time.After(time.Second):
				t.Fatal("client did not observe the closed connection")
			}
			select {
			case <-writeDone:
			case <-time.After(time.Second):
				t.Fatal("upload did not stop after the connection closed")
			}
		})
	}
}

func TestKeepaliveLargeBodyConnectionReuse(t *testing.T) {
	payload := strings.Repeat("x", 1024*1024)
	for _, protocol := range []string{"http1", "h2c", "https-http1", "https-http2"} {
		for _, leader := range []bool{false, true} {
			name := protocol + "/non-leader"
			if leader {
				name = protocol + "/leader"
			}
			t.Run(name, func(t *testing.T) {
				configureRetryTest(t, true, false)
				nonLeaderAction = nonLeaderAction503
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				wantStatus := http.StatusServiceUnavailable
				if leader {
					podLeadership.start(ctx)
					wantStatus = http.StatusOK
				}
				server, client, _ := newRetryTestServer(t, protocol)
				var firstConn net.Conn
				for i, data := range []string{payload, "next request"} {
					var connection httptrace.GotConnInfo
					trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { connection = info }}
					request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, server.URL+"/keepalive", strings.NewReader(data))
					if err != nil {
						t.Fatal(err)
					}
					response, err := client.Do(request)
					if err != nil {
						t.Fatal(err)
					}
					_, err = io.Copy(io.Discard, response.Body)
					response.Body.Close()
					if err != nil || response.StatusCode != wantStatus {
						t.Fatalf("response: status=%d error=%v", response.StatusCode, err)
					}
					if i == 0 {
						firstConn = connection.Conn
					} else if !connection.Reused || connection.Conn != firstConn {
						t.Fatal("connection was not reused after draining the 1 MiB request body")
					}
				}
			})
		}
	}
}

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

func testServerPorts(t *testing.T) (string, string) {
	t.Helper()
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	_, firstPort, err := net.SplitHostPort(first.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, secondPort, err := net.SplitHostPort(second.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return firstPort, secondPort
}

func testServerCertificate(t *testing.T) (string, *tls.Config) {
	t.Helper()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cert := server.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})...)
	path := filepath.Join(t.TempDir(), "server.pem")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path, server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
}

func waitForRefusedPorts(t *testing.T, ports ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		refused := true
		for _, port := range ports {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
			if conn != nil {
				conn.Close()
			}
			if !errors.Is(err, syscall.ECONNREFUSED) {
				if err != nil && !errors.Is(err, syscall.ECONNRESET) {
					t.Fatalf("port %s: expected connection refused, got %v", port, err)
				}
				refused = false
			}
		}
		if refused {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("server ports remained bound after leadership loss")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForBoundPorts(t *testing.T, ports ...string) {
	t.Helper()
	for _, port := range ports {
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("server port %s did not bind: %v", port, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestServerLeadershipLifecycle(t *testing.T) {
	for _, protocol := range []string{"http1", "h2c", "https-http1", "https-http2"} {
		for _, action := range []string{nonLeaderAction503, nonLeaderActionSkipPortBinding} {
			t.Run(protocol+"/"+action, func(t *testing.T) {
				configureRetryTest(t, true, false)
				nonLeaderAction = action
				oldPort, oldSecure, oldHTTP1, oldCert := httpPort, httpSecurePort, http1Only, tlsCertPath
				t.Cleanup(func() {
					httpPort, httpSecurePort, http1Only, tlsCertPath = oldPort, oldSecure, oldHTTP1, oldCert
				})
				httpPort, httpSecurePort = testServerPorts(t)
				http1Only = protocol == "http1" || protocol == "https-http1"
				var tlsConfig *tls.Config
				tlsCertPath, tlsConfig = testServerCertificate(t)
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- runServers(ctx) }()
				t.Cleanup(func() {
					cancel()
					podLeadership.stop()
					select {
					case err := <-done:
						if err != nil {
							t.Errorf("runServers() = %v", err)
						}
						waitForRefusedPorts(t, httpPort, httpSecurePort)
					case <-time.After(5 * time.Second):
						t.Error("servers did not stop")
					}
				})
				if action == nonLeaderActionSkipPortBinding {
					waitForRefusedPorts(t, httpPort, httpSecurePort)
				} else {
					waitForBoundPorts(t, httpPort, httpSecurePort)
				}

				baseURL := "http://127.0.0.1:" + httpPort
				var transport http.RoundTripper
				if protocol == "h2c" {
					tr := &http2.Transport{
						AllowHTTP: true,
						DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
							return (&net.Dialer{}).DialContext(ctx, network, addr)
						},
					}
					t.Cleanup(tr.CloseIdleConnections)
					transport = tr
				} else {
					tr := &http.Transport{TLSClientConfig: tlsConfig, ForceAttemptHTTP2: protocol == "https-http2"}
					t.Cleanup(tr.CloseIdleConnections)
					transport = tr
					if strings.HasPrefix(protocol, "https-") {
						baseURL = "https://127.0.0.1:" + httpSecurePort
					}
				}
				client := &http.Client{Transport: transport, Timeout: time.Second}
				checkResponse := func(wantStatus int) {
					t.Helper()
					deadline := time.Now().Add(5 * time.Second)
					for {
						response, err := client.Get(baseURL + "/keepalive")
						if err == nil {
							body, readErr := io.ReadAll(response.Body)
							response.Body.Close()
							if readErr != nil {
								t.Fatal(readErr)
							}
							wantProtocol := 1
							if protocol == "h2c" || protocol == "https-http2" {
								wantProtocol = 2
							}
							validBody := string(body) == "ok"
							if wantStatus == http.StatusServiceUnavailable {
								validBody = strings.Contains(string(body), "leader=false leader-election=true")
							}
							if response.StatusCode != wantStatus || response.ProtoMajor != wantProtocol || !validBody {
								t.Fatalf("response: status=%d protocol=%s body=%q", response.StatusCode, response.Proto, body)
							}
							return
						}
						if time.Now().After(deadline) {
							t.Fatalf("server did not respond: %v", err)
						}
						time.Sleep(10 * time.Millisecond)
					}
				}
				if action != nonLeaderActionSkipPortBinding {
					checkResponse(http.StatusServiceUnavailable)
				}
				term, loseLeadership := context.WithCancel(ctx)
				defer loseLeadership()
				podLeadership.start(term)
				waitForBoundPorts(t, httpPort, httpSecurePort)
				checkResponse(http.StatusOK)

				wsURL := "ws://127.0.0.1:" + httpPort + "/ws"
				if strings.HasPrefix(protocol, "https-") {
					wsURL = "wss://127.0.0.1:" + httpSecurePort + "/ws"
				}
				wsTLSConfig := tlsConfig.Clone()
				wsTLSConfig.NextProtos = []string{"http/1.1"}
				dialer := websocket.Dialer{TLSClientConfig: wsTLSConfig, HandshakeTimeout: time.Second}
				ws, _, err := dialer.Dial(wsURL, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer ws.Close()
				if err := ws.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
					t.Fatal(err)
				}
				if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				if _, _, err := ws.ReadMessage(); err != nil {
					t.Fatalf("leader WebSocket did not respond: %v", err)
				}

				var reflectionStream reflectionv1.ServerReflection_ServerReflectionInfoClient
				if protocol == "h2c" || protocol == "https-http2" {
					target := "127.0.0.1:" + httpPort
					var creds credentials.TransportCredentials = insecure.NewCredentials()
					if protocol == "https-http2" {
						target = "127.0.0.1:" + httpSecurePort
						creds = credentials.NewTLS(tlsConfig.Clone())
					}
					conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					rpcCtx, cancelRPC := context.WithTimeout(ctx, 3*time.Second)
					defer cancelRPC()
					reflectionStream, err = reflectionv1.NewServerReflectionClient(conn).ServerReflectionInfo(rpcCtx)
					if err != nil {
						t.Fatal(err)
					}
					if err := reflectionStream.Send(&reflectionv1.ServerReflectionRequest{
						MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{ListServices: ""},
					}); err != nil {
						t.Fatal(err)
					}
					response, err := reflectionStream.Recv()
					if err != nil || len(response.GetListServicesResponse().GetService()) == 0 {
						t.Fatalf("leader gRPC reflection response = %v, error = %v", response, err)
					}
				}

				loseLeadership()
				if action == nonLeaderActionSkipPortBinding {
					waitForRefusedPorts(t, httpPort, httpSecurePort)
					response, err := client.Get(baseURL + "/keepalive")
					if response != nil {
						response.Body.Close()
						t.Fatalf("non-leader served HTTP %d despite skip-port-binding", response.StatusCode)
					}
					if err == nil {
						t.Fatal("old connection must fail after losing port binding")
					}
					if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
						t.Fatal(err)
					}
					if _, _, err := ws.ReadMessage(); err == nil {
						t.Fatal("WebSocket must close after losing port binding")
					} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						t.Fatal("WebSocket was not closed on leadership loss")
					}
					if reflectionStream != nil {
						if _, err := reflectionStream.Recv(); err == nil || status.Code(err) == codes.DeadlineExceeded {
							t.Fatalf("gRPC stream must close promptly after losing port binding, got %v", err)
						}
					}
				} else {
					waitForBoundPorts(t, httpPort, httpSecurePort)
					checkResponse(http.StatusServiceUnavailable)
					if err := ws.WriteMessage(websocket.TextMessage, []byte("still connected")); err != nil {
						t.Fatal(err)
					}
					if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
						t.Fatal(err)
					}
					if _, _, err := ws.ReadMessage(); err != nil {
						t.Fatalf("WebSocket must remain usable after leadership loss: %v", err)
					}
					if reflectionStream != nil {
						if err := reflectionStream.Send(&reflectionv1.ServerReflectionRequest{
							MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{ListServices: ""},
						}); err != nil {
							t.Fatal(err)
						}
						if response, err := reflectionStream.Recv(); err != nil || len(response.GetListServicesResponse().GetService()) == 0 {
							t.Fatalf("gRPC stream must remain usable after leadership loss: response=%v error=%v", response, err)
						}
					}
				}

				nextTerm, cancelNext := context.WithCancel(ctx)
				defer cancelNext()
				podLeadership.start(nextTerm)
				waitForBoundPorts(t, httpPort, httpSecurePort)
				checkResponse(http.StatusOK)
				podLeadership.stop()
				if action == nonLeaderActionSkipPortBinding {
					waitForRefusedPorts(t, httpPort, httpSecurePort)
				} else {
					waitForBoundPorts(t, httpPort, httpSecurePort)
					checkResponse(http.StatusServiceUnavailable)
				}
			})
		}
	}
}

func TestServeWhileLeaderCancellation(t *testing.T) {
	for _, leading := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting", true: "serving"}[leading], func(t *testing.T) {
			var state leaderState
			if leading {
				state.start(context.Background())
				defer state.stop()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- serveWhileLeader(ctx, &state, func(ctx context.Context) error {
					close(started)
					<-ctx.Done()
					return nil
				})
			}()
			if leading {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("leader did not start serving")
				}
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("server manager did not stop on cancellation")
			}
			if !leading {
				select {
				case <-started:
					t.Fatal("non-leader must not start servers")
				default:
				}
			}
		})
	}
}

func TestServeWhileLeaderErrors(t *testing.T) {
	var state leaderState
	state.start(context.Background())
	defer state.stop()
	want := errors.New("bind failed")
	err := serveWhileLeader(context.Background(), &state, func(context.Context) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("serveWhileLeader() = %v, want %v", err, want)
	}
	err = serveWhileLeader(context.Background(), &state, func(context.Context) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "leadership was still active") {
		t.Fatalf("unexpected clean server exit must be reported, got %v", err)
	}
}

func TestRunServersPropagatesBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	oldPort, oldSecure := httpPort, httpSecurePort
	defer func() { httpPort, httpSecurePort = oldPort, oldSecure }()
	_, httpPort, err = net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	httpSecurePort = ""
	if err := runServers(context.Background()); !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("runServers() = %v, want address already in use", err)
	}
}

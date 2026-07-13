package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	wsURL := flag.String("url", "wss://127.0.0.1:10001/ws", "websocket URL")
	message := flag.String("message", "ping", "message payload to send")
	count := flag.Int("count", 10, "number of ping messages to send")
	interval := flag.Duration("interval", time.Second, "delay between messages")
	flag.Parse()

	if *count <= 0 {
		log.Fatalf("invalid count: %d", *count)
	}

	u, err := url.Parse(*wsURL)
	if err != nil {
		log.Fatalf("invalid url %q: %v", *wsURL, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		log.Fatalf("url scheme must be ws or wss, got %q", u.Scheme)
	}

	dialer := websocket.Dialer{}
	if strings.EqualFold(u.Scheme, "wss") {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	headers := http.Header{}
	conn, resp, err := dialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			log.Fatalf("dial failed: %v (status=%s)", err, resp.Status)
		}
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	log.Printf("connected to %s", u.String())

	for i := 1; i <= *count; i++ {
		payload := fmt.Sprintf("%s #%d", *message, i)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			log.Fatalf("write failed on iteration %d: %v", i, err)
		}

		msgType, reply, err := conn.ReadMessage()
		if err != nil {
			log.Fatalf("read failed on iteration %d: %v", i, err)
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			log.Printf("received non-data frame type=%d on iteration %d", msgType, i)
		} else {
			fmt.Printf("recv[%d]: %s\n", i, string(reply))
		}

		if i < *count {
			time.Sleep(*interval)
		}
	}

	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
		time.Now().Add(2*time.Second),
	)

	log.Printf("completed %d ping/pong iterations", *count)
}

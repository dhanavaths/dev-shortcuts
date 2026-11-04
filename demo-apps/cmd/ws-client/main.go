package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const (
	http2StreamID              = 1
	defaultHTTP2FrameSize      = 16 * 1024
	websocketContinuationFrame = 0
	websocketGUID              = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

type clientConfig struct {
	url      *url.URL
	message  string
	count    int
	interval time.Duration
}

type websocketFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

type http2WebSocket struct {
	conn              net.Conn
	framer            *http2.Framer
	streamID          uint32
	maxFrameSize      int
	buffer            []byte
	streamEnded       bool
	fragmentOpcode    byte
	fragmentedPayload []byte
}

func main() {
	wsURL := flag.String("url", "wss://127.0.0.1:10001/falcon-core.demo-app-rwpr/ws", "websocket URL")
	message := flag.String("message", "ping", "message payload to send")
	count := flag.Int("count", 10, "number of messages to send per protocol")
	interval := flag.Duration("interval", time.Second, "delay between messages")
	protocol := flag.String("protocol", "both", "protocol to test: both, http1, or http2")
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

	cfg := clientConfig{
		url:      u,
		message:  *message,
		count:    *count,
		interval: *interval,
	}

	var runHTTP1, runHTTP2 bool
	switch strings.ToLower(*protocol) {
	case "both":
		runHTTP1 = true
		runHTTP2 = true
	case "http1":
		runHTTP1 = true
	case "http2":
		runHTTP2 = true
	default:
		log.Fatalf("invalid protocol %q: must be both, http1, or http2", *protocol)
	}

	var testErrors []error
	if runHTTP1 {
		if err := testHTTP1(cfg); err != nil {
			testErrors = append(testErrors, fmt.Errorf("HTTP/1.1 test failed: %w", err))
		}
	}
	if runHTTP2 {
		if err := testHTTP2(cfg); err != nil {
			testErrors = append(testErrors, fmt.Errorf("HTTP/2 test failed: %w", err))
		}
	}
	if err := errors.Join(testErrors...); err != nil {
		log.Fatal(err)
	}
}

func testHTTP1(cfg clientConfig) error {
	dialer := websocket.Dialer{}
	if cfg.url.Scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"http/1.1"},
		}
	}

	conn, resp, err := dialer.Dial(cfg.url.String(), http.Header{})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial: %w (status=%s)", err, resp.Status)
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	log.Printf("[http1] connected to %s", cfg.url.String())

	for i := 1; i <= cfg.count; i++ {
		payload := fmt.Sprintf("%s #%d", cfg.message, i)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			return fmt.Errorf("write iteration %d: %w", i, err)
		}

		msgType, reply, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read iteration %d: %w", i, err)
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			return fmt.Errorf("iteration %d returned non-data frame type %d", i, msgType)
		}
		fmt.Printf("http1 recv[%d]: %s\n", i, string(reply))

		if i < cfg.count {
			time.Sleep(cfg.interval)
		}
	}

	if err := conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
		time.Now().Add(2*time.Second),
	); err != nil {
		return fmt.Errorf("send close frame: %w", err)
	}

	log.Printf("[http1] completed %d message exchanges", cfg.count)
	return nil
}

func testHTTP2(cfg clientConfig) error {
	conn, err := dialHTTP2(cfg.url)
	if err != nil {
		return err
	}
	defer conn.Close()

	framer := http2.NewFramer(conn, conn)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)

	if _, err := io.WriteString(conn, http2.ClientPreface); err != nil {
		return fmt.Errorf("write client preface: %w", err)
	}
	if err := framer.WriteSettings(); err != nil {
		return fmt.Errorf("write client settings: %w", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("set settings deadline: %w", err)
	}
	allowConnect, maxFrameSize, err := readServerSettings(framer)
	if err != nil {
		return err
	}
	if !allowConnect {
		return errors.New("server did not advertise SETTINGS_ENABLE_CONNECT_PROTOCOL=1")
	}

	key, err := websocketKey()
	if err != nil {
		return err
	}
	if err := writeExtendedConnectHeaders(framer, cfg.url, key); err != nil {
		return err
	}

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("set handshake deadline: %w", err)
	}
	if err := readExtendedConnectResponse(framer, key); err != nil {
		return err
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("clear handshake deadline: %w", err)
	}

	client := &http2WebSocket{
		conn:         conn,
		framer:       framer,
		streamID:     http2StreamID,
		maxFrameSize: maxFrameSize,
	}
	log.Printf("[http2] connected to %s with extended CONNECT", cfg.url.String())

	for i := 1; i <= cfg.count; i++ {
		payload := fmt.Sprintf("%s #%d", cfg.message, i)
		if err := client.writeWebSocketFrame(websocket.TextMessage, []byte(payload), false); err != nil {
			return fmt.Errorf("write iteration %d: %w", i, err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return fmt.Errorf("set read deadline for iteration %d: %w", i, err)
		}
		msgType, reply, err := client.readMessage()
		if err != nil {
			return fmt.Errorf("read iteration %d: %w", i, err)
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			return fmt.Errorf("iteration %d returned non-data frame type %d", i, msgType)
		}
		fmt.Printf("http2 recv[%d]: %s\n", i, string(reply))

		if i < cfg.count {
			time.Sleep(cfg.interval)
		}
	}

	if err := client.close(); err != nil {
		return fmt.Errorf("close extended CONNECT stream: %w", err)
	}

	log.Printf("[http2] completed %d message exchanges", cfg.count)
	return nil
}

func dialHTTP2(u *url.URL) (net.Conn, error) {
	address := u.Host
	if u.Port() == "" {
		defaultPort := "80"
		if u.Scheme == "wss" {
			defaultPort = "443"
		}
		address = net.JoinHostPort(u.Hostname(), defaultPort)
	}

	if u.Scheme == "ws" {
		conn, err := net.DialTimeout("tcp", address, 10*time.Second)
		if err != nil {
			return nil, fmt.Errorf("dial HTTP/2 TCP endpoint: %w", err)
		}
		return conn, nil
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		address,
		&tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
			ServerName:         u.Hostname(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("dial HTTP/2 TLS endpoint: %w", err)
	}
	if conn.ConnectionState().NegotiatedProtocol != "h2" {
		conn.Close()
		return nil, fmt.Errorf("server negotiated %q instead of h2", conn.ConnectionState().NegotiatedProtocol)
	}
	return conn, nil
}

func readServerSettings(framer *http2.Framer) (bool, int, error) {
	allowConnect := false
	maxFrameSize := defaultHTTP2FrameSize

	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return false, 0, fmt.Errorf("read server settings: %w", err)
		}

		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if frame.IsAck() {
				continue
			}
			if err := frame.ForeachSetting(func(setting http2.Setting) error {
				switch setting.ID {
				case http2.SettingEnableConnectProtocol:
					allowConnect = setting.Val == 1
				case http2.SettingMaxFrameSize:
					maxFrameSize = int(setting.Val)
				}
				return nil
			}); err != nil {
				return false, 0, fmt.Errorf("parse server settings: %w", err)
			}
			if err := framer.WriteSettingsAck(); err != nil {
				return false, 0, fmt.Errorf("acknowledge server settings: %w", err)
			}
			return allowConnect, maxFrameSize, nil
		case *http2.PingFrame:
			if !frame.IsAck() {
				if err := framer.WritePing(true, frame.Data); err != nil {
					return false, 0, fmt.Errorf("acknowledge HTTP/2 ping: %w", err)
				}
			}
		case *http2.GoAwayFrame:
			return false, 0, fmt.Errorf("server sent GOAWAY while negotiating settings: %s", frame.ErrCode)
		}
	}
}

func websocketKey() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate websocket key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(nonce), nil
}

func writeExtendedConnectHeaders(framer *http2.Framer, u *url.URL, key string) error {
	scheme := "http"
	if u.Scheme == "wss" {
		scheme = "https"
	}

	var headerBlock bytes.Buffer
	encoder := hpack.NewEncoder(&headerBlock)
	headers := []hpack.HeaderField{
		{Name: ":method", Value: http.MethodConnect},
		{Name: ":protocol", Value: "websocket"},
		{Name: ":scheme", Value: scheme},
		{Name: ":authority", Value: u.Host},
		{Name: ":path", Value: u.RequestURI()},
		{Name: "sec-websocket-version", Value: "13"},
		{Name: "sec-websocket-key", Value: key},
	}
	for _, header := range headers {
		if err := encoder.WriteField(header); err != nil {
			return fmt.Errorf("encode extended CONNECT header %s: %w", header.Name, err)
		}
	}

	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      http2StreamID,
		BlockFragment: headerBlock.Bytes(),
		EndHeaders:    true,
	}); err != nil {
		return fmt.Errorf("write extended CONNECT headers: %w", err)
	}
	return nil
}

func readExtendedConnectResponse(framer *http2.Framer, key string) error {
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return fmt.Errorf("read extended CONNECT response: %w", err)
		}

		switch frame := frame.(type) {
		case *http2.MetaHeadersFrame:
			if frame.StreamID != http2StreamID {
				continue
			}
			status := frame.PseudoValue("status")
			if status != "200" {
				return fmt.Errorf("extended CONNECT returned HTTP status %s", status)
			}
			var accept string
			for _, header := range frame.RegularFields() {
				if header.Name == "sec-websocket-accept" {
					accept = header.Value
					break
				}
			}
			expectedAccept := websocketAccept(key)
			if accept != expectedAccept {
				return fmt.Errorf("invalid sec-websocket-accept: got %q, want %q", accept, expectedAccept)
			}
			return nil
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				if err := framer.WriteSettingsAck(); err != nil {
					return fmt.Errorf("acknowledge HTTP/2 settings: %w", err)
				}
			}
		case *http2.PingFrame:
			if !frame.IsAck() {
				if err := framer.WritePing(true, frame.Data); err != nil {
					return fmt.Errorf("acknowledge HTTP/2 ping: %w", err)
				}
			}
		case *http2.RSTStreamFrame:
			if frame.StreamID == http2StreamID {
				return fmt.Errorf("extended CONNECT stream reset: %s", frame.ErrCode)
			}
		case *http2.GoAwayFrame:
			return fmt.Errorf("server sent GOAWAY during extended CONNECT: %s", frame.ErrCode)
		}
	}
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (client *http2WebSocket) writeWebSocketFrame(opcode byte, payload []byte, endStream bool) error {
	frame, err := encodeClientWebSocketFrame(opcode, payload)
	if err != nil {
		return err
	}
	return client.writeData(frame, endStream)
}

func (client *http2WebSocket) writeData(data []byte, endStream bool) error {
	if len(data) == 0 {
		if endStream {
			if err := client.framer.WriteData(client.streamID, true, nil); err != nil {
				return fmt.Errorf("end HTTP/2 stream: %w", err)
			}
		}
		return nil
	}

	for len(data) > 0 {
		chunkSize := min(len(data), client.maxFrameSize)
		lastChunk := chunkSize == len(data)
		if err := client.framer.WriteData(client.streamID, endStream && lastChunk, data[:chunkSize]); err != nil {
			return fmt.Errorf("write HTTP/2 DATA frame: %w", err)
		}
		data = data[chunkSize:]
	}
	return nil
}

func encodeClientWebSocketFrame(opcode byte, payload []byte) ([]byte, error) {
	if opcode >= 0x8 && len(payload) > 125 {
		return nil, fmt.Errorf("control frame payload is %d bytes; maximum is 125", len(payload))
	}

	headerLength := 2
	switch {
	case len(payload) <= 125:
	case len(payload) <= 65535:
		headerLength += 2
	default:
		headerLength += 8
	}

	frame := make([]byte, headerLength+4+len(payload))
	frame[0] = 0x80 | opcode
	offset := 2
	switch {
	case len(payload) <= 125:
		frame[1] = 0x80 | byte(len(payload))
	case len(payload) <= 65535:
		frame[1] = 0x80 | 126
		binary.BigEndian.PutUint16(frame[2:4], uint16(len(payload)))
		offset = 4
	default:
		frame[1] = 0x80 | 127
		binary.BigEndian.PutUint64(frame[2:10], uint64(len(payload)))
		offset = 10
	}

	mask := frame[offset : offset+4]
	if _, err := rand.Read(mask); err != nil {
		return nil, fmt.Errorf("generate websocket mask: %w", err)
	}
	maskedPayload := frame[offset+4:]
	for i := range payload {
		maskedPayload[i] = payload[i] ^ mask[i%len(mask)]
	}
	return frame, nil
}

func parseServerWebSocketFrame(data []byte) (websocketFrame, int, bool, error) {
	if len(data) < 2 {
		return websocketFrame{}, 0, false, nil
	}

	firstByte := data[0]
	secondByte := data[1]
	if firstByte&0x70 != 0 {
		return websocketFrame{}, 0, false, errors.New("server websocket frame uses unsupported RSV bits")
	}
	if secondByte&0x80 != 0 {
		return websocketFrame{}, 0, false, errors.New("server websocket frame must not be masked")
	}

	fin := firstByte&0x80 != 0
	opcode := firstByte & 0x0f
	payloadLength := uint64(secondByte & 0x7f)
	offset := 2

	switch payloadLength {
	case 126:
		if len(data) < 4 {
			return websocketFrame{}, 0, false, nil
		}
		payloadLength = uint64(binary.BigEndian.Uint16(data[2:4]))
		offset = 4
	case 127:
		if len(data) < 10 {
			return websocketFrame{}, 0, false, nil
		}
		payloadLength = binary.BigEndian.Uint64(data[2:10])
		if payloadLength&(1<<63) != 0 {
			return websocketFrame{}, 0, false, errors.New("server websocket frame has invalid 64-bit length")
		}
		offset = 10
	}

	if opcode >= 0x8 {
		if !fin {
			return websocketFrame{}, 0, false, errors.New("fragmented websocket control frame")
		}
		if payloadLength > 125 {
			return websocketFrame{}, 0, false, errors.New("websocket control frame payload exceeds 125 bytes")
		}
	}
	if payloadLength > uint64(^uint(0)>>1) {
		return websocketFrame{}, 0, false, errors.New("websocket frame payload exceeds platform limits")
	}
	totalLength := offset + int(payloadLength)
	if len(data) < totalLength {
		return websocketFrame{}, 0, false, nil
	}

	payload := append([]byte(nil), data[offset:totalLength]...)
	return websocketFrame{fin: fin, opcode: opcode, payload: payload}, totalLength, true, nil
}

func (client *http2WebSocket) nextWebSocketFrame() (websocketFrame, error) {
	for {
		frame, consumed, complete, err := parseServerWebSocketFrame(client.buffer)
		if err != nil {
			return websocketFrame{}, err
		}
		if complete {
			client.buffer = client.buffer[consumed:]
			return frame, nil
		}
		if client.streamEnded {
			return websocketFrame{}, io.ErrUnexpectedEOF
		}

		http2Frame, err := client.framer.ReadFrame()
		if err != nil {
			return websocketFrame{}, fmt.Errorf("read HTTP/2 frame: %w", err)
		}
		switch frame := http2Frame.(type) {
		case *http2.DataFrame:
			if frame.StreamID != client.streamID {
				continue
			}
			payload := frame.Data()
			client.buffer = append(client.buffer, payload...)
			if len(payload) > 0 {
				increment := uint32(len(payload))
				if err := client.framer.WriteWindowUpdate(0, increment); err != nil {
					return websocketFrame{}, fmt.Errorf("update HTTP/2 connection window: %w", err)
				}
				if err := client.framer.WriteWindowUpdate(client.streamID, increment); err != nil {
					return websocketFrame{}, fmt.Errorf("update HTTP/2 stream window: %w", err)
				}
			}
			client.streamEnded = frame.StreamEnded()
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				if err := client.framer.WriteSettingsAck(); err != nil {
					return websocketFrame{}, fmt.Errorf("acknowledge HTTP/2 settings: %w", err)
				}
			}
		case *http2.PingFrame:
			if !frame.IsAck() {
				if err := client.framer.WritePing(true, frame.Data); err != nil {
					return websocketFrame{}, fmt.Errorf("acknowledge HTTP/2 ping: %w", err)
				}
			}
		case *http2.RSTStreamFrame:
			if frame.StreamID == client.streamID {
				return websocketFrame{}, fmt.Errorf("HTTP/2 stream reset: %s", frame.ErrCode)
			}
		case *http2.GoAwayFrame:
			return websocketFrame{}, fmt.Errorf("server sent GOAWAY: %s", frame.ErrCode)
		}
	}
}

func (client *http2WebSocket) readMessage() (byte, []byte, error) {
	for {
		frame, err := client.nextWebSocketFrame()
		if err != nil {
			return 0, nil, err
		}

		switch frame.opcode {
		case websocketContinuationFrame:
			if client.fragmentOpcode == 0 {
				return 0, nil, errors.New("received websocket continuation without an initial fragment")
			}
			client.fragmentedPayload = append(client.fragmentedPayload, frame.payload...)
			if frame.fin {
				opcode := client.fragmentOpcode
				payload := client.fragmentedPayload
				client.fragmentOpcode = 0
				client.fragmentedPayload = nil
				return opcode, payload, nil
			}
		case websocket.TextMessage, websocket.BinaryMessage:
			if client.fragmentOpcode != 0 {
				return 0, nil, errors.New("received new websocket data frame during fragmented message")
			}
			if frame.fin {
				return frame.opcode, frame.payload, nil
			}
			client.fragmentOpcode = frame.opcode
			client.fragmentedPayload = append(client.fragmentedPayload[:0], frame.payload...)
		case websocket.PingMessage:
			if err := client.writeWebSocketFrame(websocket.PongMessage, frame.payload, false); err != nil {
				return 0, nil, fmt.Errorf("write websocket pong: %w", err)
			}
		case websocket.PongMessage:
			continue
		case websocket.CloseMessage:
			code, reason := parseClosePayload(frame.payload)
			return 0, nil, &websocket.CloseError{Code: code, Text: reason}
		default:
			return 0, nil, fmt.Errorf("received unsupported websocket opcode %d", frame.opcode)
		}
	}
}

func parseClosePayload(payload []byte) (int, string) {
	if len(payload) < 2 {
		return websocket.CloseNoStatusReceived, ""
	}
	return int(binary.BigEndian.Uint16(payload[:2])), string(payload[2:])
}

func (client *http2WebSocket) close() error {
	closePayload := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done")
	if err := client.writeWebSocketFrame(websocket.CloseMessage, closePayload, false); err != nil {
		return err
	}
	if err := client.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return fmt.Errorf("set close deadline: %w", err)
	}

	for {
		frame, err := client.nextWebSocketFrame()
		if err != nil {
			return err
		}
		switch frame.opcode {
		case websocket.CloseMessage:
			code, reason := parseClosePayload(frame.payload)
			if code != websocket.CloseNormalClosure {
				return fmt.Errorf("server closed websocket with code %d: %s", code, reason)
			}
			return client.writeData(nil, true)
		case websocket.PingMessage:
			if err := client.writeWebSocketFrame(websocket.PongMessage, frame.payload, false); err != nil {
				return fmt.Errorf("write websocket pong during close: %w", err)
			}
		}
	}
}

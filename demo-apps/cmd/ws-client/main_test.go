package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gorilla/websocket"
)

func TestEncodeClientWebSocketFrame(t *testing.T) {
	payload := []byte("hello")
	frame, err := encodeClientWebSocketFrame(websocket.TextMessage, payload)
	if err != nil {
		t.Fatalf("encodeClientWebSocketFrame() error = %v", err)
	}

	if frame[0] != 0x81 {
		t.Fatalf("first byte = %#x, want %#x", frame[0], byte(0x81))
	}
	if frame[1]&0x80 == 0 {
		t.Fatal("client frame is not masked")
	}
	if got := int(frame[1] & 0x7f); got != len(payload) {
		t.Fatalf("payload length = %d, want %d", got, len(payload))
	}

	mask := frame[2:6]
	decoded := make([]byte, len(payload))
	for i := range payload {
		decoded[i] = frame[6+i] ^ mask[i%len(mask)]
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded payload = %q, want %q", decoded, payload)
	}
}

func TestParseServerWebSocketFrame(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		header  []byte
	}{
		{
			name:    "short payload",
			payload: []byte("hello"),
			header:  []byte{0x81, 5},
		},
		{
			name:    "16-bit payload",
			payload: bytes.Repeat([]byte("a"), 126),
			header:  []byte{0x82, 126, 0, 126},
		},
		{
			name:    "64-bit payload",
			payload: bytes.Repeat([]byte("b"), 65536),
			header: func() []byte {
				header := []byte{0x82, 127, 0, 0, 0, 0, 0, 0, 0, 0}
				binary.BigEndian.PutUint64(header[2:], 65536)
				return header
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := append(append([]byte(nil), tt.header...), tt.payload...)
			frame, consumed, complete, err := parseServerWebSocketFrame(wire)
			if err != nil {
				t.Fatalf("parseServerWebSocketFrame() error = %v", err)
			}
			if !complete {
				t.Fatal("parseServerWebSocketFrame() complete = false")
			}
			if consumed != len(wire) {
				t.Fatalf("consumed = %d, want %d", consumed, len(wire))
			}
			if !bytes.Equal(frame.payload, tt.payload) {
				t.Fatalf("payload length = %d, want %d", len(frame.payload), len(tt.payload))
			}
		})
	}
}

func TestParseServerWebSocketFrameIncomplete(t *testing.T) {
	frame, consumed, complete, err := parseServerWebSocketFrame([]byte{0x81, 5, 'h'})
	if err != nil {
		t.Fatalf("parseServerWebSocketFrame() error = %v", err)
	}
	if complete {
		t.Fatalf("complete = true, frame = %#v, consumed = %d", frame, consumed)
	}
}

func TestWebSocketAccept(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	const want = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := websocketAccept(key); got != want {
		t.Fatalf("websocketAccept() = %q, want %q", got, want)
	}
}

package grpcdemo

import (
	"context"
	"testing"
)

func TestInc(t *testing.T) {
	server := NewGrpcDemoServiceImpl()

	for _, test := range []struct {
		name    string
		counter int64
		want    int64
	}{
		{name: "zero", counter: 0, want: 1},
		{name: "positive", counter: 41, want: 42},
		{name: "negative", counter: -2, want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			reply, err := server.Inc(context.Background(), &IncRequest{Counter: test.counter})
			if err != nil {
				t.Fatalf("Inc() error = %v", err)
			}
			if got := reply.GetCounter(); got != test.want {
				t.Errorf("Inc() counter = %d, want %d", got, test.want)
			}
		})
	}
}

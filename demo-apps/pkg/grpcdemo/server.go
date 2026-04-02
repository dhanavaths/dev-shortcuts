package grpcdemo

import (
	"context"
	"fmt"
	"io"
	"log"

	"google.golang.org/grpc"
)

// GrpcDemoServiceImpl implements GrpcDemoServiceServer.
type GrpcDemoServiceImpl struct {
	UnimplementedGrpcDemoServiceServer
}

// NewGrpcDemoServiceImpl returns a new GrpcDemoServiceImpl.
func NewGrpcDemoServiceImpl() *GrpcDemoServiceImpl {
	return &GrpcDemoServiceImpl{}
}

// Ping is a unary RPC that echoes the request message back as a pong.
func (s *GrpcDemoServiceImpl) Ping(ctx context.Context, req *PingRequest) (*PingReply, error) {
	log.Printf("Ping received: %s", req.GetMessage())
	return &PingReply{
		Message: fmt.Sprintf("Pong: %s", req.GetMessage()),
	}, nil
}

// StreamMessages is a bidirectional streaming RPC that reads from the client
// stream until it is closed and sends a reply for every incoming message.
func (s *GrpcDemoServiceImpl) StreamMessages(stream grpc.BidiStreamingServer[StreamRequest, StreamReply]) error {
	sequence := int32(0)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			log.Printf("StreamMessages completed after %d messages", sequence)
			return nil
		}
		if err != nil {
			log.Printf("StreamMessages encountered an error after %d messages: %v", sequence, err)
			return err
		}

		sequence++
		log.Printf("StreamMessages received #%d: %s", sequence, req.GetMessage())

		if err := stream.Send(&StreamReply{
			Sequence: sequence,
			Message:  fmt.Sprintf("Echo: %s", req.GetMessage()),
		}); err != nil {
			log.Printf("StreamMessages send error after %d messages: %v", sequence, err)
			return err
		}
	}
}

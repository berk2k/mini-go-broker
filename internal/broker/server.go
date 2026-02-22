package broker

import (
	"context"
	"log"

	brokerv1 "github.com/berk2k/mini-go-broker/api/proto/gen"
	"github.com/berk2k/mini-go-broker/internal/queue/inmem"

	"github.com/google/uuid"
)

type Server struct {
	brokerv1.UnimplementedBrokerServiceServer
	Queue *inmem.Queue
}

func (s *Server) Publish(ctx context.Context, req *brokerv1.PublishRequest) (*brokerv1.PublishResponse, error) {

	id := uuid.NewString()

	s.Queue.Enqueue(inmem.Message{
		ID:      id,
		Payload: req.Payload,
	})

	log.Printf("Message published. Queue size=%d\n", s.Queue.Size())

	return &brokerv1.PublishResponse{
		MessageId: id,
	}, nil
}

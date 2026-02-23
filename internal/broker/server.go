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

func (s *Server) Consume(
	req *brokerv1.ConsumeRequest,
	stream brokerv1.BrokerService_ConsumeServer,
) error {

	for {
		msg := s.Queue.DequeueBlocking()

		err := stream.Send(&brokerv1.Delivery{
			DeliveryId: msg.ID,
			MessageId:  msg.ID,
			Payload:    msg.Payload,
			Attempt:    1,
		})

		if err != nil {
			log.Println("Consumer disconnected")
			return err
		}
	}
}

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

	consumerID := req.ConsumerId
	if consumerID == "" {
		consumerID = "default"
	}

	for {
		deliveryID, msg := s.Queue.DequeueLeaseBlocking(consumerID)

		err := stream.Send(&brokerv1.Delivery{
			DeliveryId: deliveryID,
			MessageId:  msg.ID,
			Payload:    msg.Payload,
			Attempt:    uint32(msg.Attempts),
		})

		if err != nil {
			return err
		}
	}
}

func (s *Server) Ack(
	ctx context.Context,
	req *brokerv1.AckRequest,
) (*brokerv1.AckResponse, error) {

	err := s.Queue.Ack(req.DeliveryId, req.ConsumerId)
	if err != nil {
		return nil, err
	}

	return &brokerv1.AckResponse{}, nil
}

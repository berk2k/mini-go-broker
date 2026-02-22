package main

import (
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	brokerv1 "github.com/berk2k/mini-go-broker/api/proto/gen"
	"github.com/berk2k/mini-go-broker/internal/broker"
	"github.com/berk2k/mini-go-broker/internal/queue/inmem"
)

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	queue := inmem.NewQueue()

	grpcServer := grpc.NewServer()

	srv := &broker.Server{
		Queue: queue,
	}

	brokerv1.RegisterBrokerServiceServer(grpcServer, srv)

	reflection.Register(grpcServer)

	log.Println("Broker running on :50051")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

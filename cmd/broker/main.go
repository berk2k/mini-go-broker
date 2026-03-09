package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("Shutdown signal received...")

		queue.Shutdown()

		drainTimeout := 10 * time.Second
		deadline := time.Now().Add(drainTimeout)

		for {
			inflight := queue.InflightSize()
			if inflight == 0 {
				log.Println("All inflight messages drained.")
				break
			}

			if time.Now().After(deadline) {
				log.Println("Drain timeout reached. Forcing requeue...")
				queue.ForceRequeueAll()
				break
			}

			time.Sleep(500 * time.Millisecond)
		}

		grpcServer.GracefulStop()
	}()

	log.Println("Broker running on :50051")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

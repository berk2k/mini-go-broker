package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	brokerv1 "github.com/berk2k/mini-go-broker/api/proto/gen"
	"github.com/berk2k/mini-go-broker/internal/broker"
	"github.com/berk2k/mini-go-broker/internal/observability"
	"github.com/berk2k/mini-go-broker/internal/queue/inmem"
)

func main() {
	logger := observability.NewLogger()

	queue := inmem.NewQueue(logger)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		logger.Error("failed to listen", slog.String("error", err.Error()))
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	srv := &broker.Server{Queue: queue, Logger: logger}
	brokerv1.RegisterBrokerServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	observability.StartMetricsServer(queue, ":8080", logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("broker_started", slog.String("port", ":50051"))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("grpc_serve_failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown_signal_received")

	queue.Shutdown()

	drainTimeout := 10 * time.Second
	deadline := time.Now().Add(drainTimeout)

	for {
		inflight := queue.InflightSize()

		if inflight == 0 {
			logger.Info("drain_complete")
			break
		}

		if time.Now().After(deadline) {
			logger.Warn("drain_timeout_reached", slog.Int("inflight", inflight))
			queue.ForceRequeueAll()
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	grpcServer.GracefulStop()
	logger.Info("broker_stopped")
}

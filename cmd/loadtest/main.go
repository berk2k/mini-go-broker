package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	brokerv1 "github.com/berk2k/mini-go-broker/api/proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	brokerAddr    = "localhost:50051"
	totalMessages = 1000
	consumerCount = 3
	prefetch      = 10
)

func main() {
	conn, err := grpc.NewClient(brokerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := brokerv1.NewBrokerServiceClient(conn)

	var (
		totalAcked   atomic.Int64
		totalLatency atomic.Int64
	)

	// --- Start consumers ---
	var wg sync.WaitGroup
	for i := 0; i < consumerCount; i++ {
		wg.Add(1)
		go func(consumerID string) {
			defer wg.Done()
			runConsumer(client, consumerID, prefetch, &totalAcked, &totalLatency)
		}(fmt.Sprintf("consumer-%d", i+1))
	}

	// --- Publish messages ---
	fmt.Printf("Publishing %d messages...\n", totalMessages)
	publishStart := time.Now()

	for i := 0; i < totalMessages; i++ {
		_, err := client.Publish(context.Background(), &brokerv1.PublishRequest{
			Payload: []byte(fmt.Sprintf("message-%d", i)),
		})
		if err != nil {
			log.Printf("publish error: %v", err)
		}
	}

	publishDuration := time.Since(publishStart)
	fmt.Printf("Published %d messages in %v\n", totalMessages, publishDuration)

	// --- Wait for all messages to be acked ---
	fmt.Println("Waiting for consumers to process all messages...")
	for {
		acked := totalAcked.Load()
		if acked >= totalMessages {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// --- Results ---
	acked := totalAcked.Load()
	totalLatencyMs := totalLatency.Load()
	avgLatencyMs := float64(totalLatencyMs) / float64(acked)
	throughput := float64(acked) / publishDuration.Seconds()

	fmt.Println("\n--- Load Test Results ---")
	fmt.Printf("Messages published:     %d\n", totalMessages)
	fmt.Printf("Messages acked:         %d\n", acked)
	fmt.Printf("Publish duration:       %v\n", publishDuration)
	fmt.Printf("Throughput:             %.0f msg/sec\n", throughput)
	fmt.Printf("Avg ack latency:        %.2f ms\n", avgLatencyMs)
}

func runConsumer(
	client brokerv1.BrokerServiceClient,
	consumerID string,
	prefetch int,
	totalAcked *atomic.Int64,
	totalLatency *atomic.Int64,
) {
	stream, err := client.Consume(context.Background(), &brokerv1.ConsumeRequest{
		ConsumerId: consumerID,
		Prefetch:   uint32(prefetch),
	})
	if err != nil {
		log.Printf("consumer %s failed to connect: %v", consumerID, err)
		return
	}

	for {
		delivery, err := stream.Recv()
		if err != nil {
			return
		}

		start := time.Now()

		_, ackErr := client.Ack(context.Background(), &brokerv1.AckRequest{
			DeliveryId: delivery.DeliveryId,
			ConsumerId: consumerID,
		})

		if ackErr != nil {
			log.Printf("ack error: %v", ackErr)
			continue
		}

		latency := time.Since(start).Milliseconds()
		totalLatency.Add(latency)
		totalAcked.Add(1)
	}
}

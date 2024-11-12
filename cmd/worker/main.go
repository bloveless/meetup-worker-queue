package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"queue-worker/internal/models"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	priority, id := os.Args[1], os.Args[2]
	consumerName := fmt.Sprintf("worker_%s", priority)
	workerName := fmt.Sprintf("worker_%s_%s", priority, id)

	logger := slog.With("workerName", workerName)

	logger.Info("Booted")

	nc, err := nats.Connect("localhost", nats.Name(workerName))
	if err != nil {
		logger.Error("unable to connect to nats", "err", err)
		os.Exit(1)
	}
	defer nc.Close()

	logger.Info("connected", "url", nc.ConnectedUrl())

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("unable to connect to jetstream", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	consumer, err := js.CreateOrUpdateConsumer(ctx, "jobs", jetstream.ConsumerConfig{
		Name:        consumerName,
		Durable:     consumerName,
		Description: fmt.Sprintf("Worker pool with priority %s", priority),
		BackOff: []time.Duration{
			5 * time.Second,
			10 * time.Second,
			15 * time.Second,
		},
		MaxDeliver: 4,
		FilterSubjects: []string{
			fmt.Sprintf("jobs.%s.>", priority),
		},
	})
	if err != nil {
		logger.Error("unable to create or update consumer", "err", err)
		os.Exit(1)
	}

	c, err := consumer.Consume(func(msg jetstream.Msg) {
		meta, err := msg.Metadata()
		if err != nil {
			logger.Error("getting metadata", "err", err)
			return
		}

		var num models.Number
		json.NewDecoder(bytes.NewReader(msg.Data())).Decode(&num)

		logger.Info("received number", "id", num.ID, "num", num.Num)

		if num.Num >= 8 {
			logger.Error("random number >= 8 simulating worker failure")
			logger.Info("Sequence", "id", num.ID, "consumer", meta.Sequence.Consumer, "stream", meta.Sequence.Stream)
			return
		}

		logger.Info("Received message sequence", "sequence", meta.Sequence.Stream)
		time.Sleep(10 * time.Millisecond)
		msg.Ack()
	})
	if err != nil {
		slog.Error("consumer errored", "err", err)
		os.Exit(1)
	}

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	c.Stop()
}

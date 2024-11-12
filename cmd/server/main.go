package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"queue-worker/internal/id"
	"queue-worker/internal/models"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
)

const (
	exitCodeErr       = 1
	exitCodeInterrupt = 2
)

func main() {
	fmt.Print("Starting server again")

	natsServer, err := setupNATSServer()
	if err != nil {
		panic(err)
	}
	defer natsServer.Shutdown()

	slog.Info("NATS server is ready to accept connections")

	// Configure context for shutdown handling
	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	nc, err := nats.Connect("localhost", nats.InProcessServer(natsServer))
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatal(err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:        "jobs",
		Description: "Work that needs to be done",
		Subjects: []string{
			"jobs.*.*", // jobs.PRIORITY.TYPE I.E. jobs.high.send_email
		},
		Replicas:  1,
		Retention: nats.WorkQueuePolicy,
		Discard:   nats.DiscardNew, // Discard any new messages if the topic cannot accept new messages
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:        "jobs_dlq",
		Description: "Jobs that have failed",
		Subjects: []string{
			"$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.jobs.*",
		},
	})

	wg := startCreatingMessages(ctx, nc)

	go func() {
		select {
		case <-signalChan: // first signal, cancel context
			natsServer.Shutdown()
			cancel()
		case <-ctx.Done():
		}
		<-signalChan // second signal, hard exit
		os.Exit(exitCodeInterrupt)
	}()

	if err := wg.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		if errors.Is(err, context.Canceled) {
			os.Exit(0)
		}

		os.Exit(exitCodeErr)
	}
}

func startCreatingMessages(ctx context.Context, nc *nats.Conn) errgroup.Group {
	wg := errgroup.Group{}
	wg.Go(func() error {
		for {
			num := rand.Intn(10)
			var b bytes.Buffer
			err := json.NewEncoder(&b).Encode(models.Number{ID: id.Generate(), Num: num})
			if err != nil {
				slog.Error("error encoding number message", "err", err)
			}

			nc.Publish("jobs.high.send_email", b.Bytes())

			slog.Info("created jobs.high.send_email")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
		}
	})

	wg.Go(func() error {
		for {
			num := rand.Intn(10)
			var b bytes.Buffer
			err := json.NewEncoder(&b).Encode(models.Number{Num: num})
			if err != nil {
				slog.Error("error encoding number message", "err", err)
			}

			nc.Publish("jobs.low.calc_stats", b.Bytes())

			slog.Info("created jobs.low.calc_stats")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	})

	return wg
}

func setupNATSServer() (*natsserver.Server, error) {
	opts := &natsserver.Options{
		Debug:     false,
		Trace:     false,
		JetStream: true,
		StoreDir:  "./storage",
	}
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("setting up nats server: %w", err)
	}

	go ns.Start()

	ns.ConfigureLogger()

	maxWait := 4 * time.Second
	if !ns.ReadyForConnections(10 * time.Second) {
		return nil, fmt.Errorf("nats server wasn't able to accept connections after %s", maxWait)
	}

	// Reset any signals that were set by nats server Start
	signal.Reset()

	return ns, nil
}

package main

import (
	"log"
	"os"
	"time"

	"github.com/jhunt/go-cli"
	env "github.com/jhunt/go-envirotron"
	"github.com/streadway/amqp"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func main() {
	var opts struct {
		URL      string `cli:"-u, --url"  env:"RMQ_MIGRATE_URL"`
		Queue    string `cli:"-q, --queue"    env:"RMQ_MIGRATE_QUEUE"`
		Exchange string `cli:"-x, --exchange" env:"RMQ_MIGRATE_EXCHANGE"`

		Init struct{} `cli:"init"`
		RX   struct{} `cli:"rx"`
		TX   struct{} `cli:"tx"`
	}
	opts.URL = "amqp://rmq:sekrit@localhost:25672/"
	opts.Queue = "rmq-default"
	env.Override(&opts)
	command, _, err := cli.Parse(&opts)
	if err != nil {
		log.Fatalf("!! %s", err)
	}

	if opts.Exchange == "" {
		opts.Exchange = opts.Queue
	}

	if opts.Queue == "" {
		log.Fatalf("!! missing required --queue / $RMQ_MIGRATE_QUEUE")
	}
	if opts.Exchange == "" {
		log.Fatalf("!! missing required --exchange / $RMQ_MIGRATE_EXCHANGE")
	}
	if opts.URL == "" {
		log.Fatalf("!! missing required --url / $RMQ_MIGRATE_URL")
	}

	tx := NewTransmitter(Wrap)
	rx := NewReceiver(1000)

	for {
		log.Printf("connecting to %s...", opts.URL)
		conn, err := amqp.Dial(opts.URL)
		i := 0
		for err != nil {
			if i%200 == 0 {
				log.Printf("failed to connect to %s: %s", opts.URL, err)
			}
			i++
			nap(50)
			conn, err = amqp.Dial(opts.URL)
		}
		defer conn.Close()

		log.Printf("opening comms channel...")
		ch, err := conn.Channel()
		i = 0
		for err != nil {
			if i%200 == 0 {
				log.Printf("failed to open comms channel: %s", err)
			}
			i++
			nap(50)
			ch, err = conn.Channel()
		}
		defer ch.Close()

		switch command {
		case "init":
			log.Printf("INIT: setting up queue %s and exchange %s...", opts.Queue, opts.Exchange)
			if err := setup(ch, opts.Queue, opts.Exchange); err != nil {
				log.Printf("oops: %s", err)
			} else {
				os.Exit(0)
			}

		case "tx":
			log.Printf("TX: sending messages to %s via %s...", opts.Queue, opts.Exchange)
			tx.Run(ch, opts.Queue, time.NewTicker(Nap*time.Millisecond))

		case "rx":
			log.Printf("RX: receiving messages from %s via %s...", opts.Queue, opts.Exchange)
			rx.Run(ch, opts.Queue)

		default:
			log.Fatalf("unrecognized sub-command '%s'", command)
		}
	}
}

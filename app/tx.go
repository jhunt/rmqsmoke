package main

import (
	"fmt"
	"log"
	"time"

	"github.com/streadway/amqp"
)

type Transmitter struct {
	period  int
	batch   int
	counter int
}

func NewTransmitter(n int) *Transmitter {
	return &Transmitter{
		period:  n,
		batch:   0,
		counter: 0,
	}
}

func (tx Transmitter) State() string {
	return fmt.Sprintf("%d|%d", tx.batch, tx.counter)
}

func (tx *Transmitter) Next() {
	tx.counter++
	if tx.counter >= tx.period {
		log.Printf("--[ tx.COMPLETE %d 100%% ]----------", tx.batch)
		tx.batch++
		tx.counter = 0
	}
}

func (tx *Transmitter) Run(ch *amqp.Channel, queue string, t *time.Ticker) {
	if err := ch.Confirm(false); err != nil {
		log.Printf("unable to put channel in 'confirm' mode: %s", err)
		return
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	q, err := ch.QueueInspect(queue)
	if err != nil {
		log.Printf("unable to inspect queue: %s", err)
		return
	}

	log.Printf("initiating TX ->%s @%s", queue, tx.State())
	for range t.C {
		if tx.counter == 0 {
			log.Printf("--[ tx.START    %d   0%% ]----------", tx.batch)
		}

		now := fmt.Sprintf("%s|%s", time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"), tx.State())
		for {
			err = ch.Publish(
				"",     // exchange
				q.Name, // routing key
				false,  // mandatory
				false,  // immediate
				amqp.Publishing{
					ContentType:  "text/plain",
					DeliveryMode: amqp.Persistent,
					Body:         []byte(now),
				})
			if err != nil {
				fmt.Printf("\n")
				log.Printf("last sent '%s'", tx.State())
				return
			}

			if confirmation := <-confirms; confirmation.Ack {
				break
			}
		}

		if Debug {
			log.Printf("SENT [%s]", now)
		}

		tx.Next()
	}
}

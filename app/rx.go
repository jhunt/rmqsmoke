package main

import (
	"log"
	"strconv"
	"strings"

	"github.com/jhunt/rmqsmoke/data"

	"github.com/streadway/amqp"
)

func dparse(d amqp.Delivery) (string, int, int) {
	return parse(string(d.Body))
}

func parse(s string) (string, int, int) {
	l := strings.Split(s, "|")
	if len(l) != 3 {
		log.Fatalf("message [%s] failed: unexpected format!", s)
	}
	x, err := strconv.ParseInt(l[1], 10, 64)
	if err != nil {
		log.Fatalf("message [%s] failed: %s", s, err)
	}
	y, err := strconv.ParseInt(l[2], 10, 64)
	if err != nil {
		log.Fatalf("message [%s] failed: %s", s, err)
	}
	return l[0], int(x), int(y)
}

type Receiver struct {
	cards  []*data.Card
	max    int
	latest int
}

func NewReceiver(max int) *Receiver {
	return &Receiver{
		cards: make([]*data.Card, max),
		max:   max,
	}
}

func (rx *Receiver) Track(batch, counter int) {
	if batch > rx.latest {
		rx.latest = batch
	}
	if batch >= rx.max {
		return
	}

	if rx.cards[batch] == nil {
		log.Printf("--[ rx.START    %d   0%% ]----------", batch)
		rx.cards[batch] = data.NewCard(Wrap)
		rx.Summarize()
	}

	card := rx.cards[batch]
	card.Track(counter)
	if card.Complete() {
		log.Printf("--[ rx.COMPLETE %d 100%% ]----------", batch)
	}
}

func (rx *Receiver) Summarize() {
	a := 0
	b := 0
	for i, card := range rx.cards {
		if card == nil {
			continue
		}

		if card.Complete() {
			b = i
		} else {
			if i == rx.latest {
				break
			}

			if b > a {
				log.Printf("batch %d ... %d COMPLETE", a, b)
			}
			log.Printf("batch %d INCOMPLETE", i)
			log.Printf("   - missing %d data points", card.Missing())
			if card.Missing() < 10 {
				log.Printf("    - %v", card.MissingValues())
			}
			b = i
			a = i
		}
	}
	if b > a {
		log.Printf("batch %d ... %d COMPLETE", a, b)
	}
	log.Printf("batch %d IN PROGRESS", rx.latest)
}

func (rx *Receiver) Run(ch *amqp.Channel, queue string) {
	q, err := ch.QueueInspect(queue)
	if err != nil {
		log.Printf("unable to inspect queue: %s", err)
		return
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	for d := range msgs {
		now, batch, counter := dparse(d)
		if Debug {
			log.Printf("RECV [%s|%d|%d]", now, batch, counter)
		}

		rx.Track(batch, counter)

		if err := d.Ack(false); err != nil {
			log.Printf("!!!!! failed to ack (%d:%d): %s", batch, counter, err)
			return
		}
	}

	log.Printf("=========[ the story so far... ]=============")
	rx.Summarize()
}

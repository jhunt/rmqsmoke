package main

import (
	"github.com/streadway/amqp"
)

func setup(ch *amqp.Channel, queue, exchange string) error {
	err := ch.ExchangeDeclare(
		exchange, // name
		"direct", // kind
		true,     // durable
		false,    // delete when unused
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return err
	}

	_, err = ch.QueueDeclare(
		queue, // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return err
	}

	err = ch.QueueBind(
		queue,    // queue name
		queue,    // routing key
		exchange, // exchange
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return err
	}

	return nil
}

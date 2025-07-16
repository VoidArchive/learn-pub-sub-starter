package pubsub

import (
	"encoding/json"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AckType int

const (
	Ack AckType = iota
	NackRequeue
	NackDiscard
)

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) AckType,
) error {
	ch, q, err := DeclareAndBind(
		conn,
		exchange,
		queueName,
		key,
		queueType,
	)
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			var val T
			if err := json.Unmarshal(msg.Body, &val); err != nil {
				fmt.Println("error unmarshaling message: ", err)
				log.Println("NackDiscard: error unmarshaling message")
				msg.Nack(false, false)
				continue
			}
			ackType := handler(val)
			switch ackType {
			case Ack:
				log.Println("Ack: message acknowledged")
				msg.Ack(false)
			case NackRequeue:
				log.Println("NackRequeue: message rejected and requeued")
				msg.Nack(false, true)
			case NackDiscard:
				log.Println("NackDiscard: message rejected and discarded")
				msg.Nack(false, false)
			}
		}
	}()
	return nil
}

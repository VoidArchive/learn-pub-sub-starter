package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	const rabbitConnString = "amqp://guest:guest@localhost:5672/"
	conn, err := amqp.Dial(rabbitConnString)
	if err != nil {
		log.Fatalf("could not connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	username, _ := gamelogic.ClientWelcome()

	queueName := routing.PauseKey + "." + username
	routingKey := routing.PauseKey
	exchange := routing.ExchangePerilDirect

	ch, q, err := pubsub.DeclareAndBind(
		conn,
		exchange,
		queueName,
		routingKey,
		pubsub.Transient,
	)
	if err != nil {
		log.Fatalf("couldn't declare and bind queue %v", err)
	}

	defer ch.Close()

	fmt.Println("Starting Peril client...")
	fmt.Println("Listening on queue:", q.Name)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("Shutting down...")
}

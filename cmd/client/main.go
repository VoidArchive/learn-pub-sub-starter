package main

import (
	"fmt"
	"log"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) {
	return func(state routing.PlayingState) {
		defer fmt.Print("> ")
		gs.HandlePause(state)
	}
}

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

	gs := gamelogic.NewGameState(username)

	err = pubsub.SubscribeJSON(
		conn,
		exchange,
		queueName,
		routingKey,
		pubsub.Transient,
		handlerPause(gs),
	)
	if err != nil {
		log.Fatalf("failed to subscribe to pause queue: %v", err)
	}

	for {
		words := gamelogic.GetInput()

		if len(words) == 0 {
			continue
		}

		switch words[0] {
		case "move":
			_, err := gs.CommandMove(words)
			if err != nil {
				fmt.Println(err)
				continue
			}

		case "spawn":
			err = gs.CommandSpawn(words)
			if err != nil {
				fmt.Println(err)
				continue
			}

		case "status":
			gs.CommandStatus()
		case "help":
			gamelogic.PrintClientHelp()
		case "spam":
			fmt.Println("not allowed yet")
		case "quit":
			gamelogic.PrintQuit()
			return
		default:
			fmt.Println("unknown command")
		}
	}
}

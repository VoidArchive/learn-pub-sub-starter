package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {
	return func(state routing.PlayingState) pubsub.AckType {
		defer fmt.Print("> ")
		gs.HandlePause(state)
		return pubsub.Ack
	}
}

func handlerMove(gs *gamelogic.GameState, publishCh *amqp.Channel) func(gamelogic.ArmyMove) pubsub.AckType {
	return func(move gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(move)
		
		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			// Publish war message
			warMessage := gamelogic.RecognitionOfWar{
				Attacker: move.Player,
				Defender: gs.GetPlayerSnap(),
			}
			warRoutingKey := routing.WarRecognitionsPrefix + "." + gs.GetUsername()
			err := pubsub.PublishJSON(publishCh, routing.ExchangePerilTopic, warRoutingKey, warMessage)
			if err != nil {
				fmt.Printf("failed to publish war message: %v\n", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.NackDiscard
		default:
			return pubsub.NackDiscard
		}
	}
}

func publishGameLog(publishCh *amqp.Channel, username, message string) error {
	gameLog := routing.GameLog{
		CurrentTime: time.Now(),
		Message:     message,
		Username:    username,
	}
	
	logRoutingKey := routing.GameLogSlug + "." + username
	return pubsub.PublishGob(publishCh, routing.ExchangePerilTopic, logRoutingKey, gameLog)
}

func handlerWar(gs *gamelogic.GameState, publishCh *amqp.Channel) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(rw gamelogic.RecognitionOfWar) pubsub.AckType {
		defer fmt.Print("> ")
		outcome, winner, loser := gs.HandleWar(rw)
		
		// Create log message based on outcome
		var logMessage string
		var initiator string
		
		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeOpponentWon:
			logMessage = fmt.Sprintf("%s won a war against %s", winner, loser)
			initiator = rw.Attacker.Username
		case gamelogic.WarOutcomeYouWon:
			logMessage = fmt.Sprintf("%s won a war against %s", winner, loser)
			initiator = rw.Attacker.Username
		case gamelogic.WarOutcomeDraw:
			logMessage = fmt.Sprintf("A war between %s and %s resulted in a draw", winner, loser)
			initiator = rw.Attacker.Username
		default:
			fmt.Printf("Error: unknown war outcome: %v\n", outcome)
			return pubsub.NackDiscard
		}
		
		// Publish game log
		err := publishGameLog(publishCh, initiator, logMessage)
		if err != nil {
			fmt.Printf("failed to publish game log: %v\n", err)
			return pubsub.NackRequeue
		}
		
		return pubsub.Ack
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

	// Create a publishing channel for army moves
	publishCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("could not create publishing channel: %v", err)
	}
	defer publishCh.Close()

	// Declare the topic exchange for army moves
	err = publishCh.ExchangeDeclare(
		routing.ExchangePerilTopic,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Fatalf("could not declare topic exchange: %v", err)
	}

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

	// Subscribe to army moves
	armyMovesQueueName := routing.ArmyMovesPrefix + "." + username
	armyMovesRoutingKey := routing.ArmyMovesPrefix + ".*"
	armyMovesExchange := routing.ExchangePerilTopic

	fmt.Printf("Subscribing to army moves with queue: %s, routing key: %s, exchange: %s\n", 
		armyMovesQueueName, armyMovesRoutingKey, armyMovesExchange)

	err = pubsub.SubscribeJSON(
		conn,
		armyMovesExchange,
		armyMovesQueueName,
		armyMovesRoutingKey,
		pubsub.Transient,
		handlerMove(gs, publishCh),
	)
	if err != nil {
		log.Fatalf("failed to subscribe to army moves queue: %v", err)
	}

	// Subscribe to war messages
	warQueueName := "war"
	warRoutingKey := routing.WarRecognitionsPrefix + ".*"
	warExchange := routing.ExchangePerilTopic

	fmt.Printf("Subscribing to war messages with queue: %s, routing key: %s, exchange: %s\n", 
		warQueueName, warRoutingKey, warExchange)

	err = pubsub.SubscribeJSON(
		conn,
		warExchange,
		warQueueName,
		warRoutingKey,
		pubsub.Durable,
		handlerWar(gs, publishCh),
	)
	if err != nil {
		log.Fatalf("failed to subscribe to war queue: %v", err)
	}

	for {
		words := gamelogic.GetInput()

		if len(words) == 0 {
			continue
		}

		switch words[0] {
		case "move":
			move, err := gs.CommandMove(words)
			if err != nil {
				fmt.Println(err)
				continue
			}

			// Publish the move to army_moves.username routing key
			moveRoutingKey := routing.ArmyMovesPrefix + "." + username
			fmt.Printf("Publishing move to routing key: %s on exchange: %s\n", moveRoutingKey, routing.ExchangePerilTopic)
			err = pubsub.PublishJSON(publishCh, routing.ExchangePerilTopic, moveRoutingKey, move)
			if err != nil {
				fmt.Printf("failed to publish move: %v\n", err)
				continue
			}
			fmt.Println("Move published successfully")

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

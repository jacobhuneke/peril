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

func main() {
	fmt.Println("Starting Peril client...")
	const rabbitConnString = "amqp://guest:guest@localhost:5672/"

	conn, err := amqp.Dial(rabbitConnString)
	if err != nil {
		log.Fatalf("could not connect to RabbitMQ: %v", err)
	}
	defer conn.Close()
	fmt.Println("Peril game client connected to RabbitMQ!")

	publishCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("could not start pub channel %v", err)
	}

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatalf("could not get username: %v", err)
	}
	gs := gamelogic.NewGameState(username)

	queueName := fmt.Sprintf("pause.%v", username)
	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilDirect, queueName, routing.PauseKey, pubsub.SimpleQueueTransient, handlerPause(gs))
	if err != nil {
		log.Fatalf("could not subscribe: %v", err)
	}

	queueName2 := fmt.Sprintf("army_moves.%v", username)
	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, queueName2, "army_moves.*", pubsub.SimpleQueueTransient, handlerMove(gs, publishCh))
	if err != nil {
		log.Fatalf("could not subscribe moves: %v", err)
	}
	warKey := "war.#"
	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix, warKey, pubsub.SimpleQueueDurable, handlerWar(gs, publishCh))
	if err != nil {
		log.Fatalf("could not subscribe war queue: %v", err)
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

			err = pubsub.PublishJSON(publishCh, routing.ExchangePerilTopic, queueName2, move)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			handlerMove(gs, publishCh)
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
			// TODO: publish n malicious logs
			fmt.Println("Spamming not allowed yet!")
		case "quit":
			gamelogic.PrintQuit()
			return
		default:
			fmt.Println("unknown command")
		}
	}
}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {

	return func(ps routing.PlayingState) pubsub.AckType {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}

func handlerMove(gs *gamelogic.GameState, ch *amqp.Channel) func(move gamelogic.ArmyMove) pubsub.AckType {
	return func(move gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(move)
		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			key := fmt.Sprintf("%v.%v", routing.WarRecognitionsPrefix, gs.Player.Username)
			data := gamelogic.RecognitionOfWar{
				Attacker: move.Player,
				Defender: gs.GetPlayerSnap(),
			}
			err := pubsub.PublishJSON(ch, routing.ExchangePerilTopic, key, data)
			if err != nil {
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			fallthrough
		default:
			return pubsub.NackDiscard
		}
	}
}

func handlerWar(gs *gamelogic.GameState, ch *amqp.Channel) func(rw gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(rw gamelogic.RecognitionOfWar) pubsub.AckType {
		defer fmt.Print("> ")
		outcome, winner, loser := gs.HandleWar(rw)
		var msg string
		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeYouWon:
			msg = fmt.Sprintf("%v won a war against %v", winner, loser)
			a := makeLog(msg, gs.Player.Username, ch)
			return a
		case gamelogic.WarOutcomeOpponentWon:
			msg = fmt.Sprintf("%v won a war against %v", winner, loser)
			a := makeLog(msg, gs.Player.Username, ch)
			return a
		case gamelogic.WarOutcomeDraw:
			msg = fmt.Sprintf("A war between %v and %v resulted in a draw", winner, loser)
			a := makeLog(msg, gs.Player.Username, ch)
			return a
		default:
			fmt.Println("error: unexpected war outcome")
			return pubsub.NackDiscard
		}

	}

}

func makeLog(msg, username string, ch *amqp.Channel) pubsub.AckType {
	log := routing.GameLog{
		CurrentTime: time.Time{},
		Message:     msg,
		Username:    username,
	}
	key := fmt.Sprintf("%v.%v", routing.GameLogSlug, username)
	err := pubsub.PublishGob(ch, routing.ExchangePerilTopic, key, log)
	if err != nil {
		return pubsub.NackRequeue
	}
	return pubsub.Ack
}

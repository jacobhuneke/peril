package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril server...")
	connString := "amqp://guest:guest@localhost:5672/"
	connection, err := amqp.Dial(connString)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer connection.Close()
	fmt.Println("Successfully connected")

	channel, err := connection.Channel()
	if err != nil {
		log.Fatal(err.Error())
	}
	exchange := routing.ExchangePerilDirect
	key := routing.PauseKey
	val := routing.PlayingState{
		IsPaused: true,
	}
	err = pubsub.PublishJSON(channel, exchange, key, val)
	if err != nil {
		log.Fatal(err.Error())
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	fmt.Println("Server is shutting down")
}

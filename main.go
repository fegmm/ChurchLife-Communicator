package main

import (
	"context"
	"fmt"

	"github.com/fegmm/ChurchLife-Communicator/bridges"
	"github.com/fegmm/ChurchLife-Communicator/events"
	"github.com/kelseyhightower/envconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Configuration struct {
	Whatsapp bridges.WhatsappConfiguration
	Signal   bridges.SignalConfiguration
	Telegram bridges.TelegramConfiguration
	Mail     bridges.MailConfiguration
	Server   string
}

func main() {
	config := loadConfiguration()
	eventService := getEventService(config)
	toServer := make(chan interface{})
	messanger := loadMessangers(config, toServer)

	handleToServer(eventService, toServer)
	hanldeToMessanger(eventService, messanger)
}

func getEventService(configuration Configuration) events.EventServiceClient {
	conn, err := grpc.Dial(configuration.Server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("Couldn't connect to grpc server: %v", err))
	}
	defer conn.Close()
	eventService := events.NewEventServiceClient(conn)
	return eventService
}

func loadConfiguration() Configuration {
	var configuration Configuration
	err := envconfig.Process("myapp", &configuration)
	if err != nil {
		panic(fmt.Sprintf("Settings could not be read: %v", err))
	}
	return configuration
}

func loadMessangers(config Configuration, toServer chan interface{}) []chan interface{} {
	toMessanger := make([]chan interface{}, 4)

	if config.Mail.Active {
		bridge := bridges.NewMailBridge(config.Mail, toServer)
		toMessanger = append(toMessanger, bridge.ToMessanger)
		go bridge.HandleToMessanger()
	}

	if config.Signal.Active {
		bridge := bridges.NewSignalBridge(config.Signal, toServer)
		toMessanger = append(toMessanger, bridge.ToMessanger)
		go bridge.HandleToMessanger()
		go bridge.HandleToServer()
	}

	if config.Telegram.Active {
		bridge := bridges.NewTelegramBridge(config.Telegram, toServer)
		toMessanger = append(toMessanger, bridge.ToMessanger)
		go bridge.HandleToMessanger()
		go bridge.HandleToServer()
	}

	if config.Whatsapp.Active {
		bridge := bridges.NewWhatsappBridge(config.Whatsapp, toServer)
		toMessanger = append(toMessanger, bridge.ToMessanger)
		go bridge.HandleToMessanger()
		go bridge.HandleToServer()
	}
	return toMessanger
}

func handleToServer(eventService events.EventServiceClient, toServer chan interface{}) {
	for raw_event := range toServer {
		switch event := raw_event.(type) {
		case *events.Reaction:
			_, err := eventService.React(context.Background(), event)
			if err != nil {
				fmt.Printf("%v", err)
			}
		}
	}
}

func hanldeToMessanger(eventService events.EventServiceClient, messangers []chan interface{}) {
	request := &events.GetEventsRequest{EventsSince: timestamppb.Now()}
	stream, err := eventService.GetEvents(context.Background(), request)
	if err != nil {
		panic(fmt.Sprintf("Couldn't make call to get events: %v", err))
	}
	for {
		event, err := stream.Recv()
		if err != nil {
			fmt.Printf("Message could not be received %v", err)
		}

		for _, messanger := range messangers {
			messanger <- event
		}
	}
}

package bridges

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fegmm/ChurchLife-Communicator/events"
	"gitlab.com/signald/signald-go/signald"
	client_protocol "gitlab.com/signald/signald-go/signald/client-protocol"
	v1 "gitlab.com/signald/signald-go/signald/client-protocol/v1"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SignalConfiguration struct {
	Active       bool
	SocketPath   string
	EventGroupId string
	Account      string
	HTMLMessage  string
}

type SignalBridge struct {
	config      SignalConfiguration
	ToMessanger chan interface{}
	ToServer    chan interface{}
	signald     *signald.Signald
	log         waLog.Logger
}

func NewSignalBridge(config SignalConfiguration, toServer chan interface{}) SignalBridge {
	log := waLog.Stdout("Signal", "INFO", true)
	signald := signald.Signald{SocketPath: config.SocketPath}
	err := signald.Connect()
	if err != nil {
		log.Errorf("Could not connect to signald %v", err)
		panic(err)
	}

	return SignalBridge{
		config:      config,
		ToMessanger: make(chan interface{}),
		ToServer:    toServer,
		signald:     &signald,
		log:         log,
	}
}

func (signal *SignalBridge) HandleToMessanger() {
	for {
		raw_message := <-signal.ToMessanger
		switch message := raw_message.(type) {
		case *events.Event:
			event := message
			request := v1.SendRequest{
				Request:          v1.Request{ID: event.Id},
				MessageBody:      fmt.Sprintf(signal.config.HTMLMessage, event.Title, event.Description, event.Location, event.Start, event.End, event.MinParticipants, event.MaxParticipants),
				Previews:         []*v1.JsonPreview{{Title: event.Title, Url: event.Url, Description: event.Description, Attachment: &v1.JsonAttachment{Filename: event.ImageUrl}}},
				RecipientGroupID: signal.config.EventGroupId,
				Timestamp:        event.CreationTimestamp.Seconds,
				Account:          signal.config.Account,
			}
			request.Submit(signal.signald)
		}
	}
}

func (signal *SignalBridge) HandleToServer() {
	c := make(chan client_protocol.BasicResponse)
	signal.signald.Listen(c)

	req := v1.SubscribeRequest{Account: signal.config.Account}
	err := req.Submit(signal.signald)
	if err != nil {
		panic(err)
	}

	for msg := range c {
		if msg.Type != "IncomingMessage" {
			continue
		}
		var data v1.IncomingMessage
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			signal.log.Errorf("Could not unmarshal incoming message %v", err)
		}

		if data.DataMessage == nil {
			continue
		}
		if data.DataMessage.Reaction == nil {
			continue
		}
		reaction := events.Reaction{
			EventId:         data.DataMessage.Quote.Text,
			Emoji:           data.DataMessage.Reaction.Emoji,
			Channel:         events.Channel_SIGNAL,
			UserIdentifier:  data.Source.UUID,
			UserDisplayName: data.Source.Number,
			Timestamp:       timestamppb.New(time.UnixMilli(data.Timestamp)),
			Remove:          data.DataMessage.Reaction.Remove,
		}
		signal.ToServer <- reaction
	}

	go func() {
		for {
			message := <-c
			fmt.Printf("%v", message)
		}
	}()
}

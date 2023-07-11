package bridges

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/fegmm/ChurchLife-Communicator/events"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type WhatsappConfiguration struct {
	Active        bool
	DbPath        string
	EventGroupJID string
	HTMLMessage   string
}

type WhatsappBridge struct {
	Config      WhatsappConfiguration
	whatsapp    *whatsmeow.Client
	log         waLog.Logger
	dbLog       waLog.Logger
	ToMessanger chan interface{}
	ToServer    chan interface{}
}

func NewWhatsappBridge(config WhatsappConfiguration, toServer chan interface{}) WhatsappBridge {
	log := waLog.Stdout("Main", "INFO", true)
	dbLog := waLog.Stdout("Database", "INFO", true)
	dbAddress := "file:" + config.DbPath + "?_foreign_keys=on"

	storeContainer, err := sqlstore.New("sqlite3", dbAddress, dbLog)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}
	device, err := storeContainer.GetFirstDevice()
	if err != nil {
		panic(fmt.Sprintf("Failed to get device: %v", err))
	}
	whatsapp := *whatsmeow.NewClient(device, log)

	qr_c, err := whatsapp.GetQRChannel(context.Background())
	if err != nil {
		// This error means that we're already logged in, so ignore it.
		if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			log.Errorf("Failed to get QR channel: %v", err)
		}
	} else {
		go func() {
			for evt := range qr_c {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				} else {
					log.Infof("QR channel result: %s", evt.Event)
				}
			}
		}()
	}

	c := make(chan interface{})
	whatsapp.AddEventHandler(func(rawEvent interface{}) {
		c <- rawEvent
	})
	err = whatsapp.Connect()
	if err != nil {
		panic(fmt.Sprintf("Failed to connect: %v", err))
	}

	toMessanger := make(chan interface{})
	return WhatsappBridge{
		Config:      config,
		whatsapp:    &whatsapp,
		log:         log,
		dbLog:       dbLog,
		ToMessanger: toMessanger,
		ToServer:    toServer,
	}
}

func (bridge WhatsappBridge) HandleToMessanger() {
	for {
		raw_message := <-bridge.ToMessanger
		switch message := raw_message.(type) {
		case *events.Event:
			event := message
			text := fmt.Sprintf(bridge.Config.HTMLMessage, event.Title, event.Description, event.Location, event.Start, event.End, event.MinParticipants, event.MaxParticipants, event.Url)
			to, err := types.ParseJID(bridge.Config.EventGroupJID)
			if err != nil {
				bridge.log.Errorf("Could not parse JID of EventGroup %v", err)
			}
			thumbnail_resp, err := http.Get(event.ImageUrl)
			if err != nil {
				bridge.log.Warnf("Could not receive thumbnail %v", err)
			}
			thumbnail := make([]byte, thumbnail_resp.ContentLength)
			n, err := thumbnail_resp.Body.Read(thumbnail)
			if err != nil {
				bridge.log.Warnf("Could not receive thumbnail %v", err)
			}
			if n != int(thumbnail_resp.ContentLength) {
				bridge.log.Warnf("Could not read all bytes of thumbnail. Read %d from %d bytes.", n, thumbnail_resp.ContentLength)
			}
			bridge.whatsapp.SendMessage(context.Background(), to, &proto.Message{
				//Conversation: &text,
				ExtendedTextMessage: &proto.ExtendedTextMessage{
					Text:          &text,
					Title:         &event.Title,
					CanonicalUrl:  &event.Url,
					JpegThumbnail: thumbnail,
					Description:   &event.Description,
					MatchedText:   &event.Url,
				},
			})
		}
	}
}

func (bridge WhatsappBridge) HandleToServer() {
	message_handler := func(rawEvent interface{}) {
		switch event := rawEvent.(type) {
		case *waEvents.Message:
			if event.Message.EncReactionMessage != nil {
				decrypted, err := bridge.whatsapp.DecryptReaction(event)
				if err != nil {
					bridge.log.Errorf("Failed to decrypt encrypted reaction: %v", err)
				}
				bridge.ToServer <- events.Reaction{
					EventId:         *event.Message.EncReactionMessage.TargetMessageKey.Id,
					Emoji:           *decrypted.Text,
					Channel:         events.Channel_WHATSAPP,
					UserDisplayName: event.Info.PushName,
					UserIdentifier:  event.Info.Sender.String(),
					Timestamp:       timestamppb.New(time.UnixMilli(*decrypted.SenderTimestampMs)),
				}
			} else if event.Message.ReactionMessage != nil {
				bridge.ToServer <- events.Reaction{
					EventId:         *event.Message.ReactionMessage.Key.Id,
					Emoji:           *event.Message.ReactionMessage.Text,
					Channel:         events.Channel_WHATSAPP,
					UserDisplayName: event.Info.PushName,
					UserIdentifier:  event.Info.Sender.String(),
					Timestamp:       timestamppb.New(time.UnixMilli(*event.Message.ReactionMessage.SenderTimestampMs)),
				}
			}
		}
	}
	bridge.whatsapp.AddEventHandler(message_handler)
}

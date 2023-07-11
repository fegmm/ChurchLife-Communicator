package bridges

import (
	"fmt"

	"github.com/fegmm/ChurchLife-Communicator/events"
	"github.com/nickname76/telegrambot"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TelegramConfiguration struct {
	Active                 bool
	EventGroupId           int64
	MessageTemplate        string
	BotToken               string
	ConfirmationButtonText string
}

type TelegramBridge struct {
	log         waLog.Logger
	config      TelegramConfiguration
	ToMessanger chan interface{}
	ToServer    chan interface{}
	bot         *telegrambot.API
}

func NewTelegramBridge(config TelegramConfiguration, toServer chan interface{}) TelegramBridge {
	log := waLog.Stdout("Telegram", "INFO", true)
	bot, me, err := telegrambot.NewAPI(config.BotToken)
	if err != nil {
		log.Errorf("Could not connect to Telegram Bot API %v", err)
	}
	log.Infof("Authorized on account %s", me.Username)
	return TelegramBridge{
		log:         log,
		config:      config,
		bot:         bot,
		ToMessanger: make(chan interface{}),
		ToServer:    toServer,
	}
}

func (telegram *TelegramBridge) HandleToMessanger() {
	for raw_event := range telegram.ToMessanger {
		switch event := raw_event.(type) {
		case *events.Event:
			_, err := telegram.bot.SendMessage(&telegrambot.SendMessageParams{
				Text:   fmt.Sprintf(telegram.config.MessageTemplate, event.Title, event.Description, event.Location, event.Start, event.End, event.MinParticipants, event.MaxParticipants),
				ChatID: telegrambot.ChatID(telegram.config.EventGroupId),
				ReplyMarkup: &telegrambot.InlineKeyboardMarkup{
					InlineKeyboard: [][]*telegrambot.InlineKeyboardButton{{{
						Text:         telegram.config.ConfirmationButtonText,
						CallbackData: event.Id,
					}}},
				},
			})
			if err != nil {
				telegram.log.Errorf("Failed to send event %v", err)
			}
		}
	}
}

func (telegram *TelegramBridge) HandleToServer() {
	telegrambot.StartReceivingUpdates(telegram.bot, func(update *telegrambot.Update, err error) {
		if update.CallbackQuery != nil {
			if err != nil {
				telegram.log.Errorf("Could not get callback %v", err)
			}
			reaction := events.Reaction{
				EventId:         update.CallbackQuery.Data,
				Emoji:           "👍",
				Channel:         events.Channel_TELEGRAM,
				UserIdentifier:  fmt.Sprintf("%d", update.CallbackQuery.From.ID),
				UserDisplayName: fmt.Sprintf("%s %s (%s)", update.CallbackQuery.From.FirstName, update.CallbackQuery.From.LastName, update.CallbackQuery.From.Username),
				Timestamp:       timestamppb.Now(),
				Remove:          false,
			}
			telegram.ToServer <- reaction
		}
	})
}

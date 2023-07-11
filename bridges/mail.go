package bridges

import (
	"fmt"
	"net/smtp"

	"github.com/fegmm/ChurchLife-Communicator/events"
)

type MailConfiguration struct {
	Active        bool
	SenderAddress string
	SMTPUser      string
	SMTPPassword  string
	SMTPHost      string
	SMTPPort      string
	TargetMail    string
	HTMLMessage   string
}

type MailBridge struct {
	Config      MailConfiguration
	ToMessanger chan interface{}
	ToServer    chan interface{}
}

func NewMailBridge(config MailConfiguration, toServer chan interface{}) MailBridge {
	result := MailBridge{
		Config:      config,
		ToMessanger: make(chan interface{}),
		ToServer:    toServer,
	}
	return result
}

func (mail *MailBridge) HandleToMessanger() {
	for {
		raw_message := <-mail.ToMessanger
		switch message := raw_message.(type) {
		case *events.Event:
			mail.sendEvent(message)
		}
	}
}

func (bridge *MailBridge) sendEvent(event *events.Event) {
	from := bridge.Config.SenderAddress
	password := bridge.Config.SMTPPassword
	to := []string{
		bridge.Config.TargetMail,
	}
	message := []byte(fmt.Sprintf(bridge.Config.HTMLMessage, event.Title, event.Description, event.Location, event.Start, event.End, event.MinParticipants, event.MaxParticipants, event.Url, event.ImageUrl))

	smtpHost := bridge.Config.SMTPHost
	smtpPort := bridge.Config.SMTPPort
	auth := smtp.PlainAuth("", from, password, smtpHost)

	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, to, message)
	if err != nil {
		fmt.Printf("Could not send email: %v", err)
	}
}

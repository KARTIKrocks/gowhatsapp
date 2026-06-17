// Command send demonstrates sending a text message and an OTP auth template.
//
// Set WHATSAPP_PHONE_NUMBER_ID, WHATSAPP_ACCESS_TOKEN, and WHATSAPP_TO, then:
//
//	go run .
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

func main() {
	client, err := whatsapp.New(whatsapp.Config{
		PhoneNumberID: os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
		AccessToken:   os.Getenv("WHATSAPP_ACCESS_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	to := os.Getenv("WHATSAPP_TO") // E.164 without '+', e.g. 15551234567

	// 1) A free-form text (only valid inside an open 24-hour service window).
	if _, err := client.SendText(ctx, to, "Hello from gowhatsapp 👋"); err != nil {
		// Outside the window, expect ErrReengagementRequired — fall back to a template.
		if errors.Is(err, whatsapp.ErrReengagementRequired) {
			log.Println("session window closed; sending a template instead")
		} else {
			log.Printf("send text: %v", err)
		}
	}

	// 2) An authentication template carrying an OTP (you generate the code).
	res, err := client.SendTemplate(ctx, to,
		whatsapp.AuthTemplate("otp_login", "en_US", "472913"))
	if err != nil {
		log.Fatalf("send template: %v", err)
	}
	log.Printf("queued message id=%s wa_id=%s", res.MessageID, res.WAID)
}

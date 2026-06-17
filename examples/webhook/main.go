// Command webhook runs an HTTP server exposing a WhatsApp Cloud API webhook.
//
// Set WHATSAPP_APP_SECRET and WHATSAPP_VERIFY_TOKEN, then:
//
//	go run .   # listens on :8080, mount /webhook in the Meta App dashboard
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

func main() {
	handler := whatsapp.NewWebhookHandler(
		whatsapp.WebhookConfig{
			AppSecret:   os.Getenv("WHATSAPP_APP_SECRET"),
			VerifyToken: os.Getenv("WHATSAPP_VERIFY_TOKEN"),
		},
		whatsapp.Handlers{
			OnStatus: func(_ context.Context, s whatsapp.MessageStatus) {
				log.Printf("status: %s -> %s (conv=%s)", s.MessageID, s.Status, s.ConversationID)
			},
			OnMessage: func(_ context.Context, m whatsapp.InboundMessage) {
				log.Printf("inbound from %s [%s]: %s", m.From, m.Type, m.Text)
			},
		},
	)

	http.Handle("/webhook", handler)
	log.Println("listening on :8080 (GET/POST /webhook)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

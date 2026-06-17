# gowhatsapp

[![Go Reference](https://pkg.go.dev/badge/github.com/KARTIKrocks/gowhatsapp.svg)](https://pkg.go.dev/github.com/KARTIKrocks/gowhatsapp)
[![Go Report Card](https://goreportcard.com/badge/github.com/KARTIKrocks/gowhatsapp)](https://goreportcard.com/report/github.com/KARTIKrocks/gowhatsapp)
[![Go Version](https://img.shields.io/github/go-mod/go-version/KARTIKrocks/gowhatsapp)](go.mod)
[![CI](https://github.com/KARTIKrocks/gowhatsapp/actions/workflows/ci.yml/badge.svg)](https://github.com/KARTIKrocks/gowhatsapp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A clean, dependency-free Go client for the **WhatsApp Cloud API** (Meta Graph
API). Send text, media, location, reactions, and approved templates (including
**authentication templates for OTP delivery**), and receive webhooks (delivery
statuses + inbound messages) with signature verification.

- **Zero third-party dependencies** — standard library only.
- **Targets Meta Cloud API directly** — not a BSP wrapper.
- **Typed, matchable errors** — `errors.Is` + `*APIError` with `fbtrace_id`.
- **Hermetic tests** — a built-in `MockTransport` captures exact payloads.
- **Future-proof** — pinned/overridable API version, swappable transport, sealed
  message types. See [`DESIGN.md`](./DESIGN.md).

> **OTP note:** Meta only _delivers_ the code; it does not generate or verify
> it. Keep generation + verification in your app. `AuthTemplate` formats the
> delivery.

## Installation

```bash
go get github.com/KARTIKrocks/gowhatsapp
```

Requires Go 1.22+.

## Quick start

```go
package main

import (
	"context"
	"log"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

func main() {
	client, err := whatsapp.New(whatsapp.Config{
		PhoneNumberID: "1234567890",   // Cloud API phone number ID
		AccessToken:   "EAAG...",      // system-user access token
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Free-form text — only inside an open 24-hour service window.
	if _, err := client.SendText(ctx, "15551234567", "Hello from gowhatsapp!"); err != nil {
		log.Fatal(err)
	}

	// Approved template — to start a conversation or message outside the window.
	res, err := client.SendTemplate(ctx, "15551234567",
		whatsapp.AuthTemplate("otp_login", "en_US", "472913"))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("queued message %s", res.MessageID)
}
```

## Messages

```go
client.Send(ctx, to, whatsapp.Text{Body: "hi", PreviewURL: true})
client.Send(ctx, to, whatsapp.ImageByLink("https://…/p.jpg", "caption"))
client.Send(ctx, to, whatsapp.DocumentByLink("https://…/d.pdf", "invoice.pdf", ""))
client.Send(ctx, to, whatsapp.Location{Latitude: 12.97, Longitude: 77.59, Name: "HQ"})
client.Send(ctx, to, whatsapp.Reaction{MessageID: "wamid.X", Emoji: "👍"})

// Reply to a received message, quoting it in the chat:
client.SendReply(ctx, to, m.ID, whatsapp.Text{Body: "got it 👍"}) // m is an InboundMessage

// Interactive reply buttons (up to 3):
client.Send(ctx, to, whatsapp.InteractiveButtons{
	Body: "Confirm your order?",
	Buttons: []whatsapp.Button{
		{ID: "confirm", Title: "Confirm"},
		{ID: "cancel", Title: "Cancel"},
	},
})

// Interactive list menu:
client.Send(ctx, to, whatsapp.InteractiveList{
	Body: "Choose a plan", ButtonText: "View plans",
	Sections: []whatsapp.ListSection{{
		Title: "Plans",
		Rows: []whatsapp.ListRow{
			{ID: "basic", Title: "Basic", Description: "$5/mo"},
			{ID: "pro", Title: "Pro", Description: "$20/mo"},
		},
	}},
})

// Template with body parameters:
client.SendTemplate(ctx, to, whatsapp.TemplateMessage{
	Name:       "match_mutual",
	Language:   "en_US",
	Components:  []whatsapp.TemplateComponent{whatsapp.BodyComponent("Asha", "Bengaluru")},
})
```

## Error handling

```go
_, err := client.SendTemplate(ctx, to, tmpl)
switch {
case errors.Is(err, whatsapp.ErrReengagementRequired): // 24h window closed
case errors.Is(err, whatsapp.ErrRateLimited):          // back off; Retry-After honored
case errors.Is(err, whatsapp.ErrTemplateNotApproved):  // fix/await template
}

var apiErr *whatsapp.APIError
if errors.As(err, &apiErr) {
	log.Printf("code=%d trace=%s", apiErr.Code, apiErr.FBTraceID)
}
```

## Webhooks

```go
http.Handle("/webhook/whatsapp", whatsapp.NewWebhookHandler(
	whatsapp.WebhookConfig{AppSecret: appSecret, VerifyToken: verifyToken},
	whatsapp.Handlers{
		OnStatus: func(ctx context.Context, s whatsapp.MessageStatus) {
			log.Printf("%s -> %s", s.MessageID, s.Status)
		},
		OnMessage: func(ctx context.Context, m whatsapp.InboundMessage) {
			log.Printf("from %s: %s", m.From, m.Text)
		},
	},
))
```

The handler answers the GET subscription challenge, verifies the
`X-Hub-Signature-256` HMAC on POSTs, and always replies `200` to well-formed
payloads (hand off durably in your callbacks and return quickly).

Acknowledge an inbound message (the blue ticks), or show a typing indicator
while you prepare a reply (which also marks it read):

```go
client.MarkRead(ctx, m.ID)    // m is the InboundMessage from OnMessage
client.SendTyping(ctx, m.ID)  // shows "typing…" for ~25s
```

Inbound messages are parsed into typed sub-fields by `Type` — tap/selection
replies, media, location, reactions, and the replied-to message ID:

```go
OnMessage: func(ctx context.Context, m whatsapp.InboundMessage) {
	switch {
	case m.Interactive != nil: // button/list reply
		log.Printf("chose %s (%s)", m.Interactive.ID, m.Interactive.Title)
	case m.Media != nil:       // image/video/audio/document/sticker
		data, mime, _ := client.Download(ctx, m.Media.ID)
		_ = data; _ = mime
	case m.Location != nil:
		log.Printf("at %f,%f", m.Location.Latitude, m.Location.Longitude)
	default:
		log.Printf("text: %s", m.Text)
	}
}
```

## Media upload & download

When you can't host a public URL, upload bytes to get a reusable media ID, and
download media users send you:

```go
id, _ := client.Upload(ctx, "invoice.pdf", "application/pdf", file) // file is an io.Reader
client.Send(ctx, to, whatsapp.DocumentByID(id, "invoice.pdf", ""))

data, mime, _ := client.Download(ctx, m.Media.ID) // m.Media from an inbound message
```

## Testing your code

Inject `MockTransport` to assert what _would_ be sent — no network:

```go
mt := whatsapp.NewMockTransport()
client, _ := whatsapp.New(cfg, whatsapp.WithHTTPClient(mt))

_, _ = client.SendText(ctx, "15551234567", "hi")

req, _ := mt.LastRequest()
// req.Body["type"] == "text"

// Simulate an API error:
mt.WithResponse(429, `{"error":{"code":131056,"message":"rate limited"}}`)
```

## Configuration

| Option                     | Purpose                                             |
| -------------------------- | --------------------------------------------------- |
| `WithHTTPClient(Doer)`     | inject `*http.Client` / custom round-tripper / mock |
| `WithAPIVersion(string)`   | override `DefaultAPIVersion` (e.g. `"v22.0"`)       |
| `WithBaseURL(string)`      | override the Graph host (proxy/sandbox/test)        |
| `WithRetry(max, base)`     | retries for 429/5xx (default 2, 500ms, exp backoff) |
| `WithLogger(*slog.Logger)` | structured logging                                  |

## Status & roadmap

Phase 1 (send + webhooks) and Phase 2 (interactive messages, rich inbound
parsing, media upload/download, typing indicator) are implemented. Template
management and Flows are planned — see [`DESIGN.md`](./DESIGN.md). API may change
during `v0.x`; see [`CHANGELOG.md`](./CHANGELOG.md).

## License

[MIT](./LICENSE) © KARTIKrocks

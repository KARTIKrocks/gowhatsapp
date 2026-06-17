// Package whatsapp is a Go client for the WhatsApp Cloud API (Meta Graph API).
//
// It sends messages — text, media, location, reactions, and approved
// templates (including authentication templates for OTP delivery) — and parses
// inbound webhooks (delivery statuses and user messages) with signature
// verification. The library targets Meta's Cloud API directly; it is not a BSP
// wrapper and has zero third-party dependencies.
//
// # Design
//
// WhatsApp Cloud is a single vendor, so rather than the provider-per-module
// shape of a multi-vendor library, gowhatsapp keeps one module and makes the
// future-proofing axes explicit:
//
//   - The HTTP boundary is a swappable [Doer] seam — inject any *http.Client,
//     a custom round-tripper, or [MockTransport] in tests.
//   - The Graph API version is pinned in [DefaultAPIVersion] and overridable
//     per client, isolating callers from Meta's quarterly version churn.
//   - [Message] is a sealed interface, so new message types are additive and
//     never break existing callers.
//
// # Quick start
//
//	client, err := whatsapp.New(whatsapp.Config{
//	    PhoneNumberID: "1234567890",
//	    AccessToken:   "EAAG...",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Inside an open 24-hour service window: free-form text.
//	_, err = client.SendText(ctx, "15551234567", "Hello from gowhatsapp!")
//
//	// To start a conversation or send outside the window: an approved template.
//	_, err = client.SendTemplate(ctx, "15551234567",
//	    whatsapp.AuthTemplate("otp_login", "en_US", "472913"))
//
// # OTP delivery
//
// [AuthTemplate] formats an authentication-category template carrying a code.
// Meta only delivers the code — it does not generate or verify it. Keep code
// generation and verification in your application.
//
// # Webhooks
//
// Mount [NewWebhookHandler] to receive delivery statuses and inbound messages;
// it handles the subscription challenge and verifies X-Hub-Signature-256.
//
//	http.Handle("/webhook/whatsapp", whatsapp.NewWebhookHandler(
//	    whatsapp.WebhookConfig{AppSecret: appSecret, VerifyToken: verifyToken},
//	    whatsapp.Handlers{
//	        OnStatus:  func(ctx context.Context, s whatsapp.MessageStatus) { /* ... */ },
//	        OnMessage: func(ctx context.Context, m whatsapp.InboundMessage) { /* ... */ },
//	    },
//	))
package whatsapp

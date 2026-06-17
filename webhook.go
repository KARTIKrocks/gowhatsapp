package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// signatureHeader is the header Meta signs webhook payloads with.
const signatureHeader = "X-Hub-Signature-256"

// maxWebhookBody caps the request body read by the handler (1 MiB).
const maxWebhookBody = 1 << 20

// WebhookConfig configures webhook verification.
type WebhookConfig struct {
	// AppSecret verifies the X-Hub-Signature-256 payload signature. When empty,
	// signature verification is skipped (not recommended outside tests).
	AppSecret string
	// VerifyToken is matched during the GET subscription handshake.
	VerifyToken string
	// Logger records unparseable payloads (which are answered with 200 to avoid
	// pointless Meta redelivery). Defaults to slog.Default() when nil.
	Logger *slog.Logger
}

// Handlers carries the per-event callbacks invoked by the webhook handler. Any
// nil callback is skipped, so you only wire up what you need.
type Handlers struct {
	// OnStatus fires for each delivery status update (sent/delivered/read/failed).
	OnStatus func(ctx context.Context, s MessageStatus)
	// OnMessage fires for each inbound message from a user.
	OnMessage func(ctx context.Context, m InboundMessage)
}

// WebhookEvent is a parsed webhook notification, flattened across the nested
// entry/changes envelope into the two lists callers actually care about.
type WebhookEvent struct {
	Object   string
	Metadata WebhookMetadata
	Statuses []MessageStatus
	Messages []InboundMessage
}

// WebhookMetadata identifies the receiving number.
type WebhookMetadata struct {
	DisplayPhoneNumber string
	PhoneNumberID      string
}

// MessageStatus is an outbound message delivery status update.
type MessageStatus struct {
	// MessageID is the wamid... of the message this status refers to.
	MessageID string
	// Status is "sent", "delivered", "read", or "failed".
	Status string
	// Timestamp is the Unix timestamp string from the event.
	Timestamp string
	// RecipientID is the recipient's wa_id.
	RecipientID string
	// ConversationID identifies the billing conversation, when present.
	ConversationID string
	// Category is the conversation origin category (e.g. "marketing",
	// "utility", "authentication", "service").
	Category string
	// Errors carries failure detail when Status == "failed".
	Errors []APIError
}

// InboundMessage is a message received from a user. Beyond the always-present
// envelope fields (From, ID, Timestamp, Type), the typed sub-fields are
// populated according to Type: Text for "text"; Interactive for "interactive"
// (button/list replies); Media for "image"/"video"/"audio"/"document"/
// "sticker"; Location for "location"; Reaction for "reaction". Unmodeled types
// are still available via Raw.
type InboundMessage struct {
	// From is the sender's wa_id.
	From string
	// ID is the wamid... of the inbound message.
	ID string
	// Timestamp is the Unix timestamp string from the event.
	Timestamp string
	// Type is the message type ("text", "image", "interactive", "location", ...).
	Type string
	// Text is the body for text messages.
	Text string
	// ButtonText is the label for legacy quick-reply (template) button replies.
	ButtonText string
	// ButtonPayload is the payload for legacy quick-reply (template) replies.
	ButtonPayload string
	// ContextID is the wamid... of the message this one replies to, when the
	// user replied to (quoted) a previous message.
	ContextID string
	// Interactive is set when the user tapped a reply button or picked a list
	// row from an InteractiveButtons / InteractiveList message.
	Interactive *InteractiveReply
	// Media is set for image, video, audio, document, and sticker messages.
	Media *InboundMedia
	// Location is set for location messages.
	Location *InboundLocation
	// Reaction is set for reaction messages.
	Reaction *InboundReaction
	// Raw is the unparsed message object for types this struct does not model.
	Raw json.RawMessage
}

// InteractiveReply is a user's selection from an interactive message. Kind is
// "button_reply" or "list_reply"; ID is the developer-assigned identifier set
// when sending; Title is the visible label the user saw.
type InteractiveReply struct {
	Kind  string
	ID    string
	Title string
}

// InboundMedia identifies a received media asset. Use [Client.Download] with ID
// to fetch the bytes.
type InboundMedia struct {
	ID       string
	MimeType string
	SHA256   string
	Caption  string
	Filename string // documents only
	Voice    bool   // audio recorded as a voice note
}

// InboundLocation is a received location pin.
type InboundLocation struct {
	Latitude  float64
	Longitude float64
	Name      string
	Address   string
}

// InboundReaction is a received reaction to one of your messages. Emoji is
// empty when the user removed their reaction.
type InboundReaction struct {
	MessageID string
	Emoji     string
}

// VerifySignature checks an X-Hub-Signature-256 header against the raw body
// using HMAC-SHA256 keyed by appSecret. It is constant-time.
func VerifySignature(appSecret string, body []byte, header string) error {
	if appSecret == "" {
		return fmt.Errorf("%w: app secret not configured", ErrInvalidConfig)
	}
	got := strings.TrimPrefix(header, "sha256=")
	if got == "" || got == header {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(got)) {
		return ErrInvalidSignature
	}
	return nil
}

// ParseEvent flattens a raw webhook body into a WebhookEvent.
func ParseEvent(body []byte) (*WebhookEvent, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("whatsapp: parse webhook: %w", err)
	}
	ev := &WebhookEvent{Object: p.Object}
	for _, entry := range p.Entry {
		for _, ch := range entry.Changes {
			v := ch.Value
			if v.Metadata.PhoneNumberID != "" {
				ev.Metadata = WebhookMetadata{
					DisplayPhoneNumber: v.Metadata.DisplayPhoneNumber,
					PhoneNumberID:      v.Metadata.PhoneNumberID,
				}
			}
			for _, s := range v.Statuses {
				ms := MessageStatus{
					MessageID:      s.ID,
					Status:         s.Status,
					Timestamp:      s.Timestamp,
					RecipientID:    s.RecipientID,
					ConversationID: s.Conversation.ID,
					Category:       s.Conversation.Origin.Type,
				}
				for _, e := range s.Errors {
					ms.Errors = append(ms.Errors, APIError{
						Code:    e.Code,
						Title:   e.Title,
						Message: e.Message,
						Details: e.ErrorData.Details,
					})
				}
				ev.Statuses = append(ev.Statuses, ms)
			}
			for _, raw := range v.Messages {
				var m inboundMessageJSON
				if err := json.Unmarshal(raw, &m); err != nil {
					return nil, fmt.Errorf("whatsapp: parse inbound message: %w", err)
				}
				ev.Messages = append(ev.Messages, m.toInbound(raw))
			}
		}
	}
	return ev, nil
}

// NewWebhookHandler returns an http.Handler implementing the full Cloud API
// webhook contract:
//
//   - GET  verifies the subscription challenge (hub.mode/hub.verify_token →
//     echoes hub.challenge).
//   - POST verifies the signature (when AppSecret is set), parses the payload,
//     and dispatches each status/message to the matching callback.
//
// It always replies 200 to a signature-valid POST — even when the body cannot
// be parsed — so Meta does not redeliver a payload that would never parse
// anyway; the only rejections are a failed GET handshake (403) and a bad
// signature (401). Process callbacks should hand off durably (queue/store) and
// return quickly.
func NewWebhookHandler(cfg WebhookConfig, h Handlers) http.Handler {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			if cfg.VerifyToken != "" &&
				q.Get("hub.mode") == "subscribe" &&
				q.Get("hub.verify_token") == cfg.VerifyToken {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, q.Get("hub.challenge"))
				return
			}
			http.Error(w, "verification failed", http.StatusForbidden)

		case http.MethodPost:
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if cfg.AppSecret != "" {
				if err := VerifySignature(cfg.AppSecret, body, r.Header.Get(signatureHeader)); err != nil {
					http.Error(w, "invalid signature", http.StatusUnauthorized)
					return
				}
			}
			ev, err := ParseEvent(body)
			if err != nil {
				// A malformed payload won't parse on redelivery either, so 200
				// it (the documented contract) rather than invite retry storms.
				log.Warn("whatsapp: dropping unparseable webhook payload", "error", err)
				w.WriteHeader(http.StatusOK)
				return
			}
			ctx := r.Context()
			if h.OnStatus != nil {
				for _, s := range ev.Statuses {
					h.OnStatus(ctx, s)
				}
			}
			if h.OnMessage != nil {
				for _, m := range ev.Messages {
					h.OnMessage(ctx, m)
				}
			}
			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// webhookPayload mirrors the raw Cloud API webhook envelope.
type webhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Metadata struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Statuses []struct {
					ID           string `json:"id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					RecipientID  string `json:"recipient_id"`
					Conversation struct {
						ID     string `json:"id"`
						Origin struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation"`
					Errors []struct {
						Code      int    `json:"code"`
						Title     string `json:"title"`
						Message   string `json:"message"`
						ErrorData struct {
							Details string `json:"details"`
						} `json:"error_data"`
					} `json:"errors"`
				} `json:"statuses"`
				Messages []json.RawMessage `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// inboundMessageJSON are the inbound-message fields this library models; the
// full raw object is preserved on InboundMessage.Raw for everything else.
type inboundMessageJSON struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Button struct {
		Text    string `json:"text"`
		Payload string `json:"payload"`
	} `json:"button"`
	Context struct {
		ID string `json:"id"`
	} `json:"context"`
	Interactive struct {
		Type        string         `json:"type"`
		ButtonReply interactiveSel `json:"button_reply"`
		ListReply   interactiveSel `json:"list_reply"`
	} `json:"interactive"`
	Image    *mediaJSON `json:"image"`
	Video    *mediaJSON `json:"video"`
	Audio    *mediaJSON `json:"audio"`
	Document *mediaJSON `json:"document"`
	Sticker  *mediaJSON `json:"sticker"`
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	} `json:"location"`
	Reaction *struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	} `json:"reaction"`
}

type interactiveSel struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type mediaJSON struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Voice    bool   `json:"voice"`
}

// toInbound maps the parsed JSON onto the public InboundMessage, populating the
// typed sub-field that matches the message Type.
func (m inboundMessageJSON) toInbound(raw json.RawMessage) InboundMessage {
	im := InboundMessage{
		From:          m.From,
		ID:            m.ID,
		Timestamp:     m.Timestamp,
		Type:          m.Type,
		Text:          m.Text.Body,
		ButtonText:    m.Button.Text,
		ButtonPayload: m.Button.Payload,
		ContextID:     m.Context.ID,
		Raw:           raw,
	}

	switch m.Type {
	case "interactive":
		sel := m.Interactive.ButtonReply
		if m.Interactive.Type == "list_reply" {
			sel = m.Interactive.ListReply
		}
		im.Interactive = &InteractiveReply{
			Kind:  m.Interactive.Type,
			ID:    sel.ID,
			Title: sel.Title,
		}
	case "location":
		if m.Location != nil {
			im.Location = &InboundLocation{
				Latitude:  m.Location.Latitude,
				Longitude: m.Location.Longitude,
				Name:      m.Location.Name,
				Address:   m.Location.Address,
			}
		}
	case "reaction":
		if m.Reaction != nil {
			im.Reaction = &InboundReaction{
				MessageID: m.Reaction.MessageID,
				Emoji:     m.Reaction.Emoji,
			}
		}
	default:
		if md := m.mediaFor(m.Type); md != nil {
			im.Media = &InboundMedia{
				ID:       md.ID,
				MimeType: md.MimeType,
				SHA256:   md.SHA256,
				Caption:  md.Caption,
				Filename: md.Filename,
				Voice:    md.Voice,
			}
		}
	}
	return im
}

// mediaFor returns the media object for a media message type, or nil.
func (m inboundMessageJSON) mediaFor(typ string) *mediaJSON {
	switch typ {
	case "image":
		return m.Image
	case "video":
		return m.Video
	case "audio":
		return m.Audio
	case "document":
		return m.Document
	case "sticker":
		return m.Sticker
	default:
		return nil
	}
}

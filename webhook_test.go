package whatsapp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

const sampleWebhook = `{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "WABA",
    "changes": [{
      "field": "messages",
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {"display_phone_number": "15550000000", "phone_number_id": "PNID"},
        "statuses": [{
          "id": "wamid.S1",
          "status": "delivered",
          "timestamp": "1700000000",
          "recipient_id": "15551234567",
          "conversation": {"id": "conv1", "origin": {"type": "utility"}}
        }],
        "messages": [{
          "from": "15551234567",
          "id": "wamid.M1",
          "timestamp": "1700000001",
          "type": "text",
          "text": {"body": "hello back"}
        }]
      }
    }]
  }]
}`

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte("payload")
	valid := sign("secret", "payload")

	if err := whatsapp.VerifySignature("secret", body, valid); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := whatsapp.VerifySignature("secret", body, sign("wrong", "payload")); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("wrong key: got %v, want ErrInvalidSignature", err)
	}
	if err := whatsapp.VerifySignature("secret", body, "garbage"); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("malformed header: got %v, want ErrInvalidSignature", err)
	}
	if err := whatsapp.VerifySignature("", body, valid); !errors.Is(err, whatsapp.ErrInvalidConfig) {
		t.Fatalf("empty secret: got %v, want ErrInvalidConfig", err)
	}
}

func TestParseEvent(t *testing.T) {
	ev, err := whatsapp.ParseEvent([]byte(sampleWebhook))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if ev.Metadata.PhoneNumberID != "PNID" {
		t.Fatalf("metadata = %+v", ev.Metadata)
	}
	if len(ev.Statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(ev.Statuses))
	}
	s := ev.Statuses[0]
	if s.MessageID != "wamid.S1" || s.Status != "delivered" || s.ConversationID != "conv1" || s.Category != "utility" {
		t.Fatalf("status = %+v", s)
	}
	if len(ev.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(ev.Messages))
	}
	m := ev.Messages[0]
	if m.From != "15551234567" || m.Text != "hello back" || m.Type != "text" {
		t.Fatalf("message = %+v", m)
	}
	if len(m.Raw) == 0 {
		t.Fatal("Raw not preserved")
	}
}

const richWebhook = `{
  "object": "whatsapp_business_account",
  "entry": [{
    "changes": [{
      "field": "messages",
      "value": {
        "metadata": {"phone_number_id": "PNID"},
        "messages": [
          {
            "from": "1555", "id": "wamid.A", "timestamp": "1", "type": "interactive",
            "context": {"id": "wamid.PREV"},
            "interactive": {"type": "list_reply", "list_reply": {"id": "opt2", "title": "Option 2"}}
          },
          {
            "from": "1555", "id": "wamid.B", "timestamp": "2", "type": "image",
            "image": {"id": "MID9", "mime_type": "image/jpeg", "sha256": "xx", "caption": "look"}
          },
          {
            "from": "1555", "id": "wamid.C", "timestamp": "3", "type": "location",
            "location": {"latitude": 12.97, "longitude": 77.59, "name": "HQ"}
          },
          {
            "from": "1555", "id": "wamid.D", "timestamp": "4", "type": "reaction",
            "reaction": {"message_id": "wamid.X", "emoji": "👍"}
          }
        ]
      }
    }]
  }]
}`

func TestParseEvent_RichTypes(t *testing.T) {
	ev, err := whatsapp.ParseEvent([]byte(richWebhook))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if len(ev.Messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(ev.Messages))
	}

	// Each case checks the typed sub-field populated for one message type.
	// The checks are top-level funcs so no single one accumulates the whole
	// matrix's branches (gocyclo counts nested closures toward their parent).
	cases := []struct {
		name  string
		check func(t *testing.T, m whatsapp.InboundMessage)
	}{
		{"interactive", checkInteractive},
		{"image", checkImage},
		{"location", checkLocation},
		{"reaction", checkReaction},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, ev.Messages[i])
		})
	}
}

func checkInteractive(t *testing.T, m whatsapp.InboundMessage) {
	if m.ContextID != "wamid.PREV" {
		t.Fatalf("ContextID = %q, want wamid.PREV", m.ContextID)
	}
	if m.Interactive == nil || m.Interactive.Kind != "list_reply" ||
		m.Interactive.ID != "opt2" || m.Interactive.Title != "Option 2" {
		t.Fatalf("Interactive = %+v", m.Interactive)
	}
}

func checkImage(t *testing.T, m whatsapp.InboundMessage) {
	if m.Media == nil || m.Media.ID != "MID9" || m.Media.MimeType != "image/jpeg" || m.Media.Caption != "look" {
		t.Fatalf("Media = %+v", m.Media)
	}
}

func checkLocation(t *testing.T, m whatsapp.InboundMessage) {
	if m.Location == nil || m.Location.Latitude != 12.97 || m.Location.Name != "HQ" {
		t.Fatalf("Location = %+v", m.Location)
	}
}

func checkReaction(t *testing.T, m whatsapp.InboundMessage) {
	if m.Reaction == nil || m.Reaction.MessageID != "wamid.X" || m.Reaction.Emoji != "👍" {
		t.Fatalf("Reaction = %+v", m.Reaction)
	}
}

func TestWebhookHandler_GETVerification(t *testing.T) {
	h := whatsapp.NewWebhookHandler(whatsapp.WebhookConfig{VerifyToken: "vtok"}, whatsapp.Handlers{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/wh?hub.mode=subscribe&hub.verify_token=vtok&hub.challenge=CHAL", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "CHAL" {
		t.Fatalf("verify ok: code=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/wh?hub.mode=subscribe&hub.verify_token=WRONG&hub.challenge=CHAL", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad token should be 403, got %d", rec.Code)
	}
}

func TestWebhookHandler_POSTDispatch(t *testing.T) {
	var gotStatus whatsapp.MessageStatus
	var gotMessage whatsapp.InboundMessage
	h := whatsapp.NewWebhookHandler(
		whatsapp.WebhookConfig{AppSecret: "secret", VerifyToken: "vtok"},
		whatsapp.Handlers{
			OnStatus:  func(_ context.Context, s whatsapp.MessageStatus) { gotStatus = s },
			OnMessage: func(_ context.Context, m whatsapp.InboundMessage) { gotMessage = m },
		},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(sampleWebhook))
	req.Header.Set("X-Hub-Signature-256", sign("secret", sampleWebhook))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if gotStatus.MessageID != "wamid.S1" {
		t.Fatalf("OnStatus not dispatched: %+v", gotStatus)
	}
	if gotMessage.Text != "hello back" {
		t.Fatalf("OnMessage not dispatched: %+v", gotMessage)
	}
}

func TestWebhookHandler_POSTBadSignature(t *testing.T) {
	called := false
	h := whatsapp.NewWebhookHandler(
		whatsapp.WebhookConfig{AppSecret: "secret"},
		whatsapp.Handlers{OnStatus: func(context.Context, whatsapp.MessageStatus) { called = true }},
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(sampleWebhook))
	req.Header.Set("X-Hub-Signature-256", sign("wrong", sampleWebhook))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("handler invoked despite bad signature")
	}
}

func TestWebhookHandler_UnparseablePayloadReplies200(t *testing.T) {
	// A signature-valid but malformed body must get 200 (so Meta does not
	// redeliver a payload that would never parse), not 400.
	h := whatsapp.NewWebhookHandler(
		whatsapp.WebhookConfig{AppSecret: "secret"},
		whatsapp.Handlers{},
	)
	rec := httptest.NewRecorder()
	body := "{not json"
	req := httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign("secret", body))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 for unparseable payload", rec.Code)
	}
}

func TestWebhookHandler_RejectsOtherMethods(t *testing.T) {
	h := whatsapp.NewWebhookHandler(whatsapp.WebhookConfig{}, whatsapp.Handlers{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/wh", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code = %d, want 405", rec.Code)
	}
	_, _ = io.Copy(io.Discard, rec.Body)
}

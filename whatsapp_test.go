package whatsapp_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

func newTestClient(t *testing.T, mt *whatsapp.MockTransport, opts ...whatsapp.Option) *whatsapp.Client {
	t.Helper()
	all := append([]whatsapp.Option{whatsapp.WithHTTPClient(mt)}, opts...)
	c, err := whatsapp.New(whatsapp.Config{
		PhoneNumberID: "PNID",
		AccessToken:   "TOKEN",
	}, all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNew_RequiresCredentials(t *testing.T) {
	if _, err := whatsapp.New(whatsapp.Config{AccessToken: "x"}); !errors.Is(err, whatsapp.ErrInvalidConfig) {
		t.Fatalf("missing phone number ID: got %v, want ErrInvalidConfig", err)
	}
	if _, err := whatsapp.New(whatsapp.Config{PhoneNumberID: "x"}); !errors.Is(err, whatsapp.ErrInvalidConfig) {
		t.Fatalf("missing token: got %v, want ErrInvalidConfig", err)
	}
}

func TestSendText_BuildsRequest(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	res, err := c.SendText(context.Background(), "15551234567", "hello")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if res.MessageID != "wamid.MOCK" {
		t.Fatalf("MessageID = %q, want wamid.MOCK", res.MessageID)
	}

	req, ok := mt.LastRequest()
	if !ok {
		t.Fatal("no request captured")
	}
	// URL must embed the default API version and phone number ID.
	if !strings.HasSuffix(req.URL, "/"+whatsapp.DefaultAPIVersion+"/PNID/messages") {
		t.Fatalf("URL = %q, want .../%s/PNID/messages", req.URL, whatsapp.DefaultAPIVersion)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer TOKEN" {
		t.Fatalf("Authorization = %q, want Bearer TOKEN", got)
	}
	if req.Body["type"] != "text" || req.Body["to"] != "15551234567" {
		t.Fatalf("payload = %#v", req.Body)
	}
	text, _ := req.Body["text"].(map[string]any)
	if text["body"] != "hello" {
		t.Fatalf("text.body = %v, want hello", text["body"])
	}
}

func TestSendReply_AddsContext(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	if _, err := c.SendReply(context.Background(), "15551234567", "wamid.IN", whatsapp.Text{Body: "hi"}); err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	req, ok := mt.LastRequest()
	if !ok {
		t.Fatal("no request captured")
	}
	ctxObj, _ := req.Body["context"].(map[string]any)
	if ctxObj["message_id"] != "wamid.IN" {
		t.Fatalf("context = %#v, want message_id=wamid.IN", req.Body["context"])
	}
	if req.Body["type"] != "text" {
		t.Fatalf("type = %v, want text", req.Body["type"])
	}
}

func TestSendReply_Validation(t *testing.T) {
	c := newTestClient(t, whatsapp.NewMockTransport())
	if _, err := c.SendReply(context.Background(), "", "wamid.IN", whatsapp.Text{Body: "x"}); !errors.Is(err, whatsapp.ErrInvalidRecipient) {
		t.Fatalf("empty recipient: got %v, want ErrInvalidRecipient", err)
	}
	if _, err := c.SendReply(context.Background(), "1555", "", whatsapp.Text{Body: "x"}); !errors.Is(err, whatsapp.ErrInvalidMessage) {
		t.Fatalf("empty reply-to: got %v, want ErrInvalidMessage", err)
	}
	if _, err := c.SendReply(context.Background(), "1555", "wamid.IN", nil); !errors.Is(err, whatsapp.ErrInvalidMessage) {
		t.Fatalf("nil message: got %v, want ErrInvalidMessage", err)
	}
}

func TestSend_RejectsEmptyRecipientAndNilMessage(t *testing.T) {
	c := newTestClient(t, whatsapp.NewMockTransport())
	if _, err := c.SendText(context.Background(), "", "x"); !errors.Is(err, whatsapp.ErrInvalidRecipient) {
		t.Fatalf("empty recipient: got %v, want ErrInvalidRecipient", err)
	}
	if _, err := c.Send(context.Background(), "1555", nil); !errors.Is(err, whatsapp.ErrInvalidMessage) {
		t.Fatalf("nil message: got %v, want ErrInvalidMessage", err)
	}
}

func TestSend_NormalizesRecipient(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	// A leading "+" and surrounding whitespace are stripped before sending.
	if _, err := c.SendText(context.Background(), "  +15551234567 ", "hi"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	req, _ := mt.LastRequest()
	if req.Body["to"] != "15551234567" {
		t.Fatalf("to = %v, want 15551234567 (normalized)", req.Body["to"])
	}

	// A non-digit recipient is rejected up front, before any request.
	if _, err := c.SendText(context.Background(), "+1 (555) 123", "hi"); !errors.Is(err, whatsapp.ErrInvalidRecipient) {
		t.Fatalf("non-digit recipient: got %v, want ErrInvalidRecipient", err)
	}
	// Only "+"/whitespace means no digits at all -> empty recipient.
	if _, err := c.SendText(context.Background(), " + ", "hi"); !errors.Is(err, whatsapp.ErrInvalidRecipient) {
		t.Fatalf("bare plus: got %v, want ErrInvalidRecipient", err)
	}
}

func TestSend_RejectsMediaWithoutSource(t *testing.T) {
	c := newTestClient(t, whatsapp.NewMockTransport())
	// Media{} carries neither ID nor Link -> rejected before any request.
	if _, err := c.Send(context.Background(), "15551234567", whatsapp.ImageByLink("", "")); !errors.Is(err, whatsapp.ErrInvalidMessage) {
		t.Fatalf("empty media: got %v, want ErrInvalidMessage", err)
	}
}

func TestSend_APIErrorMapsToSentinel(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(400,
		`{"error":{"message":"Re-engagement message","type":"OAuthException","code":131047,"fbtrace_id":"AbC"}}`)
	c := newTestClient(t, mt)

	_, err := c.SendText(context.Background(), "1555", "x")
	if !errors.Is(err, whatsapp.ErrReengagementRequired) {
		t.Fatalf("got %v, want ErrReengagementRequired", err)
	}
	var apiErr *whatsapp.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *APIError: %v", err)
	}
	if apiErr.Code != 131047 || apiErr.FBTraceID != "AbC" {
		t.Fatalf("APIError = %+v", apiErr)
	}
}

func TestSend_RetriesOnRateLimit(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(429,
		`{"error":{"message":"rate limited","code":131056}}`)
	c := newTestClient(t, mt, whatsapp.WithRetry(2, time.Millisecond))

	_, err := c.SendText(context.Background(), "1555", "x")
	if !errors.Is(err, whatsapp.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	// 1 initial + 2 retries = 3 attempts.
	if n := len(mt.Requests()); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

// TestSend_RetryBackoffNotDoubled guards against the double-sleep regression:
// with maxRetries=1 there is exactly one inter-attempt gap, so the total wait
// must be roughly one backoff (~40ms), not two (~80ms).
func TestSend_RetryBackoffNotDoubled(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(503, `{"error":{"message":"down","code":131000}}`)
	c := newTestClient(t, mt, whatsapp.WithRetry(1, 40*time.Millisecond))

	start := time.Now()
	_, _ = c.SendText(context.Background(), "1555", "x")
	elapsed := time.Since(start)

	if n := len(mt.Requests()); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to back off ~40ms", elapsed)
	}
	if elapsed > 70*time.Millisecond {
		t.Fatalf("elapsed = %v, want ~40ms (double-sleep regression?)", elapsed)
	}
}

func TestMarkRead_BuildsRequest(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	if err := c.MarkRead(context.Background(), "wamid.IN"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	req, ok := mt.LastRequest()
	if !ok {
		t.Fatal("no request captured")
	}
	if !strings.HasSuffix(req.URL, "/PNID/messages") {
		t.Fatalf("URL = %q, want .../PNID/messages", req.URL)
	}
	if req.Body["messaging_product"] != "whatsapp" ||
		req.Body["status"] != "read" ||
		req.Body["message_id"] != "wamid.IN" {
		t.Fatalf("payload = %#v", req.Body)
	}
}

func TestMarkRead_RejectsEmptyID(t *testing.T) {
	c := newTestClient(t, whatsapp.NewMockTransport())
	if err := c.MarkRead(context.Background(), ""); !errors.Is(err, whatsapp.ErrInvalidMessage) {
		t.Fatalf("got %v, want ErrInvalidMessage", err)
	}
}

func TestSendTyping_BuildsRequest(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	if err := c.SendTyping(context.Background(), "wamid.IN"); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
	req, _ := mt.LastRequest()
	if req.Body["status"] != "read" || req.Body["message_id"] != "wamid.IN" {
		t.Fatalf("payload = %#v", req.Body)
	}
	ti, _ := req.Body["typing_indicator"].(map[string]any)
	if ti["type"] != "text" {
		t.Fatalf("typing_indicator = %#v", req.Body["typing_indicator"])
	}

	if err := c.SendTyping(context.Background(), ""); !errors.Is(err, whatsapp.ErrInvalidMessage) {
		t.Fatalf("empty ID: got %v, want ErrInvalidMessage", err)
	}
}

func TestSend_ContextCancellationStopsRetry(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(503, `{"error":{"message":"down","code":131000}}`)
	c := newTestClient(t, mt, whatsapp.WithRetry(5, 50*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := c.SendText(ctx, "1555", "x")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
}

func TestWithAPIVersion_Override(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt, whatsapp.WithAPIVersion("v22.0"))
	_, _ = c.SendText(context.Background(), "1555", "x")
	req, _ := mt.LastRequest()
	if !strings.Contains(req.URL, "/v22.0/") {
		t.Fatalf("URL = %q, want /v22.0/", req.URL)
	}
}

func TestClassify_UnmappedCodeFallsBackToSendFailed(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(400, `{"error":{"message":"weird","code":999999}}`)
	c := newTestClient(t, mt)
	_, err := c.SendText(context.Background(), "1555", "x")
	if !errors.Is(err, whatsapp.ErrSendFailed) {
		t.Fatalf("got %v, want ErrSendFailed", err)
	}
}

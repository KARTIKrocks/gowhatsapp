package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Default endpoint settings. The API version is pinned and configurable — the
// single most important future-proofing lever, since Meta ships a new Graph
// API version roughly quarterly and deprecates old ones on a ~2-year clock.
// Override per client with [WithAPIVersion] or Config.APIVersion.
const (
	// DefaultBaseURL is the Meta Graph API host.
	DefaultBaseURL = "https://graph.facebook.com"
	// DefaultAPIVersion is the Graph API version requests target by default.
	DefaultAPIVersion = "v23.0"
)

const defaultHTTPTimeout = 30 * time.Second

// Config holds the credentials and endpoint settings for a Client. Only
// PhoneNumberID and AccessToken are required to send; AppSecret and
// WebhookVerifyToken are needed for the webhook handler; BusinessAccountID is
// reserved for template management (a future addition).
type Config struct {
	// PhoneNumberID is the Cloud API phone number ID messages are sent from.
	PhoneNumberID string
	// AccessToken is the system-user (preferably permanent) access token.
	AccessToken string
	// BusinessAccountID (WABA ID) — reserved for the template-management API.
	BusinessAccountID string
	// APIVersion overrides DefaultAPIVersion (e.g. "v22.0").
	APIVersion string
	// AppSecret signs/verifies webhook payloads (X-Hub-Signature-256).
	AppSecret string
	// WebhookVerifyToken is echoed during webhook subscription verification.
	WebhookVerifyToken string
	// BaseURL overrides DefaultBaseURL (proxy, sandbox, or test server).
	BaseURL string
}

// Option customizes a Client at construction.
type Option func(*Client)

// WithHTTPClient injects the HTTP doer used for all requests. Pass an
// *http.Client with your preferred timeout, transport, or instrumentation, or
// a [MockTransport] in tests.
func WithHTTPClient(d Doer) Option {
	return func(c *Client) {
		if d != nil {
			c.tr.doer = d
		}
	}
}

// WithAPIVersion overrides the Graph API version for this client.
func WithAPIVersion(version string) Option {
	return func(c *Client) {
		if version != "" {
			c.tr.apiVersion = version
		}
	}
}

// WithBaseURL overrides the API host (trailing slash trimmed).
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.tr.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithRetry configures retry behavior for transient failures (429 / 5xx).
// maxRetries is the number of additional attempts; base is the initial backoff
// (doubled each attempt, unless the response carries Retry-After).
func WithRetry(maxRetries int, base time.Duration) Option {
	return func(c *Client) {
		if maxRetries >= 0 {
			c.tr.maxRetries = maxRetries
		}
		if base > 0 {
			c.tr.backoff = base
		}
	}
}

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.log = l
			c.tr.log = l
		}
	}
}

// Client is a WhatsApp Cloud API client for a single phone number. It is safe
// for concurrent use.
type Client struct {
	phoneNumberID string
	appSecret     string
	verifyToken   string
	tr            *transport
	log           *slog.Logger
}

// New constructs a Client. PhoneNumberID and AccessToken are required.
func New(cfg Config, opts ...Option) (*Client, error) {
	if cfg.PhoneNumberID == "" {
		return nil, fmt.Errorf("%w: PhoneNumberID is required", ErrInvalidConfig)
	}
	if cfg.AccessToken == "" {
		return nil, fmt.Errorf("%w: AccessToken is required", ErrInvalidConfig)
	}

	log := slog.Default()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}

	c := &Client{
		phoneNumberID: cfg.PhoneNumberID,
		appSecret:     cfg.AppSecret,
		verifyToken:   cfg.WebhookVerifyToken,
		log:           log,
		tr: &transport{
			baseURL:    strings.TrimRight(baseURL, "/"),
			apiVersion: apiVersion,
			token:      cfg.AccessToken,
			doer:       &http.Client{Timeout: defaultHTTPTimeout},
			maxRetries: 2,
			backoff:    500 * time.Millisecond,
			log:        log,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Send delivers any [Message] to a recipient in E.164 form. A leading "+" and
// surrounding whitespace are accepted and stripped (the Cloud API wants the
// bare digits). The returned [Result] confirms acceptance and carries the
// message ID for correlating later webhook status events.
func (c *Client) Send(ctx context.Context, to string, msg Message) (*Result, error) {
	to, err := normalizeRecipient(to)
	if err != nil {
		return nil, err
	}
	if err := validateMessage(msg); err != nil {
		return nil, err
	}
	return c.send(ctx, msg.buildRequest(to))
}

// normalizeRecipient trims surrounding whitespace and an optional leading "+"
// (E.164 is written with one, but the Cloud API rejects it) and verifies that
// what remains is a non-empty run of digits.
func normalizeRecipient(to string) (string, error) {
	to = strings.TrimPrefix(strings.TrimSpace(to), "+")
	if to == "" {
		return "", fmt.Errorf("%w: empty recipient", ErrInvalidRecipient)
	}
	for _, r := range to {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%w: %q is not a digits-only phone number", ErrInvalidRecipient, to)
		}
	}
	return to, nil
}

// validateMessage rejects a nil message and lets a Message validate itself
// (see the optional validator interface) before any request is built.
func validateMessage(msg Message) error {
	if msg == nil {
		return fmt.Errorf("%w: nil message", ErrInvalidMessage)
	}
	if v, ok := msg.(validator); ok {
		return v.validate()
	}
	return nil
}

// SendReply sends a message as a reply to a previously received message,
// quoting it in the recipient's chat. replyToID is the message being replied to
// (the wamid... from [InboundMessage.ID] or a prior [Result.MessageID]). Any
// [Message] type may be sent as a reply.
func (c *Client) SendReply(ctx context.Context, to, replyToID string, msg Message) (*Result, error) {
	to, err := normalizeRecipient(to)
	if err != nil {
		return nil, err
	}
	if replyToID == "" {
		return nil, fmt.Errorf("%w: empty reply-to message ID", ErrInvalidMessage)
	}
	if err := validateMessage(msg); err != nil {
		return nil, err
	}
	req := msg.buildRequest(to)
	req["context"] = map[string]any{"message_id": replyToID}
	return c.send(ctx, req)
}

// send posts a fully built message request and parses the result.
func (c *Client) send(ctx context.Context, req map[string]any) (*Result, error) {
	body, err := c.tr.post(ctx, c.phoneNumberID+"/messages", req)
	if err != nil {
		return nil, err
	}
	return parseSendResult(body)
}

// SendText sends a plain text message (valid only inside an open 24-hour
// service window).
func (c *Client) SendText(ctx context.Context, to, body string) (*Result, error) {
	return c.Send(ctx, to, Text{Body: body})
}

// SendTemplate sends an approved message template.
func (c *Client) SendTemplate(ctx context.Context, to string, tmpl TemplateMessage) (*Result, error) {
	return c.Send(ctx, to, tmpl)
}

// MarkRead marks a previously received message as read (the blue ticks shown to
// the sender). Pass the inbound message's ID (the wamid... from
// [InboundMessage.ID]). The Cloud API returns no message ID for this call, so
// only an error is reported.
func (c *Client) MarkRead(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("%w: empty message ID", ErrInvalidMessage)
	}
	_, err := c.tr.post(ctx, c.phoneNumberID+"/messages", map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	})
	return err
}

// SendTyping displays a typing indicator to the user in reply to their message
// (messageID, the wamid... from [InboundMessage.ID]). WhatsApp dismisses it when
// you send your next message or after ~25 seconds, whichever comes first. This
// also marks the message as read, so there is no need to also call [Client.MarkRead].
func (c *Client) SendTyping(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("%w: empty message ID", ErrInvalidMessage)
	}
	_, err := c.tr.post(ctx, c.phoneNumberID+"/messages", map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
		"typing_indicator":  map[string]any{"type": "text"},
	})
	return err
}

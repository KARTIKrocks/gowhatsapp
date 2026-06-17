package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Sentinel errors. Every error returned by the library wraps one of these,
// so callers can branch with errors.Is without parsing message strings.
//
// The Cloud API surfaces failures as numeric codes; [APIError] carries the
// full detail while its Unwrap target is one of these sentinels (see the
// code → sentinel mapping in classify).
var (
	// ErrInvalidConfig means the client or webhook was configured incorrectly
	// (missing phone number ID, access token, app secret, etc.).
	ErrInvalidConfig = errors.New("whatsapp: invalid configuration")

	// ErrInvalidRecipient means the destination number was empty or malformed.
	ErrInvalidRecipient = errors.New("whatsapp: invalid recipient")

	// ErrInvalidMessage means the message payload was nil or incomplete.
	ErrInvalidMessage = errors.New("whatsapp: invalid message")

	// ErrSendFailed is the generic fallback for an API failure that does not
	// map to a more specific sentinel below.
	ErrSendFailed = errors.New("whatsapp: send failed")

	// ErrUnauthorized means the access token is missing, invalid, or expired
	// (Cloud API codes 0, 190).
	ErrUnauthorized = errors.New("whatsapp: unauthorized")

	// ErrRateLimited means the account or number hit a throughput / messaging
	// limit (codes 4, 80007, 130429, 131048, 131056). Honor Retry-After.
	ErrRateLimited = errors.New("whatsapp: rate limited")

	// ErrReengagementRequired means the 24-hour customer service window is
	// closed, so only an approved template may be sent (code 131047).
	ErrReengagementRequired = errors.New("whatsapp: re-engagement required (24h session window closed)")

	// ErrRecipientNotAllowed means the recipient is not in the allowed list of
	// an app still in development mode (code 131030).
	ErrRecipientNotAllowed = errors.New("whatsapp: recipient not in allowed list")

	// ErrTemplateNotApproved means the named template is missing, paused, or
	// not yet approved, or its parameters do not match (codes 132xxx).
	ErrTemplateNotApproved = errors.New("whatsapp: template not found or not approved")

	// ErrMediaError means a media asset could not be fetched or was rejected
	// (codes 131052, 131053).
	ErrMediaError = errors.New("whatsapp: media error")

	// ErrTransient means a temporary upstream failure that is safe to retry
	// (codes 131000, 131016, 131026, and 5xx responses).
	ErrTransient = errors.New("whatsapp: transient server error")

	// ErrInvalidSignature means a webhook payload failed X-Hub-Signature-256
	// verification.
	ErrInvalidSignature = errors.New("whatsapp: invalid webhook signature")

	// ErrUnsupported marks a capability not yet implemented by this library.
	ErrUnsupported = errors.New("whatsapp: operation not supported")
)

// APIError is the structured form of a WhatsApp Cloud API error response.
// It implements error and unwraps to one of the package sentinels so that
// both errors.Is(err, whatsapp.ErrRateLimited) and a *APIError type assertion
// work against the same value.
//
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
type APIError struct {
	// HTTPStatus is the HTTP status code of the response.
	HTTPStatus int
	// Code is the Cloud API error code.
	Code int
	// Subcode is the error_subcode, when present.
	Subcode int
	// Type is the error type string (e.g. "OAuthException").
	Type string
	// Title is a short human title, when the API provides one.
	Title string
	// Message is the human-readable error message.
	Message string
	// Details is the error_data.details field, when present.
	Details string
	// FBTraceID is Meta's trace identifier — quote it in support tickets.
	FBTraceID string
	// Raw is the unparsed response body.
	Raw []byte

	sentinel error
}

// Error implements the error interface.
func (e *APIError) Error() string {
	msg := e.Message
	if e.Details != "" {
		msg = msg + " (" + e.Details + ")"
	}
	if e.Subcode != 0 {
		return fmt.Sprintf("whatsapp: api error %d/%d: %s [type=%s http=%d trace=%s]",
			e.Code, e.Subcode, msg, e.Type, e.HTTPStatus, e.FBTraceID)
	}
	return fmt.Sprintf("whatsapp: api error %d: %s [type=%s http=%d trace=%s]",
		e.Code, msg, e.Type, e.HTTPStatus, e.FBTraceID)
}

// Unwrap returns the mapped sentinel so errors.Is works.
func (e *APIError) Unwrap() error { return e.sentinel }

// Retryable reports whether retrying the request might succeed.
func (e *APIError) Retryable() bool {
	return errors.Is(e.sentinel, ErrTransient) || errors.Is(e.sentinel, ErrRateLimited)
}

// classify maps a Cloud API error code to a sentinel. An unmapped code
// returns nil; callers fall back to ErrSendFailed.
func classify(code int) error {
	switch code {
	case 0, 190:
		return ErrUnauthorized
	case 4, 80007, 130429, 131048, 131056:
		return ErrRateLimited
	case 131047:
		return ErrReengagementRequired
	case 131030:
		return ErrRecipientNotAllowed
	case 131052, 131053:
		return ErrMediaError
	case 131000, 131016, 131026:
		return ErrTransient
	case 132000, 132001, 132005, 132007, 132012, 132015, 132016, 132068, 132069:
		return ErrTemplateNotApproved
	default:
		return nil
	}
}

// parseAPIError converts a non-2xx Cloud API response into an *APIError.
func parseAPIError(status int, body []byte) *APIError {
	var env struct {
		Error struct {
			Message      string `json:"message"`
			Type         string `json:"type"`
			Code         int    `json:"code"`
			ErrorSubcode int    `json:"error_subcode"`
			FBTraceID    string `json:"fbtrace_id"`
			ErrorData    struct {
				Details string `json:"details"`
			} `json:"error_data"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)

	e := &APIError{
		HTTPStatus: status,
		Code:       env.Error.Code,
		Subcode:    env.Error.ErrorSubcode,
		Type:       env.Error.Type,
		Message:    env.Error.Message,
		Details:    env.Error.ErrorData.Details,
		FBTraceID:  env.Error.FBTraceID,
		Raw:        body,
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("unexpected status %d", status)
	}
	e.sentinel = classify(e.Code)
	if e.sentinel == nil {
		if status >= 500 {
			e.sentinel = ErrTransient
		} else {
			e.sentinel = ErrSendFailed
		}
	}
	return e
}

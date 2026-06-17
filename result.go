package whatsapp

import "encoding/json"

// Result is the outcome of a successful send. The Cloud API accepts a message
// for delivery asynchronously, so a Result confirms acceptance (and yields the
// message ID to correlate later webhook status updates) — not final delivery.
type Result struct {
	// MessageID is the wamid... identifier. Match it against the message_id of
	// later webhook status events (sent/delivered/read/failed).
	MessageID string
	// WAID is the recipient's resolved WhatsApp ID, when returned.
	WAID string
	// Input is the recipient address as the API echoed it back.
	Input string
	// Raw is the unparsed success response body.
	Raw []byte
}

// parseSendResult extracts a Result from a Cloud API send response.
func parseSendResult(body []byte) (*Result, error) {
	var r struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Contacts []struct {
			Input string `json:"input"`
			WaID  string `json:"wa_id"`
		} `json:"contacts"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		// A 2xx with an unexpected shape still counts as accepted; surface the
		// raw body rather than failing the caller.
		return &Result{Raw: body}, nil //nolint:nilerr // accepted-but-opaque is not an error
	}
	res := &Result{Raw: body}
	if len(r.Messages) > 0 {
		res.MessageID = r.Messages[0].ID
	}
	if len(r.Contacts) > 0 {
		res.Input = r.Contacts[0].Input
		res.WAID = r.Contacts[0].WaID
	}
	return res, nil
}

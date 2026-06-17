package whatsapp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

// mockSuccessBody is the default success response the mock returns.
const mockSuccessBody = `{"messaging_product":"whatsapp","contacts":[{"input":"recipient","wa_id":"recipient"}],"messages":[{"id":"wamid.MOCK"}]}`

// MockTransport is a [Doer] for tests. It records every request (with the
// decoded WhatsApp payload) and returns a programmable response, so consumer
// code can assert on what would have been sent without touching the network.
//
//	mt := whatsapp.NewMockTransport()
//	c, _ := whatsapp.New(cfg, whatsapp.WithHTTPClient(mt))
//	_, _ = c.SendText(ctx, "15551234567", "hi")
//	req, _ := mt.LastRequest()
//	// assert req.Body["type"] == "text"
//
// It is safe for concurrent use.
type MockTransport struct {
	mu       sync.Mutex
	requests []CapturedRequest
	respCode int
	respBody []byte
	err      error
}

// CapturedRequest is one request the mock observed.
type CapturedRequest struct {
	Method  string
	URL     string
	Header  http.Header
	RawBody []byte
	// Body is RawBody decoded as JSON, for convenient assertions.
	Body map[string]any
}

// NewMockTransport returns a mock that responds 200 with a stub message ID.
func NewMockTransport() *MockTransport {
	return &MockTransport{}
}

// WithResponse programs the HTTP status and body returned for every request.
// Use it to simulate API errors:
//
//	mt.WithResponse(429, `{"error":{"code":131056,"message":"rate limited"}}`)
func (m *MockTransport) WithResponse(status int, body string) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.respCode = status
	m.respBody = []byte(body)
	return m
}

// WithError programs a transport-level (network) error for every request.
func (m *MockTransport) WithError(err error) *MockTransport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
	return m
}

// Do implements [Doer].
func (m *MockTransport) Do(req *http.Request) (*http.Response, error) {
	captured := CapturedRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header.Clone(),
	}
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		captured.RawBody = raw
		_ = json.Unmarshal(raw, &captured.Body)
	}

	m.mu.Lock()
	m.requests = append(m.requests, captured)
	err := m.err
	code := m.respCode
	body := m.respBody
	m.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if code == 0 {
		code = http.StatusOK
	}
	if body == nil {
		body = []byte(mockSuccessBody)
	}
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// Requests returns a copy of all captured requests, oldest first.
func (m *MockTransport) Requests() []CapturedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CapturedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

// LastRequest returns the most recent captured request, if any.
func (m *MockTransport) LastRequest() (CapturedRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return CapturedRequest{}, false
	}
	return m.requests[len(m.requests)-1], true
}

// Reset clears captured requests and programmed responses.
func (m *MockTransport) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
	m.respCode = 0
	m.respBody = nil
	m.err = nil
}

package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// Doer is the HTTP seam: anything that can execute a request. *http.Client
// satisfies it, as does [MockTransport] for tests and any custom round-tripper
// (proxy, instrumentation, on-prem gateway). This is the single point where
// network behavior is injected, keeping the client itself transport-agnostic.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// transport holds the Graph API wiring. It is intentionally unexported; the
// client owns one and configures it through functional options.
type transport struct {
	baseURL    string
	apiVersion string
	token      string
	doer       Doer
	maxRetries int
	backoff    time.Duration
	log        *slog.Logger
}

// post sends a JSON payload to {baseURL}/{apiVersion}/{path} and returns the
// raw success body, or an *APIError.
func (t *transport) post(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal payload: %w", ErrInvalidMessage, err)
	}
	url := t.baseURL + "/" + t.apiVersion + "/" + path
	out, _, err := t.execute(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	return out, err
}

// postRaw sends a pre-encoded body (e.g. multipart) to {apiVersion}/{path}.
func (t *transport) postRaw(ctx context.Context, path string, body []byte, contentType string) ([]byte, error) {
	url := t.baseURL + "/" + t.apiVersion + "/" + path
	out, _, err := t.execute(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.token)
		req.Header.Set("Content-Type", contentType)
		return req, nil
	})
	return out, err
}

// get performs an authenticated GET against {apiVersion}/{path}.
func (t *transport) get(ctx context.Context, path string) ([]byte, error) {
	return t.getURL(ctx, t.baseURL+"/"+t.apiVersion+"/"+path)
}

// getURL performs an authenticated GET against an absolute URL (used for the
// short-lived media download URLs Meta hands back, which live off-host).
func (t *transport) getURL(ctx context.Context, url string) ([]byte, error) {
	out, _, err := t.execute(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.token)
		return req, nil
	})
	return out, err
}

// del performs an authenticated DELETE against {apiVersion}/{path}.
func (t *transport) del(ctx context.Context, path string) ([]byte, error) {
	url := t.baseURL + "/" + t.apiVersion + "/" + path
	out, _, err := t.execute(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t.token)
		return req, nil
	})
	return out, err
}

// execute runs newReq with bounded retry on 429/5xx and network errors,
// returning the success body and response headers, or an *APIError. newReq must
// build a fresh *http.Request on each call (the body is re-read per attempt).
//
// All backoff happens at the single sleep site at the top of the loop;
// retryDelay is computed when a retryable failure is observed and carried
// forward in retryAfter, so a Retry-After header set on one attempt governs the
// wait before the next.
func (t *transport) execute(ctx context.Context, newReq func() (*http.Request, error)) ([]byte, http.Header, error) {
	var lastErr error
	var retryAfter time.Duration
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, retryAfter); err != nil {
				return nil, nil, err
			}
		}

		req, err := newReq()
		if err != nil {
			return nil, nil, fmt.Errorf("whatsapp: build request: %w", err)
		}

		resp, err := t.doer.Do(req)
		if err != nil {
			// Network-level error: retry if budget remains.
			lastErr = fmt.Errorf("%w: %w", ErrTransient, err)
			if attempt < t.maxRetries {
				retryAfter = t.retryDelay(nil, attempt)
				continue
			}
			return nil, nil, lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("%w: read response: %w", ErrTransient, readErr)
			if attempt < t.maxRetries {
				retryAfter = t.retryDelay(nil, attempt)
				continue
			}
			return nil, nil, lastErr
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, resp.Header, nil
		}

		apiErr := parseAPIError(resp.StatusCode, respBody)
		if t.shouldRetry(resp.StatusCode) && attempt < t.maxRetries {
			retryAfter = t.retryDelay(resp.Header, attempt)
			lastErr = apiErr
			continue
		}
		return nil, nil, apiErr
	}
	return nil, nil, lastErr
}

func (t *transport) shouldRetry(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// retryDelay honors a Retry-After header when present, else exponential backoff.
func (t *transport) retryDelay(h http.Header, attempt int) time.Duration {
	if h != nil {
		if ra := h.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs >= 0 {
				return time.Duration(secs) * time.Second
			}
		}
	}
	d := t.backoff
	if d <= 0 {
		d = 500 * time.Millisecond
	}
	return d << attempt
}

// sleepCtx waits for d or until ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

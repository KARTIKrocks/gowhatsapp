# Code Review — gowhatsapp

_Review date: 2026-06-14 · Reviewed at Phase 1 (send + webhooks)._

## Verdict

Yes — this would genuinely help the Go community. Most existing options (e.g.
`whatsmeow`) target the **unofficial** multi-device protocol; zero-dependency
libraries wrapping the **official WhatsApp Cloud API** with typed errors and a
built-in webhook handler are thin on the ground.

The code is clean, idiomatic, and well-documented, with good design instincts:

- Sealed `Message` interface (additive, non-breaking new message types).
- Swappable `Doer` transport seam (inject `*http.Client`, proxy, or mock).
- Pinned + overridable Graph API version (isolates callers from Meta's churn).
- Sentinel errors usable via `errors.Is` plus a rich `*APIError`.

It's a solid foundation. The items below should be addressed before publishing.

---

## Bugs

### 1. Double sleep in the retry loop (`transport.go`) — confirmed, tests miss it

For the HTTP-status retry path (429 / 5xx) the code sleeps **twice** per retry:

- `transport.go:85-88` sleeps `retryDelay(resp.Header, attempt)` inside the
  error block, then `continue`s.
- `transport.go:47-51` then sleeps **again** at the top of the next iteration
  with `retryDelay(nil, attempt-1)`.

Consequences:

- A 429 with `Retry-After: 30` waits 30s **and then** an extra backoff.
- The top-of-loop sleep passes `nil` for the header, so it **ignores
  `Retry-After`** entirely.
- The network-error path only sleeps once (top of loop) — behavior is
  inconsistent between the two retry paths.

The existing `TestSend_RetriesOnServerError` uses a 1ms backoff and only counts
*attempts* (= 3), so the doubling is invisible.

**Suggested fix** — do all backoff at the top of the loop and carry the delay
forward; delete the inner `sleepCtx`:

```go
var lastErr error
var retryAfter time.Duration
for attempt := 0; attempt <= t.maxRetries; attempt++ {
    if attempt > 0 {
        if err := sleepCtx(ctx, retryAfter); err != nil {
            return nil, err
        }
    }
    // ... do request ...
    // network error:
    retryAfter = t.retryDelay(nil, attempt)
    // ... continue
    // retryable status:
    retryAfter = t.retryDelay(resp.Header, attempt)
    lastErr = apiErr
    continue
}
```

Add a timing-based test (record timestamps / assert elapsed) so this can't
regress.

---

## Lacking features (highest value first)

_All four below were implemented in Phase 2._

- ~~**Mark message as read**~~ **Done** — `Client.MarkRead`.
- ~~**Reply context (`context.message_id`)**~~ **Done** — `Client.SendReply`,
  and inbound `ContextID` parsing.
- ~~**Interactive messages** (reply buttons / list)~~ **Done** —
  `InteractiveButtons` / `InteractiveList`, plus inbound `Interactive` parsing.
- ~~**Media upload/download**~~ **Done** — `Upload` / `MediaInfo` / `Download` /
  `DeleteMedia`, plus `*ByID` constructors and inbound `Media` parsing.

Still open / future: template management API, Flows, contacts message type,
business-profile admin.

---

## Design / smaller issues

- ~~**`go 1.26` directive is an adoption barrier.**~~ **Done** — set to `go 1.22`.
- ~~**Webhook handler returns 400 on parse failure**~~ **Done** — unparseable
  bodies are now logged and answered with `200` (the documented contract);
  `401` is still returned for a bad signature. A `Logger` field on
  `WebhookConfig` controls where the warning goes.
- ~~**No recipient normalization.**~~ **Done** — `Send`/`SendReply` strip a
  leading `+` and surrounding whitespace, then reject non-digit recipients with
  `ErrInvalidRecipient` before any request.
- ~~**`Media.buildRequest` doesn't guard "exactly one of ID/Link".**~~ **Done**
  — an optional `validator` hook lets `Media` reject "neither/both" with
  `ErrInvalidMessage` up front.
- **Webhook callbacks run synchronously** in the request goroutine. The
  "hand off durably and return quickly" guidance is correct — just make the
  warning loud, since a slow `OnMessage` blocks Meta's delivery and can trigger
  redelivery storms.
- **`classify` maps code `0` → `ErrUnauthorized`** (`errors.go`). Code 0 is
  sometimes a generic/unknown error, not strictly auth. Minor; comment or
  revisit.

---

## Nice-to-haves

- Add `golangci-lint` + race detector to CI if not already wired.
- Add a fuzz target for `ParseEvent` / `VerifySignature`.
- Add an example using `ParseEvent` / `WebhookEvent.Raw` directly (the escape
  hatch for unmodeled inbound types).

---

## Suggested order of work

1. ~~Fix the retry double-sleep bug + add a timing-based test.~~ **Done** —
   single sleep site in `transport.go`; `TestSend_RetryBackoffNotDoubled` guards
   it.
2. ~~Lower the `go` directive to `1.22`.~~ **Done** — module + both example
   modules + README.
3. ~~Add `MarkRead`.~~ **Done** — `Client.MarkRead` + tests + README.
4. ~~Add reply-context support.~~ **Done** — `Client.SendReply` injects
   `context.message_id`; `Send` refactored onto a shared `send` path; tests +
   README.

All four are addressed; the library is in good shape to publish as v0.x.

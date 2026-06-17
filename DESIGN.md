# gowhatsapp — Design & Roadmap

**Status:** v0.1.0 — Phase 1 (send + webhooks) + Phase 2 (media, interactive, mark-read/typing)
**Audience:** maintainers + contributors
**Companion docs:** [`README.md`](./README.md) (usage), [`doc.go`](./doc.go) (API reference)

A Go client for the **WhatsApp Cloud API** (Meta Graph API), built to be a
long-lived, dependency-light client the wider community can adopt.

---

## 1. Why this exists / scope

WhatsApp Cloud is a distinct messaging channel with templates, a 24-hour
customer-service window, media, interactive replies, and its own webhook +
signature model. This library targets that channel directly and does one thing
well, rather than hiding it behind a generic multi-channel messaging abstraction.

**In scope (the library's job):** _deliver_ messages and _receive_ webhooks
against Meta's Cloud API.

**Explicitly out of scope:**

- **OTP generation/verification.** Meta only _delivers_ the authentication
  template; it never validates the code. Generation + verification stay in the
  calling application (for example, a short-lived code in your own datastore).
  The library provides [`AuthTemplate`](./template.go) to format the _delivery_
  only.
- **BSP abstractions.** This targets Meta Cloud directly, not Twilio/MSG91
  WhatsApp. A multi-BSP layer is a non-goal; if ever needed it belongs one level
  up, behind a caller-owned interface.
- **Cross-channel fallback.** "WhatsApp primary → email fallback" is the
  _consumer's_ orchestration concern (it spans two libraries), not this one's.

---

## 2. Design principles

1. **Zero third-party dependencies in the core.** stdlib only (`net/http`,
   `crypto/hmac`, `encoding/json`). This is the single biggest driver of
   long-term, low-friction adoption.
2. **Single vendor → single module, seams not sub-modules.** There is exactly
   one backend (Meta Cloud), so the future-proofing axis is _internal seams_,
   not provider packages — the API absorbs change without a plugin layer.
3. **No global state. Constructor + functional options.** `New(Config, ...Option)`.
4. **Context-first.** Every network call takes `context.Context` and honors
   cancellation, including mid-retry.
5. **Errors are typed and matchable.** Every failure wraps a sentinel
   (`errors.Is`) _and_ exposes structure (`errors.As` → `*APIError`).
6. **Safe by default.** Webhook signatures verified, body size capped, default
   HTTP timeout set, retries bounded.

---

## 3. The three future-proofing levers

WhatsApp's API surface and Meta's platform both move. Three deliberate seams
absorb that change without breaking callers:

### 3.1 Pinned, configurable API version

Meta ships a new Graph API version ~quarterly and sunsets versions on a ~2-year
clock. The version lives in **one constant** (`DefaultAPIVersion`) and is
overridable per client (`WithAPIVersion` / `Config.APIVersion`). Upgrading is a
one-line change; an individual consumer can pin independently.

### 3.2 Swappable transport (`Doer`)

All HTTP goes through the `Doer` interface (`Do(*http.Request)`), satisfied by
`*http.Client`. This is the one injection point for timeouts, proxies,
instrumentation, and — crucially — `MockTransport` for hermetic consumer tests.
New endpoints (media, template management) reuse the same plumbing, retry, and
error mapping for free.

### 3.3 Sealed `Message` interface

`Message` has an **unexported** method (`buildRequest`), so only this package can
implement it. New message types (interactive, flows, contacts, …) are purely
_additive_ — they can never break a caller, because no external code holds a
competing implementation. This turns "add a message type" from a potential
major-version event into a patch.

---

## 4. Package layout

One module, files split by concern (not by layer — this is a library, not a
service):

| File             | Responsibility                                             |
| ---------------- | ---------------------------------------------------------- |
| `whatsapp.go`    | `Client`, `Config`, `New`, functional `Option`s, `Send*`   |
| `transport.go`   | `Doer` seam, HTTP plumbing, bounded retry w/ `Retry-After` |
| `message.go`     | sealed `Message` + Text / Media / Location / Reaction      |
| `template.go`    | `TemplateMessage`, components, `AuthTemplate` (OTP)        |
| `interactive.go` | `InteractiveButtons` / `InteractiveList` reply menus       |
| `media.go`       | media `Upload` / `MediaInfo` / `Download` / `DeleteMedia`  |
| `result.go`      | `Result` + response parsing                                |
| `errors.go`      | sentinels, `*APIError`, Cloud-API code → sentinel mapping  |
| `webhook.go`     | signature verify, event parsing, `http.Handler`            |
| `mock.go`        | `MockTransport` for consumer tests                         |
| `doc.go`         | package documentation                                      |

---

## 5. Error model

Failures carry **both** a matchable sentinel and full structure:

```go
res, err := client.SendTemplate(ctx, to, tmpl)
switch {
case errors.Is(err, whatsapp.ErrReengagementRequired): // 131047 — window closed
case errors.Is(err, whatsapp.ErrRateLimited):          // back off (Retry-After honored)
case errors.Is(err, whatsapp.ErrTemplateNotApproved):  // 132xxx — fix template
}

var apiErr *whatsapp.APIError
if errors.As(err, &apiErr) {
    log.Error("send failed", "code", apiErr.Code, "trace", apiErr.FBTraceID)
}
```

Unmapped codes fall back to `ErrTransient` (5xx) or `ErrSendFailed`, so callers
can always branch on _something_ even when Meta introduces a new code.

---

## 6. Roadmap (phased, each phase ships independently)

**Phase 1 — Send + Webhooks (this foundation).** Text, media-by-link, location,
reaction, templates incl. auth/OTP; webhook verify + parse + handler; mock;
typed errors; retry. ✅

**Phase 2 — Media & richer sends.** ✅

- Media **upload/download** API (`/{phone_number_id}/media`, `/{media_id}`) →
  media-by-ID sends and inbound media retrieval.
- **Interactive** messages (reply buttons, list) and parsing their inbound
  replies.
- `mark_as_read` + typing indicators.

**Phase 3 — Template management (admin).** Create / list / delete templates via
the WABA ID (`/{waba_id}/message_templates`) in a separate `templates`-admin
surface, so the hot send path stays lean. Status webhooks
(`message_template_status_update`).

**Phase 4 — Advanced.** Flows, products/catalog, reactions on inbound, richer
conversation/pricing analytics from status webhooks.

Each phase is additive: Phase 1 callers never have to change.

---

## 7. API stability policy

- **v0.x:** API may change between minor versions; changes called out in
  [`CHANGELOG.md`](./CHANGELOG.md). Aiming for a fast path to v1.
- **v1+:** semver. The sealed `Message` interface and the `Doer` seam are the
  load-bearing compatibility guarantees — new types/endpoints are additive.
- **Graph API versions:** the library tracks a recent, stable Graph API version
  in `DefaultAPIVersion`; bumping it is a minor release with a changelog note.
  Consumers can always pin their own via `WithAPIVersion`.

---

## 8. Testing strategy

- **Hermetic by default.** `MockTransport` (a `Doer`) captures the exact request
  body, so payload-shape tests need no network and consumers can test their own
  code the same way.
- **Wire-shape assertions.** Template/auth tests round-trip through JSON to lock
  the exact structure Meta requires (e.g. button `index` as a _string_).
- **Webhook tests** exercise real HMAC signing, the GET challenge, POST
  dispatch, bad-signature rejection, and raw-message preservation.
- CI runs `go test -race`, `go vet`, and `golangci-lint` (v2) on every supported
  Go version.

---

## 9. Integrating into an application

The library is a focused building block; a typical integration:

1. Construct one `Client` per WhatsApp phone number (`New(Config, ...Option)`)
   and reuse it — it is safe for concurrent use.
2. Send notifications/alerts with `SendTemplate` (outside the 24h window) or
   `Send`/`SendText` (inside it); branch on the typed sentinels to decide
   retry vs. fall back to another channel.
3. For one-time passcodes, generate and verify the code in your own
   application; use `AuthTemplate` only to _deliver_ it. The library never
   validates codes.
4. Mount `NewWebhookHandler` to ingest delivery receipts (update message status)
   and inbound messages (two-way conversations).

The library holds no global state, so multiple clients and webhook handlers can
coexist in one process.

# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-17

Initial release: a zero-dependency Go client for the WhatsApp Cloud API
(Meta Graph API, direct), covering outbound sends, media, and inbound webhooks.

### Added

- **Client** (`New`) for a single phone number, safe for concurrent use, with
  functional options: `WithHTTPClient`, `WithAPIVersion`, `WithBaseURL`,
  `WithRetry`, `WithLogger`. API version is pinned (default `v23.0`) and
  overridable.
- **Sending** — `Send`, `SendReply` (quoted replies), and convenience
  `SendText` / `SendTemplate`. Recipients are normalized (leading `+` and
  whitespace stripped, digits-only validated) before any request.
- **Message types** — `Text`, `Reaction`, `Location`, `Media`
  (image/document/video/audio/sticker, by public link or uploaded ID),
  `TemplateMessage`, and interactive `InteractiveButtons` / `InteractiveList`.
  The `Message` interface is sealed for forward-compatible additions.
- **Templates** — `TemplateComponent` / `TemplateParameter` builders, plus
  `TextParam`, `BodyComponent`, and `AuthTemplate` for OTP-delivery templates
  (the library delivers the code; generation/verification stay in your app).
- **Media API** — `Upload`, `MediaInfo`, `Download` (resolves the short-lived
  authenticated URL), and `DeleteMedia`.
- **Engagement helpers** — `MarkRead` and `SendTyping`.
- **Webhooks** — `NewWebhookHandler` implementing the full Cloud API contract
  (GET subscription challenge + POST dispatch), `ParseEvent` to flatten the
  nested envelope, and constant-time `VerifySignature` (X-Hub-Signature-256).
  Inbound parsing covers text, interactive replies, media, location, and
  reactions, with the raw object preserved on `InboundMessage.Raw`.
- **Errors** — typed `APIError` (HTTP status, code/subcode, type, message,
  details, FB trace ID, raw body) that unwraps to sentinel errors
  (`ErrUnauthorized`, `ErrRateLimited`, `ErrReengagementRequired`,
  `ErrRecipientNotAllowed`, `ErrTemplateNotApproved`, `ErrMediaError`,
  `ErrTransient`, `ErrInvalidSignature`, …) for `errors.Is` branching, plus
  `APIError.Retryable()`.
- **Resilience** — bounded retry on 429/5xx and network errors with
  exponential backoff that honors `Retry-After`, and context-aware cancellation
  of the inter-attempt wait.
- **Testing** — `MockTransport`, a `Doer` that records requests (with the
  decoded payload) and returns programmable responses/errors, safe for
  concurrent use.

### Notes

- Requires Go 1.22+. Zero third-party dependencies.
- Template-management (create/submit templates for approval) is not yet
  included — manage templates in WhatsApp Manager for now.

[Unreleased]: https://github.com/KARTIKrocks/gowhatsapp/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/KARTIKrocks/gowhatsapp/releases/tag/v0.1.0

# Style and pattern rationale

Context for the scoped rules in `config.json`. This file is freeform prose read
alongside the diff; `config.json` is what actually gates comment scope and
severity. Most of this is mined from REVIEW.md, this repo's own past review —
treat its findings as precedent, not hypotheticals.

## The retry double-sleep bug (fixed, but easy to reintroduce)

REVIEW.md's headline bug: the HTTP-status retry path slept **twice** per
retry — once inside the error branch (`retryDelay(resp.Header, attempt)`,
honoring `Retry-After`), then again at the top of the next loop iteration
(`retryDelay(nil, attempt-1)`, which — passed a nil header — silently dropped
`Retry-After` for the second sleep). The network-error path only slept once,
so the two retry paths behaved inconsistently, and the existing test missed it
entirely because it only asserted attempt *count*, not elapsed time.

The fix (now in place, guarded by `TestSend_RetryBackoffNotDoubled`) computes
the delay once per outcome and sleeps it exactly once, at the top of the next
iteration. If a diff touches `transport.go`'s retry loop, check that this
shape survives: one `sleepCtx` call site, fed by whichever branch just ran,
never a sleep inside the branch itself.

## Why `parseAPIError` uses `*int` for `Code`

REVIEW.md also flagged that `classify` maps Cloud API code `0` to
`ErrUnauthorized`, which is only sometimes right — 0 can mean "generic/unknown
error" too. The code already defends against the sharper version of this bug:
`parseAPIError`'s JSON envelope declares `Code *int`, not `int`, specifically
so a response with no `code` field at all doesn't decode to the zero value and
get misclassified as unauthorized. Don't "simplify" that back to a plain
`int` — it would silently reintroduce a misclassification for malformed or
non-standard error bodies.

## Webhook callbacks run in the request goroutine

`Handlers.OnMessage` (and the other webhook callbacks) fire synchronously
inside the HTTP handler, per DESIGN.md's "hand off durably and return
quickly" contract — the library does not spawn a goroutine for you. This is a
deliberate choice, not an oversight, but it means a slow callback blocks
Meta's delivery and can trigger redelivery storms. A PR that adds work to the
webhook path (parsing, additional callback fan-out, etc.) should keep that
work fast or push it onto a queue the caller owns; don't add unbounded work
directly on the request path without calling it out.

## Sealed `Message` interface

`Message`'s `buildRequest` method is unexported on purpose (DESIGN.md 3.3):
external code can never implement `Message`, so every new message type is
additive by construction. The risk to watch for is the opposite direction —
a change that exports `buildRequest`, adds a second required method, or
otherwise turns the interface into something an external type could
implement (and therefore something adding a case to could break). None of
that should ever happen outside a major version bump.

## Secrets: what's safe to surface, what isn't

Per SECURITY.md, `AccessToken` and `AppSecret` must never appear in a log line
or error message. `*APIError`'s `FBTraceID` and `Code` are explicitly the
safe fields for diagnostics — that's the intended path for "what do I put in
my error log," not sprinkling `fmt.Sprintf("%+v", cfg)` somewhere convenient.

## Nice-to-haves noted but not yet done

REVIEW.md also suggested a fuzz target for `ParseEvent`/`VerifySignature` and
an example built directly on `WebhookEvent.Raw` for unmodeled inbound event
types. Neither exists yet; a PR adding either is filling a known gap, not
scope creep.

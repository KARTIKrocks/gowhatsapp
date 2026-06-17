# Security Policy

## Reporting a Vulnerability

Please report security issues privately via GitHub's "Report a vulnerability"
(Security Advisories) on this repository, rather than opening a public issue.
We aim to acknowledge reports within 72 hours.

## Handling secrets

This library never logs your access token or app secret. When reporting an
issue or sharing logs:

- **Never** include the `AccessToken` or `AppSecret`.
- An `*APIError`'s `FBTraceID` and `Code` are safe to share and help diagnosis.

## Webhook verification

The webhook handler verifies the `X-Hub-Signature-256` HMAC when an `AppSecret`
is configured, and the comparison is constant-time. **Always set `AppSecret`**
in production; leaving it empty disables signature verification and is intended
only for local testing.

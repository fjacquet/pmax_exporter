# Basic auth & retry policy

## Status
Accepted.

## Context
The family standard prefers a bearer + token-refresh flow when the backend offers one.
Unisphere for PowerMax does not: its REST API authenticates every request with **HTTP
Basic over TLS**. There is no login endpoint, no token lifecycle, and no server-side
session to manage (Unisphere maintains an internal session keyed off the credentials).

## Decision
- HTTP Basic on every request (`resty.SetBasicAuth`), TLS minimum 1.2,
  `insecureSkipVerify` as an explicit operator opt-in for lab/self-signed instances.
- **Retry only on 5xx** (2 retries). Never retry 4xx — an auth or permission failure must
  surface immediately, not loop against the backend (Unisphere locks accounts on repeated
  bad credentials).
- `--trace` logs method/URL/status/**response body only** via an `OnAfterResponse` hook —
  never resty `SetDebug`, which dumps request headers including the `Authorization`
  header. With Basic auth no response body ever carries credentials, so no endpoint
  needs to be skipped (unlike ppdm's login exchange). A test asserts the trace output
  contains neither the password nor its base64 form.

## Consequences
The client is stateless — no token cache, no refresh races, no relogin-on-401 path.
The cost is credentials on every request, which is Unisphere's own design; TLS is
therefore non-negotiable (the exporter never speaks plain HTTP).

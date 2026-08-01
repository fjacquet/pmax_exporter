# Serve HTTP before first collect

## Status
Accepted.

## Context
The first collection cycle against a large array can take tens of seconds (object
discovery plus one performance POST per storage group/director). Blocking startup on it
would stall `/metrics` and `/health`, masking the exporter behind the very backend it
monitors.

## Decision
Start the HTTP server **before** the first collection cycle. The snapshot store is
pre-populated with an empty snapshot, so early scrapes return an empty (but valid)
exposition instead of hanging. `/health` reports per-instance status from the latest
snapshot in its JSON body. (Since ADR-0013, `/health` always answers 200 — the chart's
`livenessProbe`/`readinessProbe` use the always-200 `/livez`/`/readyz` endpoints
instead, so this early-serve behavior no longer affects probe results either way.)

## Consequences
Deterministic startup independent of backend health. Orchestrators see the exporter as
live immediately; data readiness is observable via `pmax_up` and `/health`'s per-server
`ok`/`err` fields rather than implied by the socket being open or by any endpoint's
status code.

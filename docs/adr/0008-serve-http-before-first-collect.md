# Serve HTTP before first collect

## Status
Accepted.

## Context
The first collection cycle against a large array can take tens of seconds (object
discovery plus one performance POST per storage group/director). Blocking startup on it
would stall `/metrics` and `/health`, failing readiness probes and masking the exporter
behind the very backend it monitors.

## Decision
Start the HTTP server **before** the first collection cycle. The snapshot store is
pre-populated with an empty snapshot, so early scrapes return an empty (but valid)
exposition instead of hanging. `/health` reports per-instance status from the latest
snapshot and returns 503 until every configured instance has a healthy cycle.

## Consequences
Deterministic startup independent of backend health. Orchestrators see the exporter as
live immediately; data readiness is observable via `pmax_up` and `/health` rather than
implied by the socket being open.

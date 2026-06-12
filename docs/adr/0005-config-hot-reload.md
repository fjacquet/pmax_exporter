# Config hot reload

## Status
Accepted.

## Context
Adding a Unisphere instance or rotating a password should not drop metrics for the other
targets, and config errors must never kill a running exporter.

## Decision
Reload on **SIGHUP and file change** (fsnotify). The watcher observes the config file's
*parent directory*, not the file inode — editors and config managers replace files via
temp-file + rename, which kills inode-bound watches. A reload rebuilds clients and the
collection loop, then swaps them in; the HTTP server and snapshot store survive. A reload
that fails validation is logged and dropped — the running config stays.

## Consequences
Zero-downtime config changes; the previous snapshot keeps serving during the swap. The
`${ENV}` interpolation runs again on reload, so rotated secrets in env/`passwordFile` are
picked up without a restart.

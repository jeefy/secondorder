# SO-16: Startup Diagnostics for Agent Count and Commit Hash

## Summary

Added startup diagnostics in `cmd/secondorder/main.go` so the service logs key boot metadata at INFO level after startup agent loading is complete.

## Changes Implemented

- Added `logStartupDiagnostics(database)` and invoked it immediately after startup template application.
- Logged total loaded agents with an INFO message:
  - `msg="loaded agents" count=<n>`
- Logged build-embedded git commit hash with an INFO message:
  - `msg="startup git commit" commit=<hash-or-unknown>`
- Reused existing `slog` logging framework with no new dependencies.

## Safety and Data Exposure

- Diagnostic output includes only:
  - integer agent count
  - build commit hash
- No API keys, tokens, or secret configuration values are logged.

## Verification

- Added unit test `TestLogStartupDiagnosticsLogsAgentCountAndCommit` in `cmd/secondorder/main_test.go`.
- Test asserts presence of both INFO log messages and the commit field value.

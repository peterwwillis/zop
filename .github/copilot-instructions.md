# Copilot Instructions for zop

## Repository overview
- Go CLI application; main entrypoint is `cmd/zop`.
- Core packages live under `internal/`:
  - `internal/cli` for Cobra command wiring and versioning.
  - `internal/config` for TOML config parsing.
  - `internal/chat` for chat session storage.
  - `internal/provider` for AI provider integrations.
  - `internal/whisper` for voice support (requires the `whisper` build tag).
- Default configuration is in `configs/default.toml` and is copied to `~/.config/zop/config.toml`.

## Build, lint, and test
Run these before and after making changes:
```sh
go vet ./...
go test ./...
go build ./...
```
CI runs `go test -race ./...` (with coverage on Linux). If you touch concurrency-sensitive code, consider running the race-enabled tests locally.

## Optional Whisper build
Whisper support is behind the `whisper` build tag and depends on `whisper.cpp`:
```sh
go build -tags whisper -o zop ./cmd/zop
```
Set `ZOP_WHISPER_MODEL` if you need to exercise voice input.

## Manual verification ideas
- Basic CLI: `zop "Hello"` (requires a configured provider/API key).
- Chat sessions: `zop --chat demo "Start"` then `zop --chat demo "Follow up"`.
- Config changes: validate with `configs/default.toml` and ensure TOML parsing still succeeds.

## Errors encountered during onboarding
- None. `go vet ./...`, `go test ./...`, and `go build ./...` completed successfully without workarounds.

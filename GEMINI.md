# Gemini AI Context & Contributor Guide

This document provides architectural context, conventions, and operational guidance for AI agents (like Gemini CLI) working on the `zop` codebase.

## Project Overview
`zop` is a multi-provider AI assistant available as both a CLI and a mobile application (built with Fyne). It supports OpenAI, Anthropic, Google Gemini, and OpenAI-compatible backends (Ollama, OpenRouter).

## Core Architecture

### 1. The Controller (`internal/app/controller.go`)
The `Controller` is the central orchestrator used by both the CLI and Mobile UI.
- **Responsibility**: Manages configuration, initializes providers, maintains chat history/sessions, and executes the **Tool Calling Loop**.
- **Pattern**: Most features should be implemented in the `Controller` or internal packages to ensure parity between CLI and Mobile.

### 2. Provider Interface (`internal/provider/`)
- **Interface**: Defined in `provider.go`.
- **Tool Calling**: All providers must map their native tool/function calling structures to the internal `provider.Tool` and `provider.ToolCall` types.
- **Gotchas**: 
    - OpenAI uses `ToolCallID` for tool results.
    - Anthropic uses content blocks (text, tool_use, tool_result).
    - Google Gemini uses a distinct `functionCall` and `functionResponse` structure within message parts.

### 3. Tool & MCP System (`internal/tool/`, `internal/mcp/`)
- **Registry**: `tool.Registry` handles built-in and MCP-sourced tools.
- **Built-in Tools**: e.g., `run_command` in `internal/tool/tool.go`.
- **MCP**: Connected via stdio using `mark3labs/mcp-go`. MCP servers are defined in the config and tools are wrapped into the registry during provider reload.

## Development Workflow

### Toolchain & Build
- **Makefile**: Always use the `Makefile` for setup and builds.
- **Go Version**: Defined in `go.mod`. Use `make setup-go` if the environment lacks the correct version.
- **CGO/Whisper**: Voice support requires CGO and `whisper.cpp`. The `Makefile` handles fetching and building the C++ library.
- **Build Tags**: Use `-tags whisper` for voice-enabled builds.

### Testing Strategy
- **Mocking**: Use the `MockProvider` pattern found in `internal/app/controller_test.go` to test tool calling loops without making API calls.
- **Regression**: Always run `go test ./...` to ensure changes to the `Provider` interface don't break existing backends.

## Common Pitfalls
- **Config Sync**: When adding new configuration sections (like `mcp_servers`), update both `internal/config/config.go` (structs and defaults) and `configs/default.toml`.
- **Session Names**: The `chat` package validates session names (no spaces, special chars). Use `sanitizeSessionNamePart` in the CLI.
- **Context Overflows**: The CLI implements a `rolloverSession` logic. If implementing new interaction patterns, ensure context limit errors are handled.

## Convention & Style
- **Surgical Edits**: Prefer small, targeted changes over large refactors.
- **Idiomatic Go**: Follow standard Go formatting and error handling (`fmt.Errorf("context: %w", err)`).
- **No Side Effects**: Avoid package-level state; prefer passing dependencies through constructors.

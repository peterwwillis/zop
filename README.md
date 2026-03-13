# pgpt
PowerGPT: A CLI tool for AI users

## Overview

`pgpt` is a multi-provider AI CLI tool written in Go. It supports OpenAI, Anthropic,
Google Gemini, OpenRouter, and Ollama (any OpenAI-compatible endpoint), and optionally
voice input via a Whisper build tag.

## Features

- **Multiple providers**: OpenAI, Anthropic (Claude), Google (Gemini), OpenRouter, Ollama
- **TOML config**: Define multiple named *agents*, *providers*, and *models* in `~/.config/pgpt/config.toml`
- **Chat sessions**: Persistent multi-turn conversations stored locally
- **Streaming**: Real-time token streaming via `--stream`
- **Voice input** *(optional build)*: `--voice` flag for microphone input via Whisper

## Installation

```sh
go install github.com/peterwwillis/pgpt/cmd/pgpt@latest
```

Or download a pre-built binary from the [Releases](https://github.com/peterwwillis/pgpt/releases) page.

## Quick Start

```sh
# Simple query (uses "default" agent from config)
pgpt "What is the capital of France?"

# Pipe from stdin
echo "Explain recursion" | pgpt

# Use a specific agent
pgpt --agent claude "Summarise this text"

# Multi-turn chat session
pgpt --chat my-chat "Start a conversation"
pgpt --chat my-chat "Follow up question"

# Stream the response
pgpt --stream "Write a haiku about Go"

# Voice input (requires whisper build tag)
pgpt --voice
```

## Configuration

Copy the built-in default config as a starting point:

```sh
mkdir -p ~/.config/pgpt
cp configs/default.toml ~/.config/pgpt/config.toml
```

Then set your API key environment variables:

```sh
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GOOGLE_API_KEY="..."
export OPENROUTER_API_KEY="..."
```

### Config Structure

```toml
# Define named agents (provider + model pairings)
[agents.default]
provider = "openai"
model    = "gpt4o"

[agents.claude]
provider = "anthropic"
model    = "claude-sonnet"

# Provider connection details
[providers.openai]
api_key_env = "OPENAI_API_KEY"

[providers.ollama]
base_url = "http://localhost:11434/v1"  # no API key required

# Model hyperparameters
[models.gpt4o]
model_id    = "gpt-4o"
max_tokens  = 4096
temperature = 1.0
top_p       = 1.0
```

See [`configs/default.toml`](configs/default.toml) for the full set of built-in agents,
providers, and models.

## Chat Sessions

```sh
pgpt chat list              # list all sessions
pgpt chat show my-chat      # show messages in a session
pgpt chat delete my-chat    # delete a session
```

## Building with Whisper Support

Whisper voice-input support is gated behind a build tag so that users who
don't need it avoid the CGo dependency:

```sh
go build -tags whisper -o pgpt ./cmd/pgpt
```

Requires the [whisper.cpp](https://github.com/ggerganov/whisper.cpp) library
and a Whisper model file.  Set `PGPT_WHISPER_MODEL` to point to the model path
(default: `~/.local/share/pgpt/whisper/ggml-base.en.bin`).

## Development

```sh
go build ./...
go test ./...
go vet ./...
```

## License

MIT – see [LICENSE](LICENSE).

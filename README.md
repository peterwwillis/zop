# zop
zop: A CLI tool for AI users

## Overview

`zop` is a multi-provider AI CLI tool written in Go. It supports OpenAI, Anthropic,
Google Gemini, OpenRouter, and Ollama (any OpenAI-compatible endpoint), plus
voice input via Whisper in whisper-enabled builds.

## Features

- **Multiple providers**: OpenAI, Anthropic (Claude), Google (Gemini), OpenRouter, Ollama
- **TOML config**: Define multiple named *agents*, *providers*, and *models* in `~/.config/zop/config.toml`
- **Chat sessions**: Persistent multi-turn conversations stored locally
- **Streaming**: Real-time token streaming via `--stream`
- **Voice input** *(whisper-enabled builds)*: `--voice` flag for microphone input via Whisper

## Installation

```sh
go install github.com/peterwwillis/zop/cmd/zop@latest
```

Or download a pre-built binary from the [Releases](https://github.com/peterwwillis/zop/releases) page.

## Quick Start

```sh
# Simple query (uses "default" agent from config)
zop -p "What is the capital of France?"

# Positional prompt still works
zop "What is the capital of France?"

# Pipe from stdin
echo "Explain recursion" | zop

# Use a specific agent
zop --agent claude "Summarise this text"

# Multi-turn chat session
zop --chat my-chat "Start a conversation"
zop --chat my-chat "Follow up question"

# Interactive chat session
zop --interactive --chat my-chat

# Stream the response
zop --stream "Write a haiku about Go"

# Voice input (whisper-enabled build)
zop --voice
```

## Configuration

On first run, zop writes the default config to `~/.config/zop/config.toml`.
You can also copy the built-in default config as a starting point:

```sh
mkdir -p ~/.config/zop
cp configs/default.toml ~/.config/zop/config.toml
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
system_prompt = "You are a helpful assistant."

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
system_prompt = "You are a helpful assistant."
```

See [`configs/default.toml`](configs/default.toml) for the full set of built-in agents,
providers, and models.

### Config Commands

```sh
zop config list
zop config get agents.default
zop config get models.gpt4o.max_tokens
zop config set models.gpt4o.max_tokens 2048
zop config unset models.gpt4o.system_prompt
zop config remove agents.temp
zop config edit
```

## Chat Sessions

```sh
zop chat list              # list all sessions
zop chat show my-chat      # show messages in a session
zop chat delete my-chat    # delete a session
```

## Building with Whisper Support

Whisper voice-input support is enabled in whisper-capable release binaries. To
build from source with Whisper support enabled:

```sh
go build -tags whisper -o zop ./cmd/zop
```

Requires the [whisper.cpp](https://github.com/ggerganov/whisper.cpp) library
and a Whisper model file. Set `ZOP_WHISPER_MODEL` to point to the model path
(default: `~/.local/share/zop/whisper/ggml-base.en.bin`).

To build a smaller binary without Whisper support, omit the build tag (or grab
release artifacts suffixed with `-nowhisper`).

## Mobile Roadmap

See [ANDROID_FYNE_PLAN.md](ANDROID_FYNE_PLAN.md) for the design and implementation
plan for the Android (Fyne) port, including the development checklist and build
workflow.

## Development

```sh
go build ./...
go test ./...
go vet ./...
```

## License

MIT – see [LICENSE](LICENSE).

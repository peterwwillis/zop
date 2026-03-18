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

# Voice input with VAD debug diagnostics
zop --voice --debug
```

Whisper's native initialization logs are suppressed by default and are shown
when `--debug` is enabled.

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

## Building from Source

### Prerequisites

- **GNU Make**
- **curl** and **tar** / **unzip** (for Go toolchain download)
- **cmake** (for whisper-enabled builds)
- **Docker** (for Android APK builds via `fyne-cross`)

### 1 — Install Go

If Go is not already installed, the Makefile can download and install the
version declared in `go.mod` automatically:

```sh
make setup-go
```

The toolchain is installed to `~/.local/go/<version>/` and is activated
automatically for all subsequent `make` targets — no shell changes required.

To install a specific version or to a custom path:

```sh
make setup-go GO_VERSION=1.23.0
make setup-go GO_INSTALL_DIR=/opt/go/1.24.0
```

### 2 — Build

The default build includes **Whisper voice support** (fetches and compiles
`whisper.cpp` automatically):

```sh
make deps    # download Go module dependencies
make build   # build all packages with -tags whisper
make build-local  # build runnable local binary at ./zop
```

Or run both in one shot:

```sh
make deps build
```

To build **without** Whisper:

```sh
make build BUILD_TAGS="" CGO_ENABLED=0
```

### 3 — Test & Vet

```sh
make test                                          # with whisper (default)
make test TEST_ARGS="-race -coverprofile=out.cov"  # with race detector + coverage
make vet                                           # go vet (with whisper by default)

make test BUILD_TAGS="" CGO_ENABLED=0              # without whisper
```

### 4 — Release binary

Build a standalone binary for the current host:

```sh
make build-bin VERSION=v1.2.3
```

Cross-compile (whisper-enabled requires native CGO, so cross-compilation only
works cleanly for the no-whisper variant):

```sh
# Whisper-enabled — must run on the target platform
make build-bin GOOS=linux GOARCH=arm64 VERSION=v1.2.3

# No-whisper — cross-compilation works anywhere
make build-bin GOOS=linux GOARCH=amd64 BUILD_TAGS="" CGO_ENABLED=0 BINARY_SUFFIX=-nowhisper VERSION=v1.2.3
```

The output binary is named `zop-<os>-<arch>[<suffix>][.exe]` by default;
override with `BINARY=<name>`.

### 5 — Android APK

Requires Docker (used internally by `fyne-cross`):

```sh
go install github.com/fyne-io/fyne-cross@latest
make android-apk
```

Output: `zop-android-arm64.apk`

### Makefile quick reference

| Target | Description |
|---|---|
| `make setup-go` | Download & install Go from `go.mod` |
| `make deps` | `go mod download` |
| `make build` | Build all packages (whisper by default) |
| `make build-local` | Build local runnable CLI binary at `./zop` |
| `make test` | Run tests (whisper by default) |
| `make vet` | Run `go vet` |
| `make build-bin` | Build release binary for current platform |
| `make whisper-fetch` | Clone & compile `whisper.cpp` |
| `make whisper-clean` | Remove `whisper.cpp` build tree |
| `make android-apk` | Build Android APK via `fyne-cross` |
| `make setup-go-clean` | Remove the installed Go toolchain |

All variables (`GO_VERSION`, `BUILD_TAGS`, `CGO_ENABLED`, `GOOS`, `GOARCH`,
`VERSION`, …) can be overridden on the command line.

## Building with Whisper Support

Whisper support is enabled by default when building with `make`. To build
manually with raw `go`:

```sh
make whisper-fetch                      # clone + compile whisper.cpp
CGO_ENABLED=1 go build -tags whisper -o zop ./cmd/zop
```

Set `ZOP_WHISPER_MODEL` to override the model path
(default: `~/.local/share/zop/whisper/ggml-base.en.bin`).

To build a smaller binary without Whisper, omit the tag, or grab
release artifacts suffixed with `-nowhisper`.

## Mobile Roadmap

See [ANDROID_FYNE_PLAN.md](ANDROID_FYNE_PLAN.md) for the design and implementation
plan for the Android (Fyne) port, including the development checklist and build
workflow.

## Android App (Fyne)

Download the `zop-android-arm64.apk` asset from the latest GitHub Release, or
build it locally:

```sh
go install github.com/fyne-io/fyne-cross@latest
make android-apk
```

### Install on a physical Android device (e.g., Samsung Galaxy S10e)

1. Enable Developer Options on the device and turn on **USB debugging**.
2. Install the Android Platform Tools (`adb`) on your workstation.
3. Connect the phone over USB and approve the debugging prompt.
4. Verify the device is visible:
   ```sh
   adb devices
   ```
5. Install (or update) the APK:
   ```sh
   adb install -r /path/to/zop-android-arm64.apk
   ```

To uninstall later:
```sh
adb uninstall com.zop.app
```

## Development

```sh
make deps build test vet
```

See [Building from Source](#building-from-source) for the full workflow.

## License

MIT – see [LICENSE](LICENSE).

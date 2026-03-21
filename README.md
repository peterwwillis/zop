# zop
zop: A CLI tool for AI users

## Overview

`zop` is a multi-provider AI CLI tool written in Go. It supports OpenAI, Anthropic,
Google Gemini, OpenRouter, and Ollama (any OpenAI-compatible endpoint), plus
voice input via Whisper in whisper-enabled builds.

## Features

- **Multiple providers**: OpenAI, Anthropic (Claude), Google (Gemini), OpenRouter, Ollama
- **TOML config**: Define multiple named *agents*, *providers*, *models*, and *MCP servers* in `~/.config/zop/config.toml`
- **Tool Calling**: Models can execute tools (on by default if `allow_list` is populated; use `--no-tools` to disable)
- **Model Context Protocol (MCP)**: Connect to external tools via MCP servers
- **Instruction Autoloading**: Automatically loads `ZOP.md` from the config directory as global instructions
- **Chat sessions**: Persistent multi-turn conversations stored locally
- **Streaming**: Real-time token streaming via `--stream`
- **Voice input** *(whisper-enabled builds)*: `--voice` flag for microphone input via Whisper
- **Voice transcription**: Transcription output shown during voice input (with `--verbose`)
- **Voice output** *(tts-enabled builds)*: `--tts` flag for offline speech output via Piper/Sherpa-ONNX
- **Wake Word mode**: Hands-free interactive mode using `--wake-word` and `--stop-word`

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

# Interactive mode with automatic session resume
zop --interactive

# Stream the response
zop --stream "Write a haiku about Go"

# Disable tool calling support (enabled by default if allow_list is populated)
zop --no-tools "How are you?"

# Voice input (whisper-enabled build)
zop --voice

# Voice input with VAD debug diagnostics
zop --voice --debug

# Voice input with manual send (disable silence auto-stop)
zop --voice --voice-manual

# Voice output (tts-enabled build)
zop --tts "Hello world"

# Interactive mode with both voice input and output
zop -iVt

# Customize TTS speed and turn safety delay
zop -iVt --tts-speed 1.2 --tts-delay 2000

# Wake Word mode (starts in "sleeping" state)
zop -iVt --wake-word "hey zop" --stop-word "goodbye"
```

Whisper's native initialization logs are suppressed by default and are shown
when `--debug` is enabled.

In interactive mode, `zop` now manages sessions automatically when `--chat` is
not provided: it creates a unique auto-session on first run and resumes that
same auto-session on the next interactive run. User-created sessions (via
`--chat`) remain separate and are never auto-resumed.

When an interactive conversation exceeds provider context limits, `zop`
automatically starts a new session and retries the turn.

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
disable_tools = false

[agents.claude]
provider = "anthropic"
model    = "claude-sonnet"

# Provider connection details
[providers.openai]
api_key_env = "OPENAI_API_KEY"

[providers.ollama]
base_url = "http://localhost:11434/v1"  # no API key required

# MCP Servers (optional)
[mcp_servers.sqlite]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-sqlite", "--db", "zop.db"]

# Model hyperparameters
[models.gpt4o]
model_id    = "gpt-4o"
max_tokens  = 4096
temperature = 1.0
top_p       = 1.0
system_prompt = "You are a helpful assistant."

# TTS settings
[tts]
speed = 1.0
safety_delay_ms = 1000
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

## Tool Calling & MCP

`zop` supports tool calling, allowing models to interact with the external world.

### Built-in Tools
- **`run_command`**: Execute a shell command. The model can request to run commands to perform tasks or gather information. *Note: Commands are executed automatically by the CLI.*

### Model Context Protocol (MCP)
`zop` supports the [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) for connecting to external tool servers.

To use MCP, add `mcp_servers` to your `config.toml`:

```toml
[mcp_servers.sqlite]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-sqlite", "--db", "zop.db"]

[mcp_servers.everything]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-everything"]

[mcp_servers.remote]
url = "http://localhost:8080/mcp/sse"
```

Tools provided by these servers will be automatically registered and made available to models that support tool calling (OpenAI, Anthropic, Google Gemini) in both the CLI and Mobile UI.

## Tool Call Security Policies

Tool calling is **enabled by default** whenever you have an `allow_list` populated in your configuration. Models will only see tool definitions that they are permitted to use.

### Disabling Tool Calling
If you wish to prevent models from using tools entirely:
- **CLI Flag**: Use `--no-tools` (or `-T`) to disable tools for a single command.
- **Configuration**: Set `disable_tools = true` globally or per-agent in your `config.toml`.

`zop` provides a flexible security policy system for tool calls, allowing you to control which commands the `run_command` tool is allowed to execute. Policies can be defined globally or overridden per-agent.

A policy consists of an `allow_list`, a `deny_list`, and tag-based filtering. **By default, no tools are allowed to run.** You must populate the `allow_list` to grant permission for specific tools or commands.

### Configuration

You can manage policies using the `config` CLI command or by editing `config.toml`:

```sh
# Allow a specific shell command
zop config set tool_policy.my-allow-rule.exact '["ls", "-la"]'

# Allow an MCP tool by name
zop config set tool_policy.mcp-rule.tool "everything.list_files"
```

Add `tool_policy` to your `config.toml`:

```toml
[tool_policy]
# Global tags filtering
deny_tags = ["dangerous", "network"]
allow_tags = ["safe"]

# Allow specific commands and MCP tools
allow_list = [
    # Shell command (run_command)
    { tool = "run_command", exact = ["ls", "-la"], tags = ["safe", "fs"] },
    
    # MCP Tool by name
    { tool = "sqlite.query", tags = ["safe"] },
    
    # MCP Tool with argument filtering (Regex on the JSON arguments string)
    { tool = "everything.read_file", regex = "notes.txt" }
]

# Deny specific tools
deny_list = [
    { tool = "everything.delete_file" },
    { tool = "run_command", regex = ".*;.*" }
]

# Per-agent overrides
[agents.restricted]
provider = "openai"
model = "gpt4o"
[agents.restricted.tool_policy]
allow_list = [{ tool = "run_command", exact = ["ls"] }]
```

### Entry Types
- **`tool`**: The name of the tool (e.g., `run_command`, `everything.list_files`). If omitted, `run_command` is assumed.
- **`exact`**: (For `run_command`) An array of strings representing the program and its arguments.
- **`regex`**: A regular expression that must match the command string (for `run_command`) or the JSON arguments string (for other tools).
- **`regex_array`**: (For `run_command`) An array of regular expressions matching parts of the command.
- **`tags`**: A list of labels associated with the entry.

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

The default build includes **Voice support** (fetches and compiles
`whisper.cpp` and `sherpa-onnx` automatically):

```sh
make deps    # download Go module dependencies
make build   # build all packages with -tags "whisper tts"

```

Or run both in one shot:

```sh
make deps build
```

To build **without** voice support:

```sh
make build BUILD_TAGS="" CGO_ENABLED=0
```

### 3 — Test & Vet

```sh
make test                                          # with voice (default)
make test TEST_ARGS="-race -coverprofile=out.cov"  # with race detector + coverage
make vet                                           # go vet (with voice by default)

make test BUILD_TAGS="" CGO_ENABLED=0              # without voice
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

# Fully static Linux build (recommended for Linux releases)
make build-static

# No-voice — cross-compilation works anywhere
make build-bin GOOS=linux GOARCH=amd64 BUILD_TAGS="" CGO_ENABLED=0 BINARY_SUFFIX=-novoice VERSION=v1.2.3
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
| `make build` | Build all packages (voice enabled by default) |

| `make test` | Run tests (voice enabled by default) |
| `make vet` | Run `go vet` |
| `make build-bin` | Build release binary for current platform |
| `make build-static` | Build a fully static Linux binary |
| `make whisper-fetch` | Clone & compile `whisper.cpp` |
| `make whisper-clean` | Remove `whisper.cpp` build tree |
| `make tts-fetch` | Clone & compile `sherpa-onnx` |
| `make tts-clean` | Remove `sherpa-onnx` build tree |
| `make android-apk` | Build Android APK via `fyne-cross` |
| `make setup-go-clean` | Remove the installed Go toolchain |

All variables (`GO_VERSION`, `BUILD_TAGS`, `CGO_ENABLED`, `GOOS`, `GOARCH`,
`VERSION`, …) can be overridden on the command line.

## Building with Voice Support

Voice support (input and output) is enabled by default when building with `make`.
To build manually with raw `go`:

```sh
make whisper-fetch                      # clone + compile whisper.cpp
make tts-fetch                          # clone + compile sherpa-onnx
CGO_ENABLED=1 go build -tags "whisper tts" -o zop ./cmd/zop
```

### Voice Models

- **Input (Whisper)**: Set `ZOP_WHISPER_MODEL` to override the model path
  (default: `~/.local/share/zop/whisper/ggml-base.en.bin`).
- **Output (TTS)**: Set `ZOP_TTS_MODEL` to override the base model directory
  (default: `~/.local/share/zop/tts/`).

To build a smaller binary without voice, omit the tags, or grab
release artifacts suffixed with `-novoice`.

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

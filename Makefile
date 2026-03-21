# ==============================================================
# Host platform detection
# Used for Go toolchain download and as fallback defaults for GOOS/GOARCH.
# Detected from the OS environment so that targets work before Go is installed.
# ==============================================================

ifeq ($(OS),Windows_NT)
  # Native Windows (cmd.exe / PowerShell / Git Bash on Windows)
  _HOST_OS   := windows
  _HOST_ARCH := $(if $(filter AMD64,$(PROCESSOR_ARCHITECTURE)),amd64,\
                 $(if $(filter ARM64,$(PROCESSOR_ARCHITECTURE)),arm64,amd64))
  _HOME      := $(or $(HOME),$(USERPROFILE))
else
  _HOST_OS   := $(shell uname -s | tr '[:upper:]' '[:lower:]' | \
                  sed 's/mingw.*/windows/;s/cygwin.*/windows/;s/msys.*/windows/')
  _HOST_ARCH := $(shell uname -m | \
                  sed 's/x86_64/amd64/;s/aarch64/arm64/')
  _HOME      := $(HOME)
endif

# ==============================================================
# Go toolchain installation
# ==============================================================

# Go version to install — read from go.mod; override with GO_VERSION=x.y.z
GO_VERSION     ?= $(shell sed -n 's/^go //p' go.mod)
# Where to unpack the toolchain. Versioned so multiple releases coexist.
GO_INSTALL_DIR ?= $(_HOME)/.local/go/$(GO_VERSION)

_go_ext := $(if $(filter windows,$(_HOST_OS)),.zip,.tar.gz)
_go_pkg := go$(GO_VERSION).$(_HOST_OS)-$(_HOST_ARCH)$(_go_ext)
_go_url := https://go.dev/dl/$(_go_pkg)

# Prepend the installed Go bin dir to PATH for every recipe.
# If setup-go has not been run yet, the dir won't exist and make falls
# back to whatever 'go' is already on the system PATH.
export PATH := $(GO_INSTALL_DIR)/bin:$(PATH)

# ==============================================================
# Variables — all overridable from the command line or environment
# ==============================================================

WHISPER_CPP_VERSION ?= v1.8.3
WHISPER_CPP_DIR     ?= internal/whisper/whisper.cpp
SHERPA_ONNX_VERSION ?= v1.10.43
SHERPA_ONNX_DIR     ?= internal/tts/_lib

# Version injected into the binary; override with e.g. make build-bin VERSION=v1.2.3
VERSION      ?= dev

ANDROID_APP_ID ?= com.zop.app
ANDROID_ARCH   ?= arm64

# Build tags — defaults to whisper and tts; override with BUILD_TAGS="" for a minimal build
BUILD_TAGS   ?= whisper tts
# Optional: extra arguments forwarded to go test (e.g. TEST_ARGS="-race -coverprofile=coverage.out")
TEST_ARGS    ?=
# Appended to the release binary name before the OS extension (e.g. BINARY_SUFFIX=-nowhisper)
BINARY_SUFFIX ?=

# CGO required for whisper and tts; override with CGO_ENABLED=0 when BUILD_TAGS is empty
CGO_ENABLED ?= 1

# Default to the host platform so that build-bin works without Go pre-installed.
GOOS   ?= $(_HOST_OS)
GOARCH ?= $(_HOST_ARCH)
# Optional ARM version (e.g. GOARM=7); leave empty for arm64
GOARM  ?=

# ==============================================================
# Computed variables (lazily evaluated so command-line overrides apply)
# ==============================================================

# Full binary name derived from the target platform; can be overridden wholesale.
# Examples:
#   zop-linux-amd64
#   zop-linux-arm64-nowhisper
#   zop-windows-amd64.exe
_binary_name = zop-$(GOOS)-$(GOARCH)$(if $(GOARM),v$(GOARM),)$(BINARY_SUFFIX)$(if $(filter windows,$(GOOS)),.exe,)
BINARY ?= $(_binary_name)

# -tags flag, empty when BUILD_TAGS is unset
_tag_args = $(if $(BUILD_TAGS),-tags "$(BUILD_TAGS)",)

# Prerequisite that pulls in whisper-fetch whenever the whisper tag is active.
# Evaluates to empty when BUILD_TAGS does not contain 'whisper', so overriding
# BUILD_TAGS="" also drops the whisper-fetch dependency automatically.
_whisper_dep = $(if $(filter whisper,$(BUILD_TAGS)),whisper-fetch,)
_tts_dep     = $(if $(filter tts,$(BUILD_TAGS)),tts-fetch,)

# ==============================================================
# Phony targets
# ==============================================================

.PHONY: all deps vet build clean test \
        whisper-fetch whisper-clean \
        tts-fetch tts-clean \
        vet-whisper build-whisper test-whisper \
        build-bin build-bin-clean android-apk screenshot \
        setup-go setup-go-clean go-env

all: build

# ==============================================================
# Standard Go targets
# ==============================================================

clean: whisper-clean tts-clean build-bin-clean

## deps: Download Go module dependencies
deps:
	go mod download

## vet: Run go vet (respects BUILD_TAGS; fetches whisper.cpp when BUILD_TAGS=whisper)
vet: $(_whisper_dep) $(_tts_dep)
	CGO_ENABLED=$(CGO_ENABLED) go vet $(_tag_args) ./...

## build: Compile all packages (respects BUILD_TAGS; fetches whisper.cpp when BUILD_TAGS=whisper)
build: $(_whisper_dep) $(_tts_dep)
	CGO_ENABLED=$(CGO_ENABLED) go build $(_tag_args) ./...

## test: Run tests (respects BUILD_TAGS and TEST_ARGS; fetches whisper.cpp when BUILD_TAGS=whisper)
##   Example: make test TEST_ARGS="-race -coverprofile=coverage.out"
test: $(_whisper_dep) $(_tts_dep)
	CGO_ENABLED=$(CGO_ENABLED) go test $(_tag_args) $(TEST_ARGS) ./...

# ==============================================================
# Whisper.cpp support
# ==============================================================

# Sentinel files let make skip steps that are already done.
$(WHISPER_CPP_DIR)/.git:
	git clone --depth 1 --branch $(WHISPER_CPP_VERSION) \
		https://github.com/ggerganov/whisper.cpp $(WHISPER_CPP_DIR)

$(WHISPER_CPP_DIR)/build/src/libwhisper.a: $(WHISPER_CPP_DIR)/.git
	cmake \
		-S $(WHISPER_CPP_DIR) \
		-B $(WHISPER_CPP_DIR)/build \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_BUILD_SERVER=OFF
	cmake --build $(WHISPER_CPP_DIR)/build --target whisper

## whisper-fetch: Clone and build whisper.cpp (idempotent via sentinel files)
whisper-fetch: $(WHISPER_CPP_DIR)/build/src/libwhisper.a

## whisper-clean: Remove the whisper.cpp source and build tree
whisper-clean:
	rm -rf $(WHISPER_CPP_DIR)

## vet-whisper: go vet with the whisper build tag
vet-whisper: whisper-fetch
	go vet -tags whisper ./...

## build-whisper: Build all packages with the whisper build tag
build-whisper: whisper-fetch
	CGO_ENABLED=1 go build -tags whisper ./...

## test-whisper: Run tests with the whisper build tag (includes race detector)
test-whisper: whisper-fetch
	CGO_ENABLED=1 go test -race -tags whisper ./...

# ==============================================================
# Sherpa-onnx support
# ==============================================================

$(SHERPA_ONNX_DIR)/.git:
	git clone --depth 1 --branch $(SHERPA_ONNX_VERSION) \
		https://github.com/k2-fsa/sherpa-onnx $(SHERPA_ONNX_DIR)

$(SHERPA_ONNX_DIR)/build/lib/libsherpa-onnx-c-api.a: $(SHERPA_ONNX_DIR)/.git
	cmake \
		-S $(SHERPA_ONNX_DIR) \
		-B $(SHERPA_ONNX_DIR)/build \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DSHERPA_ONNX_ENABLE_PYTHON=OFF \
		-DSHERPA_ONNX_ENABLE_TESTS=OFF \
		-DSHERPA_ONNX_ENABLE_CHECK=OFF \
		-DSHERPA_ONNX_ENABLE_C_API=ON \
		-DSHERPA_ONNX_ENABLE_WASM=OFF \
		-DSHERPA_ONNX_ENABLE_BINARY=OFF \
		-DSHERPA_ONNX_ENABLE_GPU=OFF \
		-DSHERPA_ONNX_ENABLE_SPEAKER_DIARIZATION=OFF \
		-DSHERPA_ONNX_ENABLE_PORTAUDIO=OFF \
		-DSHERPA_ONNX_ENABLE_WEBSOCKET=OFF \
		-DSHERPA_ONNX_BUILD_C_API_EXAMPLES=OFF
	cmake --build $(SHERPA_ONNX_DIR)/build --target sherpa-onnx-c-api

## tts-fetch: Clone and build sherpa-onnx (idempotent via sentinel files)
tts-fetch: $(SHERPA_ONNX_DIR)/build/lib/libsherpa-onnx-c-api.a
	mkdir -p $(SHERPA_ONNX_DIR)/include
	mkdir -p $(SHERPA_ONNX_DIR)/build/lib
	cp $(SHERPA_ONNX_DIR)/sherpa-onnx/c-api/c-api.h $(SHERPA_ONNX_DIR)/include/
	find $(SHERPA_ONNX_DIR)/build -name "*.a" -exec cp {} $(SHERPA_ONNX_DIR)/build/lib/ \;

## tts-clean: Remove the sherpa-onnx source and build tree
tts-clean:
	rm -rf $(SHERPA_ONNX_DIR)

# ==============================================================
# Release build
# ==============================================================

## build-bin: Build the zop CLI binary for a specific platform.
##   Defaults to the current host platform; override any variable as needed.
##   Examples:
##     make build-bin
##     make build-bin GOOS=linux GOARCH=amd64 CGO_ENABLED=1 BUILD_TAGS=whisper VERSION=v1.0.0
##     make build-bin GOOS=linux GOARCH=amd64 CGO_ENABLED=0 BINARY_SUFFIX=-nowhisper VERSION=v1.0.0
##     make build-bin BINARY=custom-name VERSION=v1.0.0
build-bin:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) GOARM=$(GOARM) \
	go build \
		$(_tag_args) \
		-ldflags="-s -w -X github.com/peterwwillis/zop/internal/cli.Version=$(VERSION)" \
		-o $(BINARY) \
		./cmd/zop

## build-static: Build a fully static CLI binary (Linux only, requires static system libs).
build-static: $(_whisper_dep) $(_tts_dep)
	CGO_ENABLED=1 GOOS=linux GOARCH=$(GOARCH) \
	go build \
		$(_tag_args) \
		-ldflags='-s -w -extldflags "-static" -X github.com/peterwwillis/zop/internal/cli.Version=$(VERSION)' \
		-o $(BINARY)-static \
		./cmd/zop

build-bin-clean:
	rm -f zop-*

# ==============================================================
# Android APK
# ==============================================================

## android-apk: Build Android APK via fyne-cross; requires Docker.
##   Output: zop-android-$(ANDROID_ARCH).apk
android-apk:
	fyne-cross android \
		-app-id $(ANDROID_APP_ID) \
		-arch $(ANDROID_ARCH) \
		./cmd/zop-mobile
	@APK_PATH=$$(find fyne-cross/dist -type f -name "*.apk" -print -quit); \
	if [ -z "$$APK_PATH" ]; then \
		echo "APK not found under fyne-cross/dist" >&2; exit 1; \
	fi; \
	cp "$$APK_PATH" "zop-android-$(ANDROID_ARCH).apk"

# ==============================================================
# Mobile UI screenshot
# ==============================================================

## screenshot: Capture a Fyne UI screenshot (output path via ZOP_SCREENSHOT_PATH)
screenshot:
	go test -tags fyne ./internal/mobileui -run TestScreenshot -count=1

# ==============================================================
# Go toolchain setup (equivalent to actions/setup-go)
# ==============================================================

## setup-go: Download and install the Go version from go.mod.
##   Installs to GO_INSTALL_DIR (default: $$HOME/.local/go/<version>).
##   Supports Linux, macOS, and Windows (Git Bash / WSL).
##   Activate the installed toolchain in your shell with:
##     eval $$(make go-env -s)
setup-go:
	@if [ -x "$(GO_INSTALL_DIR)/bin/go" ] || [ -x "$(GO_INSTALL_DIR)/bin/go.exe" ]; then \
		echo "Go $(GO_VERSION) already installed at $(GO_INSTALL_DIR)"; \
	else \
		echo "Downloading Go $(GO_VERSION) ($(_HOST_OS)/$(_HOST_ARCH))..."; \
		mkdir -p "$(GO_INSTALL_DIR)"; \
		tmpdir=$$(mktemp -d); \
		tmpfile="$$tmpdir/go$(_go_ext)"; \
		curl -fsSL "$(_go_url)" -o "$$tmpfile"; \
		if [ "$(_go_ext)" = ".zip" ]; then \
			unzip -q "$$tmpfile" -d "$$tmpdir"; \
			cp -r "$$tmpdir/go/." "$(GO_INSTALL_DIR)/"; \
		else \
			tar -xz -C "$(GO_INSTALL_DIR)" --strip-components=1 -f "$$tmpfile"; \
		fi; \
		rm -rf "$$tmpdir"; \
		echo "Go $(GO_VERSION) installed to $(GO_INSTALL_DIR)"; \
		echo "Activate with: eval \$$(make go-env -s)"; \
	fi

## setup-go-clean: Remove the Go toolchain installed by setup-go
setup-go-clean:
	rm -rf "$(GO_INSTALL_DIR)"

## go-env: Print shell export lines to activate the installed Go toolchain.
##   Usage: eval $$(make go-env -s)
##   The -s flag suppresses make's own output so only the exports are printed.
go-env:
	@printf 'export PATH="%s/bin:$$PATH"\n' "$(GO_INSTALL_DIR)"
	@printf 'export GOROOT="%s"\n' "$(GO_INSTALL_DIR)"

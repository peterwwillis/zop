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
                  sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv6l/armv6l/;s/armv7l/armv7l/')
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

# ==============================================================
# Variables — all overridable from the command line or environment
# ==============================================================

WHISPER_CPP_VERSION ?= v1.8.3
WHISPER_CPP_DIR     ?= internal/whisper/whisper.cpp

# Version injected into the binary; override with e.g. make build-bin VERSION=v1.2.3
VERSION      ?= dev

ANDROID_APP_ID ?= com.zop.app
ANDROID_ARCH   ?= arm64

# Optional: space-separated build tags (e.g. BUILD_TAGS=whisper)
BUILD_TAGS   ?=
# Optional: extra arguments forwarded to go test (e.g. TEST_ARGS="-race -coverprofile=coverage.out")
TEST_ARGS    ?=
# Appended to the release binary name before the OS extension (e.g. BINARY_SUFFIX=-nowhisper)
BINARY_SUFFIX ?=

CGO_ENABLED ?= 0

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
_tag_args = $(if $(BUILD_TAGS),-tags $(BUILD_TAGS),)

# ==============================================================
# Phony targets
# ==============================================================

.PHONY: all deps vet build test \
        whisper-fetch whisper-clean \
        vet-whisper build-whisper test-whisper \
        build-bin android-apk screenshot \
        setup-go setup-go-clean go-env

all: build

# ==============================================================
# Standard Go targets
# ==============================================================

## deps: Download Go module dependencies
deps:
	go mod download

## vet: Run go vet (respects BUILD_TAGS)
vet:
	go vet $(_tag_args) ./...

## build: Compile all packages (respects BUILD_TAGS)
build:
	go build $(_tag_args) ./...

## test: Run tests (respects BUILD_TAGS and TEST_ARGS)
##   Example: make test TEST_ARGS="-race -coverprofile=coverage.out"
test:
	go test $(_tag_args) $(TEST_ARGS) ./...

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
# Release build
# ==============================================================

## build-bin: Build the zop CLI binary for a specific platform.
##   Defaults to the current host platform; override any variable as needed.
##
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

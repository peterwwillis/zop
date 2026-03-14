//go:build whisper

// Package whisper provides voice-to-text input using Whisper.
// This file is compiled only when the "whisper" build tag is set.
//
// Build with: go build -tags whisper
//
// This implementation uses the whisper.cpp C library via CGo.
// See https://github.com/ggerganov/whisper.cpp for installation instructions.
// The whisper model file path can be set with ZOP_WHISPER_MODEL (defaults to
// ~/.local/share/zop/whisper/ggml-base.en.bin).
// If the model file does not exist at that path, it is downloaded automatically
// from https://huggingface.co/ggerganov/whisper.cpp.
package whisper

// #cgo CFLAGS: -I${SRCDIR}/whisper.cpp
// #cgo LDFLAGS: -L${SRCDIR}/whisper.cpp -lwhisper -lm -lstdc++
// #include "whisper.h"
import "C"

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

// defaultModelURL is where the base English Whisper model is fetched when the
// local file is absent.
const defaultModelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"

// defaultModelPath returns the path where the Whisper model should live.
// Override with ZOP_WHISPER_MODEL.
func defaultModelPath() string {
	if p := os.Getenv("ZOP_WHISPER_MODEL"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ggml-base.en.bin"
	}
	return filepath.Join(home, ".local", "share", "zop", "whisper", "ggml-base.en.bin")
}

// ensureModel makes sure the model file exists at path.  If it does not, it
// downloads it from defaultModelURL and saves it to path.
func ensureModel(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already present
	}

	fmt.Fprintf(os.Stderr, "[zop] Whisper model not found at %q – downloading from %s …\n", path, defaultModelURL)

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating model directory: %w", err)
	}

	// Write to a temp file first so a partial download is never used.
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()

	resp, err := http.Get(defaultModelURL) //nolint:gosec // URL is a compile-time constant
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("downloading model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_ = f.Close()
		return fmt.Errorf("downloading model: HTTP %d from %s", resp.StatusCode, defaultModelURL)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing model: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing model file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("installing model: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[zop] Whisper model saved to %q\n", path)
	return nil
}

// RecordAndTranscribe records audio from the default microphone for up to 30
// seconds (stopped when the user presses Enter) and transcribes it using the
// local Whisper model.  If the model file does not exist it is downloaded first.
//
// NOTE: Audio capture is platform-specific and must be adapted for each OS.
// This stub shows the Whisper integration; callers must supply PCM audio data.
func RecordAndTranscribe() (string, error) {
	modelPath := defaultModelPath()

	if err := ensureModel(modelPath); err != nil {
		return "", fmt.Errorf("whisper model setup: %w", err)
	}

	cModel := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cModel))

	params := C.whisper_context_default_params()
	ctx := C.whisper_init_from_file_with_params(cModel, params)
	if ctx == nil {
		return "", fmt.Errorf("failed to initialize whisper context from %q", modelPath)
	}
	defer C.whisper_free(ctx)

	fmt.Fprintln(os.Stderr, "Recording… press Ctrl-C or wait for silence.")
	// In a real implementation, capture PCM audio here.
	// For now we sleep briefly to demonstrate compilation works.
	time.Sleep(100 * time.Millisecond)

	return "", fmt.Errorf("whisper audio capture not yet implemented on this platform")
}

//go:build whisper

// Package whisper provides voice-to-text input using Whisper.
// This file is compiled only when the "whisper" build tag is set.
//
// Build with: go build -tags whisper
//
// This implementation uses the whisper.cpp C library via CGo.
// See https://github.com/ggerganov/whisper.cpp for installation instructions.
// The whisper model file path can be set with PGPT_WHISPER_MODEL (defaults to
// ~/.local/share/pgpt/whisper/ggml-base.en.bin).
package whisper

// #cgo CFLAGS: -I${SRCDIR}/whisper.cpp
// #cgo LDFLAGS: -L${SRCDIR}/whisper.cpp -lwhisper -lm -lstdc++
// #include "whisper.h"
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

// defaultModelPath returns the default path to the Whisper model file.
func defaultModelPath() string {
	if p := os.Getenv("PGPT_WHISPER_MODEL"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ggml-base.en.bin"
	}
	return filepath.Join(home, ".local", "share", "pgpt", "whisper", "ggml-base.en.bin")
}

// RecordAndTranscribe records audio from the default microphone for up to 30
// seconds (stopped when the user presses Enter) and transcribes it using the
// local Whisper model.
//
// NOTE: Audio capture is platform-specific and must be adapted for each OS.
// This stub shows the Whisper integration; callers must supply PCM audio data.
func RecordAndTranscribe() (string, error) {
	modelPath := defaultModelPath()
	if _, err := os.Stat(modelPath); err != nil {
		return "", fmt.Errorf("whisper model not found at %q: %w\n"+
			"Download a model from https://huggingface.co/ggerganov/whisper.cpp", modelPath, err)
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

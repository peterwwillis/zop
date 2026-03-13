//go:build !whisper

// Package whisper provides voice-to-text input using Whisper.
// This stub is compiled when the "whisper" build tag is NOT set.
// To enable Whisper support, build with: go build -tags whisper
package whisper

import "errors"

// ErrNotBuiltIn is returned when Whisper support was not compiled in.
var ErrNotBuiltIn = errors.New("whisper support not compiled in (rebuild with -tags whisper)")

// RecordAndTranscribe records audio from the microphone and returns the
// transcribed text.  This stub always returns ErrNotBuiltIn.
func RecordAndTranscribe() (string, error) {
	return "", ErrNotBuiltIn
}

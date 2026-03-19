package tts

import (
	"context"

	"github.com/peterwwillis/zop/internal/config"
)

// Speaker is the interface for text-to-speech output.
type Speaker interface {
	// Speak converts text to speech and plays it.
	Speak(ctx context.Context, text string) error
	// SetSpeed sets the speech speed (1.0 is normal).
	SetSpeed(speed float32)
	// Wait waits for all queued audio to finish playing.
	Wait() error
	// Close releases resources.
	Close() error
}

// NewSpeaker returns a new Speaker instance if the "tts" build tag is set.
// Otherwise it returns a stub that does nothing.
func NewSpeaker(cfg config.TTSConfig) (Speaker, error) {
	return newSpeaker(cfg)
}

// DownloadProgress is a callback for model download progress.
type DownloadProgress func(string)

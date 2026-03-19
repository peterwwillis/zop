//go:build !tts

package tts

import (
	"context"
	"fmt"

	"github.com/peterwwillis/zop/internal/config"
)

type stubSpeaker struct{}

func (s *stubSpeaker) Speak(ctx context.Context, text string) error {
	return fmt.Errorf("voice output is not enabled (build with -tags tts)")
}

func (s *stubSpeaker) SetSpeed(speed float32) {}

func (s *stubSpeaker) Wait() error {
	return nil
}

func (s *stubSpeaker) Close() error {
	return nil
}

func newSpeaker(cfg config.TTSConfig) (Speaker, error) {
	return &stubSpeaker{}, nil
}

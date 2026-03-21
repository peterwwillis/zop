//go:build tts

package tts

import (
	"context"
	"testing"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSpeaker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	s, err := NewSpeaker(config.TTSConfig{})
	if err != nil {
		t.Skipf("skipping test because NewSpeaker failed: %v", err)
		return
	}
	require.NotNil(t, s)

	ctx := context.Background()
	err = s.Speak(ctx, "Test")
	require.NoError(t, err)

	err = s.Wait()
	require.NoError(t, err)

	_ = s.Close()
}

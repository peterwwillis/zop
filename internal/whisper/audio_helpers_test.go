package whisper

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBytesToInt16(t *testing.T) {
	t.Parallel()

	out := bytesToInt16([]byte{0x01, 0x00, 0xff, 0x7f, 0x00, 0x80, 0xee})

	require.Equal(t, []int16{1, 32767, -32768}, out)
}

func TestHasSpeech(t *testing.T) {
	t.Parallel()

	require.False(t, hasSpeech([]int16{0, 100, -250, 699}, 700))
	require.True(t, hasSpeech([]int16{-700}, 700))
	require.True(t, hasSpeech([]int16{-32768}, 700))
}

func TestRMSAmplitude(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0.0, rmsAmplitude(nil))
	require.Equal(t, 0.0, rmsAmplitude([]int16{0, 0, 0}))
	require.InDelta(t, 100.0, rmsAmplitude([]int16{100, -100, 100, -100}), 1e-9)
}

func TestRMSAmplitudeFloat32(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0.0, rmsAmplitudeFloat32(nil))
	require.Equal(t, 0.0, rmsAmplitudeFloat32([]float32{0, 0, 0}))
	require.InDelta(t, 0.5, rmsAmplitudeFloat32([]float32{0.5, -0.5, 0.5, -0.5}), 1e-7)
	require.InDelta(t, 0.0, rmsAmplitudeFloat32([]float32{0.5, 0.5, 0.5, 0.5}), 1e-9)
	require.InDelta(t, 0.5, rmsAmplitudeFloat32([]float32{0.6, -0.4, 0.6, -0.4}), 1e-7)
}

func TestInt16ToPCMFloat(t *testing.T) {
	t.Parallel()

	out := int16ToPCMFloat([]int16{-32768, -16384, 0, 16384, 32767})

	require.Len(t, out, 5)
	require.InDelta(t, -1.0, out[0], 1e-6)
	require.InDelta(t, -0.5, out[1], 1e-6)
	require.Equal(t, float32(0), out[2])
	require.InDelta(t, 0.5, out[3], 1e-6)
	require.InDelta(t, 1.0-(1.0/32768.0), out[4], 1e-6)
	require.False(t, math.IsNaN(float64(out[4])))
}

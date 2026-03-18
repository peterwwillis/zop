package whisper

func bytesToInt16(raw []byte) []int16 {
	n := len(raw) / 2
	out := make([]int16, n)
	for i := range n {
		j := i * 2
		out[i] = int16(raw[j]) | int16(raw[j+1])<<8
	}
	return out
}

func hasSpeech(samples []int16, threshold int16) bool {
	for _, sample := range samples {
		// Convert to int32 before taking abs so -32768 is handled correctly.
		v := int32(sample)
		if v < 0 {
			v = -v
		}
		if v >= int32(threshold) {
			return true
		}
	}
	return false
}

func int16ToPCMFloat(samples []int16) []float32 {
	out := make([]float32, len(samples))
	// Whisper expects normalized float PCM samples in [-1.0, 1.0). int16 audio
	// spans [-32768, 32767], so divide by 32768.
	const denom = 32768.0
	for i, sample := range samples {
		out[i] = float32(sample) / denom
	}
	return out
}

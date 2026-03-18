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

// hasSpeech reports whether the RMS amplitude of samples meets or exceeds
// threshold. Using RMS rather than peak amplitude prevents isolated noise
// spikes from resetting the silence timer on every frame.
//
// The comparison is done in squared units to avoid a sqrt:
//
//	rms >= threshold  ⟺  sum(s²)/len >= threshold²
func hasSpeech(samples []int16, threshold int16) bool {
	if len(samples) == 0 {
		return false
	}
	var sum int64
	for _, s := range samples {
		v := int64(s)
		sum += v * v
	}
	t := int64(threshold)
	return sum >= t*t*int64(len(samples))
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

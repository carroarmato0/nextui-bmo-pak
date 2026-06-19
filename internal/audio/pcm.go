package audio

// S16LEToFloat32 converts little-endian signed 16-bit PCM to float32 samples
// in [-1, 1). A trailing odd byte is ignored. Mono is assumed; the wake-word
// path captures DefaultChannels (1) at DefaultSampleRate (16000).
func S16LEToFloat32(pcm []byte) []float32 {
	n := len(pcm) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(uint16(pcm[2*i]) | uint16(pcm[2*i+1])<<8)
		out[i] = float32(v) / 32768
	}
	return out
}

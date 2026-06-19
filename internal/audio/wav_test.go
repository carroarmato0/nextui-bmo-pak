package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildTestWAV writes a canonical 44-byte little-endian S16LE WAV header
// followed by the PCM payload.
func buildTestWAV(pcm []byte, rate, channels int) []byte {
	bitsPerSample := 16
	blockAlign := channels * bitsPerSample / 8
	byteRate := rate * blockAlign
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&b, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bitsPerSample))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

func TestDecodeWAV(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	t.Run("mono 22050", func(t *testing.T) {
		got, rate, ch, ok := DecodeWAV(buildTestWAV(pcm, 22050, 1))
		if !ok {
			t.Fatalf("ok = false, want true")
		}
		if rate != 22050 || ch != 1 {
			t.Fatalf("rate=%d ch=%d, want 22050 1", rate, ch)
		}
		if !bytes.Equal(got, pcm) {
			t.Fatalf("pcm = %v, want %v", got, pcm)
		}
	})

	t.Run("stereo 24000", func(t *testing.T) {
		got, rate, ch, ok := DecodeWAV(buildTestWAV(pcm, 24000, 2))
		if !ok || rate != 24000 || ch != 2 || !bytes.Equal(got, pcm) {
			t.Fatalf("got %v rate=%d ch=%d ok=%v", got, rate, ch, ok)
		}
	})

	t.Run("raw pcm is not a WAV", func(t *testing.T) {
		if _, _, _, ok := DecodeWAV(pcm); ok {
			t.Fatalf("ok = true for raw PCM, want false")
		}
	})

	t.Run("too short", func(t *testing.T) {
		if _, _, _, ok := DecodeWAV([]byte("RIFF")); ok {
			t.Fatalf("ok = true for short buffer, want false")
		}
	})

	t.Run("8-bit unsupported", func(t *testing.T) {
		w := buildTestWAV(pcm, 16000, 1)
		w[34] = 8 // bitsPerSample low byte
		if _, _, _, ok := DecodeWAV(w); ok {
			t.Fatalf("ok = true for 8-bit, want false")
		}
	})
}

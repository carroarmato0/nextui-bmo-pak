package audio

import "encoding/binary"

// DecodeWAV parses a little-endian 16-bit PCM WAV (RIFF/WAVE) container and
// returns the raw S16LE sample data with its sample rate and channel count.
// ok is false if the input is not a valid 16-bit PCM WAV.
func DecodeWAV(b []byte) (pcm []byte, sampleRate, channels int, ok bool) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, 0, false
	}
	var (
		data              []byte
		rate, chans, bits int
		haveFmt, haveData bool
	)
	off := 12
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if body+size > len(b) {
			size = len(b) - body
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				chans = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
				rate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
				bits = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
				haveFmt = true
			}
		case "data":
			data = b[body : body+size]
			haveData = true
		}
		off = body + size
		if size%2 == 1 {
			off++ // chunks are word-aligned; skip pad byte on odd size
		}
	}
	if !haveFmt || !haveData || bits != 16 || chans <= 0 || rate <= 0 {
		return nil, 0, 0, false
	}
	return data, rate, chans, true
}

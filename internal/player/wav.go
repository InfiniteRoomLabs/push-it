package player

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DecodeWAV parses a RIFF/WAVE file containing 16-bit PCM.
func DecodeWAV(r io.Reader) (*Clip, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("wav: not a RIFF/WAVE file")
	}
	var rate, channels int
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			return nil, errors.New("wav: truncated chunk")
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("wav: short fmt chunk")
			}
			format := binary.LittleEndian.Uint16(data[body:])
			channels = int(binary.LittleEndian.Uint16(data[body+2:]))
			rate = int(binary.LittleEndian.Uint32(data[body+4:]))
			bits := binary.LittleEndian.Uint16(data[body+14:])
			if format != 1 || bits != 16 {
				return nil, fmt.Errorf("wav: unsupported format tag %d / %d-bit (need 16-bit PCM)", format, bits)
			}
		case "data":
			if rate == 0 {
				return nil, errors.New("wav: data chunk before fmt chunk")
			}
			pcm := make([]byte, size)
			copy(pcm, data[body:body+size])
			return &Clip{PCM: pcm, SampleRate: rate, Channels: channels}, nil
		}
		pos = body + size + size%2 // chunks are word-aligned
	}
	return nil, errors.New("wav: no data chunk")
}

// EncodeWAV writes c as a canonical 44-byte-header 16-bit PCM WAV.
func EncodeWAV(w io.Writer, c *Clip) error {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(c.PCM)))
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], uint16(c.Channels))
	binary.LittleEndian.PutUint32(h[24:], uint32(c.SampleRate))
	binary.LittleEndian.PutUint32(h[28:], uint32(c.SampleRate*c.Channels*2))
	binary.LittleEndian.PutUint16(h[32:], uint16(c.Channels*2))
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(c.PCM)))
	if _, err := w.Write(h); err != nil {
		return err
	}
	_, err := w.Write(c.PCM)
	return err
}

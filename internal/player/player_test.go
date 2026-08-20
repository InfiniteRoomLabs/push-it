package player

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sine builds a mono 16-bit sine wave clip.
func sine(rate int, d time.Duration, hz float64) *Clip {
	n := int(float64(rate) * d.Seconds())
	pcm := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := int16(math.Sin(2*math.Pi*hz*float64(i)/float64(rate)) * 16000)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}
	return &Clip{PCM: pcm, SampleRate: rate, Channels: 1}
}

func TestDuration(t *testing.T) {
	c := sine(8000, 250*time.Millisecond, 440)
	if c.Duration() != 250*time.Millisecond {
		t.Fatalf("Duration = %v", c.Duration())
	}
}

func TestWAVRoundTrip(t *testing.T) {
	c := sine(8000, 100*time.Millisecond, 440)
	var buf bytes.Buffer
	if err := EncodeWAV(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeWAV(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.SampleRate != 8000 || got.Channels != 1 || !bytes.Equal(got.PCM, c.PCM) {
		t.Fatalf("round trip mismatch: rate=%d ch=%d len=%d", got.SampleRate, got.Channels, len(got.PCM))
	}
}

func TestDecodeWAVRejectsNonPCM(t *testing.T) {
	c := sine(8000, 10*time.Millisecond, 440)
	var buf bytes.Buffer
	_ = EncodeWAV(&buf, c)
	b := buf.Bytes()
	binary.LittleEndian.PutUint16(b[20:], 3) // format tag 3 = IEEE float
	if _, err := DecodeWAV(bytes.NewReader(b)); err == nil {
		t.Fatal("expected error for non-PCM WAV")
	}
}

func TestDecodeWAVRejectsGarbage(t *testing.T) {
	if _, err := DecodeWAV(bytes.NewReader([]byte("hello"))); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeWAVRejectsZeroChannels(t *testing.T) {
	c := &Clip{PCM: []byte{0, 0}, SampleRate: 8000, Channels: 0}
	var buf bytes.Buffer
	if err := EncodeWAV(&buf, c); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeWAV(&buf); err == nil {
		t.Fatal("expected error for zero channels")
	}
}

func TestSlice(t *testing.T) {
	c := sine(1000, time.Second, 10)
	s := c.Slice(250*time.Millisecond, 750*time.Millisecond)
	if s.Duration() != 500*time.Millisecond {
		t.Fatalf("slice duration = %v", s.Duration())
	}
	if !bytes.Equal(s.PCM, c.PCM[500:1500]) {
		t.Fatal("slice bytes wrong")
	}
	if c.Slice(-time.Second, 10*time.Second).Duration() != time.Second {
		t.Fatal("slice should clamp to clip bounds")
	}
}

func TestSliceZeroChannelsDoesNotPanic(t *testing.T) {
	s := (&Clip{}).Slice(0, time.Second)
	if s.Duration() != 0 {
		t.Fatalf("Duration = %v, want 0", s.Duration())
	}
}

func TestDecodeMP3Fixture(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "silence.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := DecodeMP3(f)
	if err != nil {
		t.Fatal(err)
	}
	if c.SampleRate != 44100 || c.Channels != 2 {
		t.Fatalf("rate=%d ch=%d", c.SampleRate, c.Channels)
	}
	if d := c.Duration(); d < 50*time.Millisecond || d > 200*time.Millisecond {
		t.Fatalf("duration = %v, want ~100ms", d)
	}
}

func TestDecodeByExtension(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Clip.WAV")
	var buf bytes.Buffer
	_ = EncodeWAV(&buf, sine(8000, 10*time.Millisecond, 440))
	_ = os.WriteFile(p, buf.Bytes(), 0o644)
	if _, err := Decode(p); err != nil {
		t.Fatal(err)
	}
	oggPath := filepath.Join(dir, "x.ogg")
	_ = os.WriteFile(oggPath, []byte("not actually ogg"), 0o644)
	_, err := Decode(oggPath)
	if err == nil {
		t.Fatal("expected unsupported extension error")
	}
	if !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "unsupported file type")
	}
}

package clipper

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func w(s string, start, end float64) Word { return Word{Word: s, Start: start, End: end} }

func TestGroupFindsPhrases(t *testing.T) {
	words := []Word{
		w("ooh", 0, 0.3), w("baby", 0.3, 0.6),
		w("push", 1.0, 1.2), w("it", 1.2, 1.4),
		w("push", 1.5, 1.7), w("it", 1.7, 1.9), w("real", 1.9, 2.1), w("good!", 2.1, 2.4),
		w("yeah", 3.5, 3.8),
		w("push", 5.0, 5.2), w("it", 5.2, 5.4),
		w("push", 6.5, 6.7), w("them", 6.7, 6.9), // not a phrase start
		w("push", 8.0, 8.2), w("it", 9.0, 9.2), // gap too big between push and it
	}
	got := Group(words, DefaultOptions())
	if len(got) != 2 {
		t.Fatalf("got %d phrases: %+v", len(got), got)
	}
	if got[0].Label != "push it push it real good" || got[0].Start != 1.0 || got[0].End != 2.4 || got[0].ID != 1 {
		t.Fatalf("phrase 1 = %+v", got[0])
	}
	if got[1].Label != "push it" || got[1].Start != 5.0 || got[1].ID != 2 {
		t.Fatalf("phrase 2 = %+v", got[1])
	}
}

func TestGroupRespectsMaxDuration(t *testing.T) {
	var words []Word
	for i := 0; i < 20; i++ {
		s := float64(i) * 0.5
		words = append(words, w("push", s, s+0.2), w("it", s+0.2, s+0.4))
	}
	got := Group(words, Options{Phrase: []string{"push", "it"}, Gap: 0.5, Max: 2.0})
	if len(got) < 2 {
		t.Fatalf("expected the run to be split by Max, got %d", len(got))
	}
	for _, p := range got {
		if p.End-p.Start > 2.0 {
			t.Fatalf("phrase exceeds max: %+v", p)
		}
	}
}

func tone(rate int, d time.Duration) *player.Clip {
	return &player.Clip{PCM: make([]byte, int(float64(rate)*d.Seconds())*2), SampleRate: rate, Channels: 1}
}

func TestCutWritesWavsAndManifest(t *testing.T) {
	out := t.TempDir()
	src := tone(8000, 10*time.Second)
	phrases := []Phrase{{ID: 1, Start: 1.0, End: 2.0, Label: "push it"}, {ID: 2, Start: 5.0, End: 5.5, Label: "push it real good"}}
	got, err := Cut(src, phrases, 0.3, out)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].File != "001-push-it.wav" || got[1].File != "002-push-it-real-good.wav" {
		t.Fatalf("files = %q %q", got[0].File, got[1].File)
	}
	c, err := player.Decode(filepath.Join(out, got[0].File))
	if err != nil {
		t.Fatal(err)
	}
	if d := c.Duration(); d < 1590*time.Millisecond || d > 1610*time.Millisecond {
		t.Fatalf("clip 1 duration = %v, want 1.6s (1s + 2x0.3 pad)", d)
	}
	var manifest []Phrase
	data, _ := os.ReadFile(filepath.Join(out, "candidates.json"))
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest) != 2 || manifest[1].File != got[1].File {
		t.Fatalf("manifest: %v %+v", err, manifest)
	}
}

func TestReviewKeepsAndSkips(t *testing.T) {
	cand, keep := t.TempDir(), t.TempDir()
	src := tone(8000, 3*time.Second)
	_, err := Cut(src, []Phrase{{ID: 1, Start: 0, End: 0.5, Label: "push it"}, {ID: 2, Start: 1, End: 1.5, Label: "push it"}, {ID: 3, Start: 2, End: 2.5, Label: "push it"}}, 0, cand)
	if err != nil {
		t.Fatal(err)
	}
	plays := 0
	in := strings.NewReader("k\nr\ns\nq\n")
	var out bytes.Buffer
	kept, err := Review(in, &out, func(*player.Clip) error { plays++; return nil }, cand, keep)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 || plays != 4 { // clip1 play, clip2 play + replay, skip, then clip3 play, quit
		t.Fatalf("kept=%d plays=%d", kept, plays)
	}
	if _, err := os.Stat(filepath.Join(keep, "001-push-it.wav")); err != nil {
		t.Fatal("keeper not moved")
	}
	if _, err := os.Stat(filepath.Join(cand, "001-push-it.wav")); err == nil {
		t.Fatal("keeper should be moved, not copied")
	}
}

func TestReviewResumesPastAlreadyReviewed(t *testing.T) {
	cand, keep, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	src := tone(8000, 3*time.Second)
	_, err := Cut(src, []Phrase{{ID: 1, Start: 0, End: 0.5, Label: "push it"}, {ID: 2, Start: 1, End: 1.5, Label: "push it"}, {ID: 3, Start: 2, End: 2.5, Label: "push it"}}, 0, cand)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a prior run having already moved the first candidate out.
	if err := os.Rename(filepath.Join(cand, "001-push-it.wav"), filepath.Join(elsewhere, "001-push-it.wav")); err != nil {
		t.Fatal(err)
	}
	plays := 0
	in := strings.NewReader("s\ns\n")
	var out bytes.Buffer
	kept, err := Review(in, &out, func(*player.Clip) error { plays++; return nil }, cand, keep)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 0 || plays != 2 {
		t.Fatalf("kept=%d plays=%d", kept, plays)
	}
}

func TestFileNameSanitizesLabel(t *testing.T) {
	got := fileName(Phrase{ID: 1, Label: `push it 24/7 "now"`})
	if got != "001-push-it-24-7-now.wav" {
		t.Fatalf("fileName = %q", got)
	}
}

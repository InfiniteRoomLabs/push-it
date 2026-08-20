package clips

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "b.mp3"))
	touch(t, filepath.Join(dir, "a.WAV"))
	touch(t, filepath.Join(dir, "notes.txt"))
	touch(t, filepath.Join(dir, "candidates.json"))
	if err := os.Mkdir(filepath.Join(dir, "nested.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "a.WAV" || filepath.Base(got[1]) != "b.mp3" {
		t.Fatalf("List = %v", got)
	}
	if !filepath.IsAbs(got[0]) {
		t.Fatalf("List entry not absolute: %q", got[0])
	}
}

func TestListMissingDirIsEmpty(t *testing.T) {
	got, err := List(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPickIsDeterministicWithSeed(t *testing.T) {
	files := []string{"a", "b", "c", "d"}
	r1 := rand.New(rand.NewPCG(1, 2))
	r2 := rand.New(rand.NewPCG(1, 2))
	p1, err := Pick(files, r1)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	p2, err := Pick(files, r2)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if p1 != p2 {
		t.Fatalf("same seed, different picks: %q %q", p1, p2)
	}
}

// TestPickConsultsRNG pins the exact draw sequence for a fixed seed. A Pick
// that ignores r (e.g. always returning files[0]) would produce a constant
// sequence and fail this after the first divergent draw.
func TestPickConsultsRNG(t *testing.T) {
	files := []string{"a", "b", "c", "d"}
	r := rand.New(rand.NewPCG(1, 2))
	want := []string{"a", "a", "a", "c", "a", "a", "b", "c"}
	for i, w := range want {
		got, err := Pick(files, r)
		if err != nil {
			t.Fatalf("Pick() call %d error = %v", i, err)
		}
		if got != w {
			t.Fatalf("Pick() call %d = %q, want %q", i, got, w)
		}
	}
}

// TestPickUniformity is a smoke test that every candidate can be drawn, not
// just a single fixed one.
func TestPickUniformity(t *testing.T) {
	files := []string{"a", "b", "c", "d"}
	r := rand.New(rand.NewPCG(1, 2))
	seen := make(map[string]bool, len(files))
	for i := 0; i < 200; i++ {
		got, err := Pick(files, r)
		if err != nil {
			t.Fatalf("Pick() call %d error = %v", i, err)
		}
		seen[got] = true
	}
	for _, f := range files {
		if !seen[f] {
			t.Fatalf("file %q never picked in 200 draws", f)
		}
	}
}

func TestPickEmpty(t *testing.T) {
	if _, err := Pick(nil, rand.New(rand.NewPCG(0, 0))); err != ErrNoClips {
		t.Fatalf("err = %v, want ErrNoClips", err)
	}
}

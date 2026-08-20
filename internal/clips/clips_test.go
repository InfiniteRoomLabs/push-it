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
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "a.WAV" || filepath.Base(got[1]) != "b.mp3" {
		t.Fatalf("List = %v", got)
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
	p1, _ := Pick(files, r1)
	p2, _ := Pick(files, r2)
	if p1 != p2 {
		t.Fatalf("same seed, different picks: %q %q", p1, p2)
	}
}

func TestPickEmpty(t *testing.T) {
	if _, err := Pick(nil, rand.New(rand.NewPCG(0, 0))); err != ErrNoClips {
		t.Fatalf("err = %v, want ErrNoClips", err)
	}
}

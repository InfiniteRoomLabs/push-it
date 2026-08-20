package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	rel, ok, err := Acquire(p, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	if _, _, err := Acquire(p, time.Minute); err != nil {
		t.Fatal(err)
	} else if _, ok2, _ := Acquire(p, time.Minute); ok2 {
		t.Fatal("second acquire should fail while held")
	}
	rel()
	if _, ok3, _ := Acquire(p, time.Minute); !ok3 {
		t.Fatal("acquire after release should succeed")
	}
}

func TestStaleLockIsTakenOver(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	_ = os.WriteFile(p, nil, 0o600)
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(p, old, old)
	_, ok, err := Acquire(p, time.Minute)
	if err != nil || !ok {
		t.Fatalf("stale takeover: ok=%v err=%v", ok, err)
	}
}

func TestReleaseDoesNotRemoveForeignLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	rel, ok, err := Acquire(p, time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(p, []byte("someone-else"), 0o600); err != nil {
		t.Fatal(err)
	}
	rel()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("release removed a foreign lock: %v", err)
	}
}

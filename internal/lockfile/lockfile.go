// Package lockfile provides a tiny cross-platform mutual-exclusion file.
package lockfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"time"
)

// Acquire creates path exclusively. It returns ok=false if another process
// holds a fresh lock. A lock older than stale is removed and retaken.
//
// Stale takeover is advisory, not atomic: two processes racing at the exact
// moment a stale lock expires can both decide to take it over and both
// succeed. The returned release never removes a lock it does not own - each
// acquired lock is stamped with a random ownership token, and release only
// unlinks the file if that token is still there.
func Acquire(path string, stale time.Duration) (release func(), ok bool, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			token, terr := newToken()
			if terr != nil {
				_ = f.Close()
				_ = os.Remove(path)
				return nil, false, terr
			}
			_, werr := f.WriteString(token)
			_ = f.Close()
			if werr != nil {
				_ = os.Remove(path)
				return nil, false, werr
			}
			return func() { releaseIfOwned(path, token) }, true, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, false, err
		}
		fi, serr := os.Stat(path)
		if serr == nil && time.Since(fi.ModTime()) > stale {
			_ = os.Remove(path)
			continue
		}
		return nil, false, nil
	}
	return nil, false, nil
}

// newToken returns a random hex-encoded ownership token.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// releaseIfOwned removes path only if its content still matches token.
// Read/compare errors are ignored - best effort.
func releaseIfOwned(path, token string) {
	got, err := os.ReadFile(path)
	if err != nil || string(got) != token {
		return
	}
	_ = os.Remove(path)
}

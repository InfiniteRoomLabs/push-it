// Package lockfile provides a tiny cross-platform mutual-exclusion file.
package lockfile

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// Acquire creates path exclusively. It returns ok=false if another process
// holds a fresh lock. A lock older than stale is removed and retaken.
func Acquire(path string, stale time.Duration) (release func(), ok bool, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, true, nil
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

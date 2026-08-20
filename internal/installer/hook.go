package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

const (
	MarkerStart = "# >>> push-it >>>"
	MarkerEnd   = "# <<< push-it <<<"
)

// Block is the shell snippet appended to pre-push. The absolute binary path
// is used because GUI git clients often run hooks with a minimal PATH.
func Block(exe string) string {
	return fmt.Sprintf("%s\n'%s' hook pre-push \"$@\" || true\n%s\n", MarkerStart, filepath.ToSlash(exe), MarkerEnd)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// WireHook makes git run push-it on pre-push, recording what it changed in st.
func WireHook(g Git, cfgDir, exe string, st *config.InstallState) error {
	hp, err := g.Get("core.hooksPath")
	if err != nil {
		return err
	}
	if hp == "" {
		dir := filepath.Join(cfgDir, "hooks")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "pre-push"), []byte("#!/bin/sh\n"+Block(exe)), 0o755); err != nil {
			return err
		}
		if err := g.Set("core.hooksPath", dir); err != nil {
			return err
		}
		st.HooksPathSetByUs, st.HooksPath, st.PrePushAppendedTo, st.PrePushCreatedByUs = true, dir, "", false
		return nil
	}
	hp = expandHome(hp)
	file := filepath.Join(hp, "pre-push")
	existing, err := os.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(hp, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file, []byte("#!/bin/sh\n"+Block(exe)), 0o755); err != nil {
			return err
		}
		st.PrePushCreatedByUs = true
	case err != nil:
		return err
	case strings.Contains(string(existing), MarkerStart):
		// already wired; idempotent
	default:
		content := string(existing)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(file, []byte(content+Block(exe)), 0o755); err != nil {
			return err
		}
	}
	st.HooksPathSetByUs, st.HooksPath, st.PrePushAppendedTo = false, hp, file
	return nil
}

// UnwireHook reverses WireHook and resets st.
func UnwireHook(g Git, st *config.InstallState) error {
	switch {
	case st.HooksPathSetByUs:
		_ = os.Remove(filepath.Join(st.HooksPath, "pre-push"))
		_ = os.Remove(st.HooksPath) // only succeeds if empty  -  never deletes user files
		if err := g.Unset("core.hooksPath"); err != nil {
			return err
		}
	case st.PrePushAppendedTo != "" && st.PrePushCreatedByUs:
		if err := os.Remove(st.PrePushAppendedTo); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	case st.PrePushAppendedTo != "":
		b, err := os.ReadFile(st.PrePushAppendedTo)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err == nil {
			if err := os.WriteFile(st.PrePushAppendedTo, []byte(stripBlock(string(b))), 0o755); err != nil {
				return err
			}
		}
	}
	st.HooksPathSetByUs, st.HooksPath, st.PrePushAppendedTo, st.PrePushCreatedByUs = false, "", "", false
	return nil
}

// stripBlock removes everything from MarkerStart through MarkerEnd inclusive
// (plus the trailing newline after MarkerEnd). WireHook only ever appends
// Block directly onto content that already ends in a newline, so this
// restores the original file byte-for-byte.
func stripBlock(s string) string {
	start := strings.Index(s, MarkerStart)
	end := strings.Index(s, MarkerEnd)
	if start < 0 || end < 0 || end < start {
		return s
	}
	end += len(MarkerEnd)
	if end < len(s) && s[end] == '\n' {
		end++
	}
	return s[:start] + s[end:]
}

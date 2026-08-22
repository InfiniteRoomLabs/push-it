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
	quoted := strings.ReplaceAll(filepath.ToSlash(exe), "'", `'\''`)
	return fmt.Sprintf("%s\n'%s' hook pre-push \"$@\" || true\n%s\n", MarkerStart, quoted, MarkerEnd)
}

// shellInterpreters is the explicit allowlist of interpreters whose syntax
// accepts the push-it block verbatim. Anything else (pwsh, fish, csh, tcsh,
// python, ...) is refused rather than risk breaking every push.
var shellInterpreters = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "ash": true,
}

// isShellInterpreter reports whether a hook starting with firstLine can have
// the push-it block appended. A file with no shebang is treated as shell
// because git runs it via sh. "#!/usr/bin/env [-S] [-flags] <interp>" is
// unwrapped to <interp>.
func isShellInterpreter(firstLine string) bool {
	if !strings.HasPrefix(firstLine, "#!") {
		return true
	}
	fields := strings.Fields(strings.TrimPrefix(firstLine, "#!"))
	if len(fields) == 0 {
		return false
	}
	interp := fields[0]
	if filepath.Base(interp) == "env" {
		interp = ""
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "-") {
				continue // -S, -i, and other env flags
			}
			interp = f
			break
		}
		if interp == "" {
			return false
		}
	}
	return shellInterpreters[filepath.Base(interp)]
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
	if st.HooksPathSetByUs || st.PrePushAppendedTo != "" {
		// Re-wiring (e.g. after the user changed core.hooksPath out from
		// under us): reverse our previous wiring first so we never orphan
		// a block in a hook we no longer track, or leave a stale
		// core.hooksPath pointing nowhere.
		if err := UnwireHook(g, st); err != nil {
			return err
		}
	}
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
	if !filepath.IsAbs(hp) {
		return fmt.Errorf("installer: core.hooksPath is relative (%q); git resolves it per repository, so push-it cannot wire a single hook - set an absolute path or unset it and re-run", hp)
	}
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
		if err := os.Chmod(file, 0o755); err != nil {
			return err
		}
		st.PrePushCreatedByUs = true
	case err != nil:
		return err
	case strings.Contains(string(existing), MarkerStart):
		// already wired; idempotent
	default:
		content := string(existing)
		firstLine := content
		if i := strings.IndexByte(content, '\n'); i >= 0 {
			firstLine = content[:i]
		}
		if !isShellInterpreter(firstLine) {
			return fmt.Errorf("installer: existing hook %s is not a shell script (%q); push-it cannot append safely - add this line to it yourself:\n%s", file, firstLine, Block(exe))
		}
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		// Add the owner exec bit only: enough for git (running as this
		// user) to execute the hook, without loosening group/other bits
		// the user may have deliberately restricted (e.g. a private 0700
		// hook must not gain group/other exec from an append).
		mode := info.Mode().Perm() | 0o100
		if err := os.WriteFile(file, []byte(content+Block(exe)), mode); err != nil {
			return err
		}
		if err := os.Chmod(file, mode); err != nil {
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
		// Only unset core.hooksPath if it's still what we set - the user may
		// have since pointed it somewhere else themselves.
		if live, err := g.Get("core.hooksPath"); err != nil {
			return err
		} else if expandHome(live) == st.HooksPath {
			if err := g.Unset("core.hooksPath"); err != nil {
				return err
			}
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

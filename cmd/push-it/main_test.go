package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestMain lets this test binary double as the detached child process that
// hook.PrePush spawns (os.Executable() is the test binary under `go test`).
// When PUSH_IT_TEST_CHILD is set, exit immediately instead of running the
// suite, so the "child" does nothing but let PrePush's already-opened log
// file prove the spawn happened.
func TestMain(m *testing.M) {
	if os.Getenv("PUSH_IT_TEST_CHILD") == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestVersionSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"version"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "push-it ") {
		t.Fatalf("stdout = %q, want prefix %q", out.String(), "push-it ")
	}
}

func TestUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"bogus"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestNoArgsPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(nil, strings.NewReader(""), &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "usage:") {
		t.Fatalf("code = %d, stderr = %q", code, errOut.String())
	}
}

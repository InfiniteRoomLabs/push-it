package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestClipsCutNoArgsFails covers `clips cut` with no positional args: it
// needs exactly SOURCE and transcript.json, so this must fail fast with a
// usage message rather than trying to open an empty path.
func TestClipsCutNoArgsFails(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"clips", "cut"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("code=%d, want 2; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("stderr = %q, want it to contain \"usage\"", errOut.String())
	}
}

// TestClipsReviewNoArgsFails covers `clips review` with no candidates
// directory named.
func TestClipsReviewNoArgsFails(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"clips", "review"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("code=%d, want 2; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

// TestClipsUnknownSubcommandFails covers `clips bogus`: an unrecognised
// clips subcommand.
func TestClipsUnknownSubcommandFails(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"clips", "bogus"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("code=%d, want 2; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

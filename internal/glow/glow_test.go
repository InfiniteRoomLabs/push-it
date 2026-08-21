package glow

import (
	"context"
	"testing"
	"time"
)

func TestStubBackendIsNoop(t *testing.T) {
	if Available() {
		t.Skip("a real backend is compiled in")
	}
	if err := Run(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	note, err := Install(nil)
	if err != nil || note != "" || Uninstall(nil) != nil {
		t.Fatal("stub install/uninstall must be no-ops")
	}
}

func TestParamsAreSane(t *testing.T) {
	if GlowWidthAt1080 <= 0 || FalloffExponent < 1 || MinOpacity >= MaxOpacity || MaxOpacity > 1 {
		t.Fatal("bad params")
	}
}

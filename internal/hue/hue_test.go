package hue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type call struct {
	method, path string
	body         map[string]any
}

func newServer(t *testing.T) (*httptest.Server, *[]call) {
	t.Helper()
	var mu sync.Mutex
	var calls []call
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
		mu.Lock()
		calls = append(calls, call{r.Method, r.URL.Path, body})
		mu.Unlock()
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"state":{"on":false,"bri":120,"hue":5000,"sat":200,"reachable":true}}`))
			return
		}
		_, _ = w.Write([]byte(`[{"success":{}}]`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func fingerprintOf(srv *httptest.Server) string {
	sum := sha256.Sum256(srv.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

// testClient uses the real pinned transport against the test server's cert.
func testClient(srv *httptest.Server) *Client {
	c := New("ignored", "KEY", 1, fingerprintOf(srv))
	c.BaseURL = srv.URL
	c.Sleep = func(time.Duration) {}
	return c
}

func TestPinnedTransportRejectsOtherCert(t *testing.T) {
	srv, _ := newServer(t)
	c := New("ignored", "KEY", 1, strings.Repeat("00", 32))
	c.BaseURL = srv.URL
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("mismatched fingerprint must fail")
	}
}

func TestFingerprintMatchesServerCert(t *testing.T) {
	srv, _ := newServer(t)
	got, err := Fingerprint(context.Background(), strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	if got != fingerprintOf(srv) {
		t.Fatalf("fingerprint = %s, want %s", got, fingerprintOf(srv))
	}
}

func TestBurstSequenceAndRestore(t *testing.T) {
	srv, calls := newServer(t)
	c := testClient(srv)
	if err := c.Burst(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := *calls
	// GET state, PUT prime, 6 hue steps, PUT restore = 9 calls
	if len(got) != 9 {
		t.Fatalf("got %d calls: %+v", len(got), got)
	}
	if got[0].method != "GET" || got[0].path != "/api/KEY/lights/1" {
		t.Fatalf("first call = %+v", got[0])
	}
	if got[1].body["hue"] != float64(0) || got[1].body["bri"] != float64(254) {
		t.Fatalf("prime = %+v", got[1].body)
	}
	for i, h := range Steps {
		if got[2+i].path != "/api/KEY/lights/1/state" || got[2+i].body["hue"] != float64(h) {
			t.Fatalf("step %d = %+v", i, got[2+i])
		}
	}
	last := got[8].body
	if last["on"] != false || last["bri"] != float64(120) || last["hue"] != float64(5000) || last["sat"] != float64(200) {
		t.Fatalf("restore = %+v", last)
	}
	if _, leaked := last["reachable"]; leaked {
		t.Fatal("restore must only send on/bri/hue/sat")
	}
}

func TestBurstFailsFastWhenBridgeDown(t *testing.T) {
	srv, _ := newServer(t)
	c := testClient(srv)
	srv.Close()
	if err := c.Burst(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestPing(t *testing.T) {
	srv, calls := newServer(t)
	if err := testClient(srv).Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %d", len(*calls))
	}
}

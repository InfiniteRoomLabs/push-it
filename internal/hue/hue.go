// Package hue runs a rainbow burst on one Philips Hue light and restores it.
package hue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Steps are the hue-wheel values (0..65535) visited in order.
var Steps = []int{10922, 21845, 32768, 43690, 54613, 65535}

// Client talks to a Hue bridge's v1 API.
type Client struct {
	BaseURL string
	Key     string
	Light   int
	HTTP    *http.Client
	Sleep   func(time.Duration)
}

// pinned returns a TLS config that accepts exactly one certificate: the one
// whose SHA-256 fingerprint is certSHA256. Hue bridges present certificates
// no public CA signs, so chain verification is replaced by this pin
// (trust-on-first-use: captured by Fingerprint at install time).
func pinned(certSHA256 string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // chain verification is replaced by the fingerprint pin below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if certSHA256 == "" {
				return errors.New("hue: no bridge certificate pinned; run push-it install --hue")
			}
			if len(rawCerts) == 0 {
				return errors.New("hue: server sent no certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			if got := hex.EncodeToString(sum[:]); got != certSHA256 {
				return fmt.Errorf("hue: bridge certificate fingerprint %s does not match pinned %s (re-run `push-it install --hue` if the bridge was replaced)", got, certSHA256)
			}
			return nil
		},
	}
}

// New returns a client for https://<bridge> pinned to certSHA256.
func New(bridge, key string, light int, certSHA256 string) *Client {
	return &Client{
		BaseURL: "https://" + bridge,
		Key:     key,
		Light:   light,
		HTTP: &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{TLSClientConfig: pinned(certSHA256)},
		},
		Sleep: time.Sleep,
	}
}

// Fingerprint connects to the bridge once and returns the SHA-256 of its
// leaf certificate, for storing as the trust-on-first-use pin.
func Fingerprint(ctx context.Context, bridge string) (string, error) {
	d := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}} // first contact: nothing to verify against yet
	host := bridge
	if _, _, err := net.SplitHostPort(bridge); err != nil {
		host = net.JoinHostPort(bridge, "443")
	}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", errors.New("hue: bridge sent no certificate")
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}

type state struct {
	On  bool `json:"on"`
	Bri int  `json:"bri"`
	Hue int  `json:"hue"`
	Sat int  `json:"sat"`
}

func (c *Client) lightURL() string {
	return fmt.Sprintf("%s/api/%s/lights/%d", c.BaseURL, c.Key, c.Light)
}

// redact replaces the API key segment of a Hue request path
// (/api/<key>/lights/...) with "<key>" so the key never lands in a log line
// or error message.
func redact(path string) string {
	parts := strings.SplitN(path, "/", 4)
	if len(parts) >= 3 && parts[1] == "api" {
		parts[2] = "<key>"
	}
	return strings.Join(parts, "/")
}

// transportErr wraps a c.HTTP.Do failure with the request path redacted -
// the Hue v1 API carries the key in the URL path, and *url.Error stringifies
// the full URL, so the raw error must never be returned as-is.
func transportErr(req *http.Request, err error) error {
	unwrapped := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		unwrapped = urlErr.Err
	}
	return fmt.Errorf("hue: %s %s: %w", req.Method, redact(req.URL.Path), unwrapped)
}

func (c *Client) get(ctx context.Context, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.lightURL(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return transportErr(req, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hue: GET light: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) put(ctx context.Context, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.lightURL()+"/state", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return transportErr(req, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hue: PUT state: %s", resp.Status)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(respBody) == 0 {
		return nil
	}
	// The Hue v1 API answers a rejected state change with HTTP 200 and an
	// array of results; a rejected element carries an "error" key instead
	// of "success", so the status code alone cannot tell success from failure.
	var results []map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &results); err != nil {
		return fmt.Errorf("hue: PUT state: unexpected response body: %w", err)
	}
	for _, r := range results {
		errRaw, ok := r["error"]
		if !ok {
			continue
		}
		var apiErr struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(errRaw, &apiErr); err == nil && apiErr.Description != "" {
			return fmt.Errorf("hue: PUT state: %s", apiErr.Description)
		}
		return fmt.Errorf("hue: PUT state: %s", string(errRaw))
	}
	return nil
}

// Ping fetches the light once to confirm bridge, key, and light ID work.
func (c *Client) Ping(ctx context.Context) error {
	var v struct {
		State state `json:"state"`
	}
	return c.get(ctx, &v)
}

// Burst saves the light's state, runs the hue wheel, and restores it.
func (c *Client) Burst(ctx context.Context) (err error) {
	var saved struct {
		State state `json:"state"`
	}
	if err := c.get(ctx, &saved); err != nil {
		return err
	}

	// If anything after this point fails, best-effort restore the saved
	// state so a mid-burst error doesn't strand the light at full
	// brightness. Use a fresh context: the caller's ctx may already be
	// cancelled or timed out, which is often exactly why we're here.
	defer func() {
		if err != nil {
			restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = c.put(restoreCtx, saved.State)
		}
	}()

	if err = c.put(ctx, map[string]any{"on": true, "bri": 254, "sat": 254, "hue": 0, "transitiontime": 0}); err != nil {
		return err
	}
	for _, h := range Steps {
		c.Sleep(450 * time.Millisecond)
		if err = c.put(ctx, map[string]any{"hue": h, "transitiontime": 3}); err != nil {
			return err
		}
	}
	c.Sleep(600 * time.Millisecond)
	if err = c.put(ctx, saved.State); err != nil {
		return err
	}
	return nil
}

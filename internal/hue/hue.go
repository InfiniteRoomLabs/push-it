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
	"net"
	"net/http"
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

func (c *Client) get(ctx context.Context, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.lightURL(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
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
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hue: PUT state: %s", resp.Status)
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
func (c *Client) Burst(ctx context.Context) error {
	var saved struct {
		State state `json:"state"`
	}
	if err := c.get(ctx, &saved); err != nil {
		return err
	}
	if err := c.put(ctx, map[string]any{"on": true, "bri": 254, "sat": 254, "hue": 0, "transitiontime": 0}); err != nil {
		return err
	}
	for _, h := range Steps {
		c.Sleep(450 * time.Millisecond)
		if err := c.put(ctx, map[string]any{"hue": h, "transitiontime": 3}); err != nil {
			return err
		}
	}
	c.Sleep(600 * time.Millisecond)
	return c.put(ctx, saved.State)
}

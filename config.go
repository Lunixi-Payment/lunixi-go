package lunixi

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config configures one merchant's client. Do not reuse a client across keys
// or merchants. HTTPClient is copied and redirects are always disabled.
type Config struct {
	BaseURL          string
	KeyID            string `json:"-"`
	PrivateKey       string `json:"-"`
	HTTPClient       *http.Client
	Timeout          time.Duration
	MaxRetries       int
	MaxResponseBytes int64
	// AllowInsecureLocalhost permits HTTP only for literal loopback addresses
	// and localhost. It is intended for local test servers, never production.
	AllowInsecureLocalhost bool
	// Now provides a deterministic clock for tests. Nil uses time.Now.
	Now func() time.Time
}

// String and GoString keep routine configuration logging from exposing keys.
func (c Config) String() string   { return "lunixi.Config{Credentials:[REDACTED]}" }
func (c Config) GoString() string { return c.String() }

// MarshalJSON intentionally excludes configuration secrets from structured logs.
func (c Config) MarshalJSON() ([]byte, error) { return []byte(`{"credentials":"[REDACTED]"}`), nil }

func validateConfig(c Config) (*url.URL, ed25519.PrivateKey, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return nil, nil, invalid("BaseURL must be an HTTPS origin")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && c.AllowInsecureLocalhost && loopback(u.Hostname())) {
		return nil, nil, invalid("BaseURL must use HTTPS")
	}
	if !safeHeader(c.KeyID, 200) || strings.TrimSpace(c.KeyID) == "" {
		return nil, nil, invalid("KeyID")
	}
	if c.Timeout < 0 || c.Timeout > 2*time.Minute || c.MaxRetries < 0 || c.MaxRetries > 5 || c.MaxResponseBytes < 0 || c.MaxResponseBytes > 32<<20 {
		return nil, nil, invalid("request limits")
	}
	key, err := parsePrivateKey(c.PrivateKey)
	return u, key, err
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parsePrivateKey(s string) (ed25519.PrivateKey, error) {
	var raw []byte
	if strings.Contains(s, "-----BEGIN") {
		block, rest := pem.Decode([]byte(s))
		if block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
			return nil, invalid("Ed25519 PKCS8 private key")
		}
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, invalid("Ed25519 PKCS8 private key")
		}
		key, ok := parsed.(ed25519.PrivateKey)
		if !ok {
			return nil, invalid("private key must be Ed25519")
		}
		return append(ed25519.PrivateKey(nil), key...), nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, invalid("base64 private key")
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(raw); err == nil {
		if key, ok := parsed.(ed25519.PrivateKey); ok {
			return append(ed25519.PrivateKey(nil), key...), nil
		}
	}
	if len(raw) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(raw), nil
	}
	if len(raw) == ed25519.PrivateKeySize {
		key := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
		if !key.Equal(ed25519.PrivateKey(raw)) {
			return nil, invalid("Ed25519 private key public half")
		}
		return key, nil
	}
	return nil, invalid("Ed25519 PKCS8 key or 32-byte seed required")
}

func safeHeader(value string, max int) bool {
	if len(value) == 0 || len(value) > max {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

package lunixi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"
)

// Test support of the wallet, bank-connect, sanction, insurance,
// subscription, KYC, identity and marketplace tests.

type productRecorded struct {
	Method, URI, Key, Accept string
	Body                     []byte
	// Signed: the request carried the per-request signature.
	Signed bool
	// Bearer: the request carried the access token.
	Bearer bool
}

// newProductClient is a signed test client; reply answers the call-th
// product request (0-based). A nil reply answers {"success":true,"data":{}}.
func newProductClient(t *testing.T, retries int, reply func(w http.ResponseWriter, r *http.Request, call int)) (*Client, func() []productRecorded) {
	t.Helper()
	var mu sync.Mutex
	var recorded []productRecorded
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"at_TEST_products","expires_in":1800}`)
			return
		}
		mu.Lock()
		call := len(recorded)
		recorded = append(recorded, productRecorded{Method: r.Method, URI: r.URL.RequestURI(), Key: r.Header.Get("Idempotency-Key"), Accept: r.Header.Get("Accept"), Body: raw, Signed: r.Header.Get("X-Signature") != "", Bearer: r.Header.Get("Authorization") == "Bearer at_TEST_products"})
		mu.Unlock()
		if reply != nil {
			reply(w, r, call)
			return
		}
		productJSON(w, 200, `{"success":true,"data":{}}`)
	}, func(c *Config) { c.MaxRetries = retries })
	return client, func() []productRecorded {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(recorded)
	}
}

func productJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprint(w, body)
}

// expectLocalError asserts ErrConfiguration and that nothing was sent.
func expectLocalError(t *testing.T, name string, err error, sent []productRecorded) {
	t.Helper()
	if !errors.Is(err, ErrConfiguration) {
		t.Fatalf("%s: want ErrConfiguration, got %v", name, err)
	}
	if len(sent) != 0 {
		t.Fatalf("%s: %d request(s) sent", name, len(sent))
	}
}

const testProductWebhookSecret = "whsec_products_test_secret_0001"

// productWebhook verifies a signed test event.
func productWebhook(t *testing.T, body string) *WebhookEvent {
	t.Helper()
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	verifier, err := NewWebhookVerifier(WebhookConfig{Secrets: []string{testProductWebhookSecret}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	event, err := verifier.Verify([]byte(body), webhookHeaders([]byte(body), "evt-1", now.Format(time.RFC3339), testProductWebhookSecret))
	if err != nil {
		t.Fatal(err)
	}
	return event
}

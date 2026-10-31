package lunixi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func webhookHeaders(body []byte, id, ts, secret string) http.Header {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(append([]byte(id+"."+ts+"."), body...))
	h := http.Header{}
	h.Set("X-Lunixi-Event-Id", id)
	h.Set("X-Lunixi-Signature-Timestamp", ts)
	h.Set("X-Lunixi-Signature", "v2="+hex.EncodeToString(mac.Sum(nil)))
	return h
}

func TestWebhookIndependentSharedVectors(t *testing.T) {
	raw, e := os.ReadFile("testdata/webhook-v2-vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var vectors []struct{ Name, Secret, EventID, Timestamp, RawBody, ExpectedV2 string }
	if e = json.Unmarshal(raw, &vectors); e != nil {
		t.Fatal(e)
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339Nano, v.Timestamp)
			verifier, e := NewWebhookVerifier(WebhookConfig{Secrets: []string{v.Secret}, Now: func() time.Time { return now }})
			if e != nil {
				t.Fatal(e)
			}
			headers := http.Header{}
			headers.Set("X-Lunixi-Event-Id", v.EventID)
			headers.Set("X-Lunixi-Signature-Timestamp", v.Timestamp)
			headers.Set("X-Lunixi-Signature", "v2="+v.ExpectedV2)
			event, e := verifier.Verify([]byte(v.RawBody), headers)
			if e != nil {
				t.Fatal(e)
			}
			if event.ID() != v.EventID {
				t.Fatal("event ID lost")
			}
			if _, e = verifier.Verify([]byte(v.RawBody+" "), headers); !errors.Is(e, ErrWebhook) {
				t.Fatal("accepted modified raw body")
			}
		})
	}
}

func TestWebhookRejectsTamperStaleFutureAndUnsignedType(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	secret := "whsec_testing_secret_1234567890"
	v, _ := NewWebhookVerifier(WebhookConfig{Secrets: []string{secret}, Now: func() time.Time { return now }})
	body := []byte(`{"id":"event-1","type":"payment.captured","data":{"amount":100,"currency":"TRY"}}`)
	for _, tc := range []struct {
		name   string
		body   []byte
		mutate func(http.Header)
	}{
		{"different-event-id", body, func(h http.Header) { h.Set("X-Lunixi-Event-Id", "event-2") }},
		{"unsigned-event-type", body, func(h http.Header) { h.Set("X-Lunixi-Event-Type", "payment.refunded") }},
		{"unknown-signature", body, func(h http.Header) { h.Set("X-Lunixi-Signature", "v1=abc") }},
		{"stale", body, func(h http.Header) {
			ts := now.Add(-6 * time.Minute).Format(time.RFC3339)
			for key, values := range webhookHeaders(body, "event-1", ts, secret) {
				h[key] = values
			}
		}},
		{"future", body, func(h http.Header) {
			ts := now.Add(6 * time.Minute).Format(time.RFC3339)
			for key, values := range webhookHeaders(body, "event-1", ts, secret) {
				h[key] = values
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := webhookHeaders(body, "event-1", now.Format(time.RFC3339), secret)
			tc.mutate(h)
			if _, e := v.Verify(tc.body, h); !errors.Is(e, ErrWebhook) {
				t.Fatal("accepted unauthenticated event")
			}
		})
	}
	wrongID := []byte(`{"id":"different","type":"payment.captured","data":{}}`)
	if _, e := v.Verify(wrongID, webhookHeaders(wrongID, "event-1", now.Format(time.RFC3339), secret)); !errors.Is(e, ErrWebhook) {
		t.Fatal("accepted header/body ID mismatch")
	}
	duplicates := []byte(`{"type":"payment.failed","type":"payment.captured"}`)
	if _, e := v.Verify(duplicates, webhookHeaders(duplicates, "event-1", now.Format(time.RFC3339), secret)); !errors.Is(e, ErrWebhook) {
		t.Fatal("accepted ambiguous event type")
	}
	missingType := []byte(`{"data":{}}`)
	h := webhookHeaders(missingType, "event-1", now.Format(time.RFC3339), secret)
	h.Set("X-Lunixi-Event-Type", "payment.captured")
	if _, e := v.Verify(missingType, h); !errors.Is(e, ErrWebhook) {
		t.Fatal("trusted unsigned event type")
	}
}

func TestWebhookRotationAndImmutability(t *testing.T) {
	secret := "whsec_previous_secret_1234567890"
	now := time.Now()
	body := []byte(`{"id":"e-1","type":"payment.captured","data":{"amount":100}}`)
	v, _ := NewWebhookVerifier(WebhookConfig{Secrets: []string{"whsec_current_secret_1234567890", secret}, Now: func() time.Time { return now }})
	h := webhookHeaders(body, "e-1", now.Format(time.RFC3339Nano), secret)
	h.Set("X-Lunixi-Signature", "v2="+strings.Repeat("0", 64)+","+h.Get("X-Lunixi-Signature"))
	e, eventErr := v.Verify(body, h)
	if eventErr != nil {
		t.Fatal(eventErr)
	}
	raw := e.Raw()
	raw[0] = '!'
	data := e.Data()
	data[0] = '!'
	if e.Raw()[0] != '{' || e.Data()[0] != '{' {
		t.Fatal("verified event exposed mutable storage")
	}
	h.Add("X-Lunixi-Event-Id", "e-1")
	if _, err := v.Verify(body, h); !errors.Is(err, ErrWebhook) {
		t.Fatal("accepted duplicate security header")
	}
}

func TestWebhookRequestGuard(t *testing.T) {
	v, _ := NewWebhookVerifier(WebhookConfig{Secrets: []string{"whsec_test_secret_1234567890"}})
	r := httptest.NewRequest(http.MethodGet, "https://merchant.example/webhook", nil)
	if _, e := v.VerifyRequest(context.Background(), r); !errors.Is(e, ErrWebhook) {
		t.Fatal("accepted GET webhook")
	}
}

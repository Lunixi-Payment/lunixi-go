package lunixi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebhookConfig verifies current and previous rotation secrets. Tolerance
// defaults to five minutes and cannot be disabled in production verification.
type WebhookConfig struct {
	Secrets   []string
	Tolerance time.Duration
	Now       func() time.Time
}

// WebhookVerifier authenticates raw request bytes. Verification does not
// consume an event ID: use a durable transactional ReplayStore before applying
// financial state, so a crash cannot lose a previously claimed event.
type WebhookVerifier struct {
	secrets   [][]byte
	tolerance time.Duration
	now       func() time.Time
}

// NewWebhookVerifier validates webhook configuration without handling data.
func NewWebhookVerifier(config WebhookConfig) (*WebhookVerifier, error) {
	if len(config.Secrets) < 1 || len(config.Secrets) > 3 || config.Tolerance < 0 || config.Tolerance > 15*time.Minute {
		return nil, invalid("webhook configuration")
	}
	secrets := make([][]byte, len(config.Secrets))
	for i, s := range config.Secrets {
		if len(s) < 16 || len(s) > 1024 {
			return nil, invalid("webhook secret must contain 16–1024 bytes")
		}
		secrets[i] = []byte(s)
	}
	tolerance := config.Tolerance
	if tolerance == 0 {
		tolerance = 5 * time.Minute
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &WebhookVerifier{secrets: secrets, tolerance: tolerance, now: now}, nil
}

// WebhookEvent is an authenticated immutable event. Accessors return copies;
// unverified events cannot be constructed through exported fields.
type WebhookEvent struct {
	id, eventType string
	timestamp     time.Time
	data          json.RawMessage
	raw           []byte
}

// ID is the event ID bound to the raw-body signature.
func (e *WebhookEvent) ID() string { return e.id }

// Type is read from the signed JSON body, never an unsigned header fallback.
func (e *WebhookEvent) Type() string { return e.eventType }

// Timestamp is the verified signature timestamp.
func (e *WebhookEvent) Timestamp() time.Time { return e.timestamp }

// Data returns an independent copy of the signed data object.
func (e *WebhookEvent) Data() json.RawMessage { return append(json.RawMessage(nil), e.data...) }

// Raw returns an independent copy of the signed body. It can contain customer
// data: do not log it or persist it outside your data-retention policy.
func (e *WebhookEvent) Raw() []byte { return append([]byte(nil), e.raw...) }

// DecodeData decodes the signed data using json.Number for interface values.
func (e *WebhookEvent) DecodeData(target any) error {
	dec := json.NewDecoder(bytes.NewReader(e.data))
	dec.UseNumber()
	return dec.Decode(target)
}

// Verify checks the exact raw bytes, event ID, ISO timestamp, body/header ID
// agreement, type and rotated v2 signatures. Unknown schemes do not authorize
// a request. Read the body with an application-level size limit before calling.
func (v *WebhookVerifier) Verify(raw []byte, headers http.Header) (*WebhookEvent, error) {
	if len(raw) == 0 || len(raw) > 8<<20 {
		return nil, ErrWebhook
	}
	id, ok := singleHeader(headers, "X-Lunixi-Event-Id")
	if !ok || !safeHeader(id, 200) {
		return nil, ErrWebhook
	}
	ts, ok := singleHeader(headers, "X-Lunixi-Signature-Timestamp")
	if !ok || len(ts) > 100 {
		return nil, ErrWebhook
	}
	sig, ok := singleHeader(headers, "X-Lunixi-Signature")
	if !ok || len(sig) > 1024 {
		return nil, ErrWebhook
	}
	timestamp, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return nil, ErrWebhook
	}
	delta := v.now().Sub(timestamp)
	if delta > v.tolerance || delta < -v.tolerance {
		return nil, ErrWebhook
	}
	parts := strings.Split(sig, ",")
	if len(parts) > 8 {
		return nil, ErrWebhook
	}
	message := append([]byte(id+"."+ts+"."), raw...)
	matched := 0
	for _, secret := range v.secrets {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write(message)
		expected := mac.Sum(nil)
		for _, part := range parts {
			scheme, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if !found || strings.TrimSpace(scheme) != "v2" {
				continue
			}
			actual, err := hex.DecodeString(strings.TrimSpace(value))
			if err != nil || len(actual) != sha256.Size {
				continue
			}
			matched |= subtle.ConstantTimeCompare(expected, actual)
		}
	}
	if matched != 1 {
		return nil, ErrWebhook
	}
	// Reject duplicate keys and excessive nesting before normal decoding.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if _, err = readJSON(dec, 0); err != nil {
		return nil, ErrWebhook
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, ErrWebhook
	}
	var envelope struct {
		ID   string          `json:"id"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Type == "" || len(envelope.Type) > 200 || (envelope.ID != "" && envelope.ID != id) {
		return nil, ErrWebhook
	}
	if typ, ok := singleHeader(headers, "X-Lunixi-Event-Type"); !ok || (typ != "" && typ != envelope.Type) {
		return nil, ErrWebhook
	}
	return &WebhookEvent{id: id, eventType: envelope.Type, timestamp: timestamp, data: append(json.RawMessage(nil), envelope.Data...), raw: append([]byte(nil), raw...)}, nil
}

func singleHeader(headers http.Header, name string) (string, bool) {
	var found []string
	for key, values := range headers {
		if strings.EqualFold(key, name) {
			found = append(found, values...)
		}
	}
	if len(found) == 0 {
		return "", true
	}
	if len(found) != 1 {
		return "", false
	}
	return found[0], true
}

// VerifyRequest reads an HTTP request with a fixed limit before verification.
// The caller remains responsible for applying ReplayStore transactionally.
func (v *WebhookVerifier) VerifyRequest(ctx context.Context, request *http.Request) (*WebhookEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request == nil || request.Body == nil || request.Method != "POST" {
		return nil, ErrWebhook
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, (8<<20)+1))
	if err != nil {
		return nil, ErrWebhook
	}
	return v.Verify(raw, request.Header)
}

package lunixi

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, mutate func(*Config)) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	config := Config{BaseURL: server.URL, KeyID: "mk_test_sk_example", PrivateKey: fixture(t).PrivateSeed, HTTPClient: server.Client()}
	if mutate != nil {
		mutate(&config)
	}
	client, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	return client, server
}

func validIntent() IntentRequest {
	return IntentRequest{Amount: 10050, Currency: "TRY", OrderID: "order-123", CallbackURL: "https://merchant.example/payments/return", BasketItems: []BasketItem{{ID: "sku-1", Name: "Example", ItemType: "PHYSICAL", Price: 10050}}}
}

func TestConfigurationLoggingDoesNotExposeCredentials(t *testing.T) {
	c := Config{KeyID: "merchant-key-private-value", PrivateKey: "private-signing-material"}
	j, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, logged := range []string{fmt.Sprint(c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), string(j)} {
		if strings.Contains(logged, c.KeyID) || strings.Contains(logged, c.PrivateKey) {
			t.Fatal("configuration logging disclosed a credential")
		}
	}
}

func TestSignedTransportAndTokenReuse(t *testing.T) {
	f := fixture(t)
	seed, _ := base64.StdEncoding.DecodeString(f.PrivateSeed)
	public := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	var tokenCalls, apiCalls atomic.Int32
	var nonces sync.Map
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		date, nonce := r.Header.Get("X-Date"), r.Header.Get("X-Nonce")
		if _, loaded := nonces.LoadOrStore(nonce, true); loaded {
			t.Error("nonce reused")
		}
		canonical := strings.ToUpper(r.Method) + "\n" + r.URL.RequestURI() + "\nX-Date:" + date + "\nX-Nonce:" + nonce
		if len(raw) > 0 {
			sum := sha256.Sum256(raw)
			expected := "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
			if r.Header.Get("Digest") != expected {
				t.Error("incorrect body digest")
			}
			canonical += "\nDigest:" + expected
		}
		sig, e := base64.StdEncoding.DecodeString(r.Header.Get("X-Signature"))
		if e != nil || !ed25519.Verify(public, []byte(canonical), sig) {
			t.Error("server rejected signature")
		}
		if r.Header.Get("X-Key-Id") != "mk_test_sk_example" {
			t.Error("missing merchant key")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			tokenCalls.Add(1)
			if len(raw) != 0 || r.Header.Get("Digest") != "" || r.Header.Get("Authorization") != "" {
				t.Error("token request must not have body or bearer")
			}
			fmt.Fprint(w, `{"access_token":"at_TEST_example","expires_in":1800}`)
			return
		}
		apiCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer at_TEST_example" {
			t.Error("missing bearer")
		}
		if r.Method == "POST" && r.Header.Get("Idempotency-Key") != "attempt-0000000001" {
			t.Error("idempotency key changed")
		}
		w.Header().Set("X-Request-Id", "req-123")
		fmt.Fprint(w, `{"success":true,"data":{"intentId":"intent-1","token":"ct_example","amount":10050},"pageInfo":{"hasMore":false}}`)
	}, nil)
	response, e := client.Payments.CreateIntent(context.Background(), validIntent(), "attempt-0000000001")
	if e != nil {
		t.Fatal(e)
	}
	if response.RequestID != "req-123" || len(response.PageInfo) == 0 {
		t.Fatal("lost response diagnostics/paging")
	}
	var data map[string]any
	if e = response.DecodeData(&data); e != nil {
		t.Fatal(e)
	}
	if _, ok := data["amount"].(json.Number); !ok {
		t.Fatal("money decoded to float64")
	}
	if _, e = client.Payments.Get(context.Background(), "intent-1"); e != nil {
		t.Fatal(e)
	}
	if tokenCalls.Load() != 1 || apiCalls.Load() != 2 {
		t.Fatal("token cache not used")
	}
}

func TestConcurrentTokenRefreshIsSingleFlight(t *testing.T) {
	var tokens atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			tokens.Add(1)
			time.Sleep(10 * time.Millisecond)
			fmt.Fprint(w, `{"access_token":"at_TEST_cached","expires_in":1800}`)
		} else {
			fmt.Fprint(w, `{"data":{"status":"CAPTURED"}}`)
		}
	}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := client.Payments.Get(context.Background(), "p-1"); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if tokens.Load() != 1 {
		t.Fatalf("expected one authentication, got %d", tokens.Load())
	}
}

func TestNoRedirectLeaksCredentials(t *testing.T) {
	var leaked atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer evil.Close()
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"at_TEST_private","expires_in":1800}`)
			return
		}
		http.Redirect(w, r, evil.URL, http.StatusTemporaryRedirect)
	}, nil)
	_, e := client.Payments.Get(context.Background(), "p-1")
	var api *APIError
	if !errors.As(e, &api) || api.StatusCode != 307 {
		t.Fatalf("expected redirect rejection, got %v", e)
	}
	if leaked.Load() != 0 {
		t.Fatal("credentials followed redirect")
	}
}

func TestConfigurationAndPathGuards(t *testing.T) {
	for _, base := range []string{"http://example.com", "http://localhost.evil.test", "https://user:secret@example.com", "https://api.example/path", "https://example.com?x=1", "https://example.com#x"} {
		_, e := New(Config{BaseURL: base, KeyID: "k", PrivateKey: fixture(t).PrivateSeed, AllowInsecureLocalhost: true})
		if e == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	var calls atomic.Int32
	client, _ := newTestClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) }, nil)
	for _, path := range []string{"https://evil.test/api/v1/payments", "//evil.test/api/v1/payments", "/api/v1/../auth", "/api/v1/%2e%2e/auth", "/api/v1/payments?token=secret", "/api/v1/payments\\x", "/api/v1/auth/token", "/api/v1/auth/token/", "/api/v1/auth/login"} {
		if _, e := client.Do(context.Background(), Request{Method: "GET", Path: path}); !errors.Is(e, ErrConfiguration) {
			t.Fatalf("accepted %s", path)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached network")
	}
}

func TestLocalHTTPRequiresExplicitLoopbackOptIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_local","expires_in":1800}`)
		} else {
			fmt.Fprint(w, `{"data":{}}`)
		}
	}))
	defer server.Close()
	cfg := Config{BaseURL: server.URL, KeyID: "k", PrivateKey: fixture(t).PrivateSeed}
	if _, e := New(cfg); e == nil {
		t.Fatal("HTTP allowed by default")
	}
	cfg.AllowInsecureLocalhost = true
	c, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Payments.Get(context.Background(), "p"); e != nil {
		t.Fatal(e)
	}
}

func TestMutationRetriesReuseBodyAndIdempotency(t *testing.T) {
	var requests atomic.Int32
	var first string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if requests.Add(1) == 1 {
			first = string(raw)
			w.WriteHeader(503)
			fmt.Fprint(w, `{"code":"unavailable"}`)
			return
		}
		if first != string(raw) || r.Header.Get("Idempotency-Key") != "attempt-0000000001" {
			t.Error("retry changed payment identity")
		}
		fmt.Fprint(w, `{"data":{"status":"CAPTURED"}}`)
	}, func(c *Config) { c.MaxRetries = 1 })
	amount := int64(100)
	if _, e := client.Payments.Capture(context.Background(), "p-1", &amount, "attempt-0000000001"); e != nil {
		t.Fatal(e)
	}
	if requests.Load() != 2 {
		t.Fatal("mutation not retried safely")
	}
}

func TestNonIdempotentMutationIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		calls.Add(1)
		w.WriteHeader(503)
		fmt.Fprint(w, `{"code":"unavailable","message":"private secret value"}`)
	}, func(c *Config) { c.MaxRetries = 3 })
	_, e := client.Fraud.Evaluate(context.Background(), map[string]any{"transactionRef": "ref-1"})
	if e == nil || strings.Contains(e.Error(), "private secret") {
		t.Fatal("missing/redaction failure")
	}
	if calls.Load() != 1 {
		t.Fatal("blind retry of unkeyed mutation")
	}
}

func TestAmountAndRemovedFieldValidationBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) }, nil)
	for _, body := range []map[string]any{{"amount": 100, "price": 100}, {"amount": 100, "paidPrice": nil}, {"amount": 100.5}, {"amount": int64(MaxSafeInteger + 1)}} {
		if _, e := client.Do(context.Background(), Request{Method: "POST", Path: "/api/v1/payments/intents", Body: body, IdempotencyKey: "attempt-0000000001"}); e == nil {
			t.Fatalf("accepted %v", body)
		}
	}
	request := validIntent()
	request.BasketItems[0].Price = 1
	if _, e := client.Payments.CreateIntent(context.Background(), request, "attempt-0000000001"); e == nil {
		t.Fatal("accepted unbalanced basket")
	}
	if _, e := client.Payments.Refund(context.Background(), "p-1", nil, "", ""); e == nil {
		t.Fatal("accepted refund without stable key")
	}
	if _, e := client.Fraud.Evaluate(context.Background(), map[string]any{"tenantId": "other"}); e == nil {
		t.Fatal("accepted tenant spoofing field")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid financial input reached network")
	}
}

func TestBoundedResponsesAndCancellation(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		fmt.Fprintf(w, `{"data":"%s"}`, strings.Repeat("s", 300))
	}, func(c *Config) { c.MaxResponseBytes = 128 })
	_, e := client.Payments.Get(context.Background(), "p-1")
	var api *APIError
	if !errors.As(e, &api) || api.Code != "RESPONSE_TOO_LARGE" {
		t.Fatalf("bad limit error: %v", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = client.Payments.Get(ctx, "p-1"); !errors.Is(e, context.Canceled) {
		t.Fatalf("ignored cancellation: %v", e)
	}
}

func TestSafeAuthenticationRefresh(t *testing.T) {
	var auth, requests atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			n := auth.Add(1)
			fmt.Fprintf(w, `{"access_token":"at_TEST_%d","expires_in":1800}`, n)
			return
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"code":"expired"}`)
			return
		}
		fmt.Fprint(w, `{"data":{}}`)
	}, nil)
	if _, e := client.Payments.Get(context.Background(), "p-1"); e != nil {
		t.Fatal(e)
	}
	if auth.Load() != 2 || requests.Load() != 2 {
		t.Fatal("token refresh failed")
	}
}

func TestSplitPendingRemainsExplicit(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		w.WriteHeader(400)
		fmt.Fprint(w, `{"success":false,"code":"AWAITING_ACTION","data":{"splitPaymentId":"split-1","status":"AWAITING_ACTION"}}`)
	}, nil)
	body := map[string]any{"totalAmount": 100, "currency": "TRY", "ownerRef": "customer-1", "legs": []any{map[string]any{"amount": 100, "cardToken": "saved-test-token"}}}
	r, e := client.Payments.Split(context.Background(), body, "attempt-0000000001")
	if e != nil {
		t.Fatal(e)
	}
	if r.StatusCode != 400 || !strings.Contains(string(r.Data), "AWAITING_ACTION") {
		t.Fatal("pending split lost")
	}
	body["totalAmount"] = 101
	if _, e = client.Payments.Split(context.Background(), body, "attempt-0000000001"); e == nil {
		t.Fatal("accepted unbalanced split")
	}
}

func TestQueryIncludedInSignature(t *testing.T) {
	f := fixture(t)
	seed, _ := base64.StdEncoding.DecodeString(f.PrivateSeed)
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		canonical := "GET\n" + r.URL.RequestURI() + "\nX-Date:" + r.Header.Get("X-Date") + "\nX-Nonce:" + r.Header.Get("X-Nonce")
		sig, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Signature"))
		if !ed25519.Verify(pub, []byte(canonical), sig) {
			t.Error("query not included in signature")
		}
		if r.URL.Query().Get("customerId") != "Şükrü / 1" {
			t.Error("query value changed")
		}
		fmt.Fprint(w, `{"data":[]}`)
	}, nil)
	if _, e := client.Payments.List(context.Background(), url.Values{"customerId": {"Şükrü / 1"}, "limit": {"20"}}); e != nil {
		t.Fatal(e)
	}
}

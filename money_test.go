package lunixi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestCanonicalMinorStrings(t *testing.T) {
	const large = "9007199254740993"
	for _, input := range []struct{ decimal, currency string }{{large, "JPY"}, {"90071992547409.93", "TRY"}, {"9007199254740.993", "KWD"}} {
		actual, err := DecimalToMinor(input.decimal, input.currency)
		if err != nil || actual != large {
			t.Fatalf("exact conversion: %s %v", actual, err)
		}
	}
	if !validMinor("-9007199254740993", false, true) {
		t.Fatal("signed balance rejected")
	}
	for _, raw := range []string{`{"amount":1}`, `{"amount":"0"}`, `{"amount":"01"}`, `{"amount":"-0"}`, `{"amount":"1.0"}`, `{"amount":"1e3"}`, `{"amount":"9223372036854775808"}`} {
		if validateMoneyJSON([]byte(raw), false) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	raw, err := json.Marshal(IntentRequest{Amount: large, Currency: "TRY", OrderID: "order", CallbackURL: "https://merchant.invalid/return", BasketItems: []BasketItem{{ID: "1", Name: "line", ItemType: "PHYSICAL", Price: large}}})
	if err != nil || validateMoneyJSON(raw, false) != nil {
		t.Fatal("wire string rejected")
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil || body["amount"] != large {
		t.Fatal("wire amount changed")
	}
	if validateMoneyJSON([]byte(`{"data":{"amount":1}}`), true) == nil {
		t.Fatal("numeric response accepted")
	}
	if validateMoneyJSON([]byte(`{"metadata":{"amount":1},"providerPayload":{"amount":1},"rate":1.5}`), false) != nil {
		t.Fatal("opaque payload changed")
	}
}

func TestOptionalMoneyRejectsExplicitZero(t *testing.T) {
	p := &PaymentClient{}
	zero := "0"
	if _, err := p.Capture(context.Background(), "p", &zero, "stable-operation-key"); err == nil {
		t.Fatal("zero capture accepted")
	}
	if _, err := p.Refund(context.Background(), "p", &zero, "", "stable-operation-key"); err == nil {
		t.Fatal("zero refund accepted")
	}
}

func TestOptionalMoneyOmissionAndRawNullAreDistinct(t *testing.T) {
	amounts := make(chan map[string]any, 2)
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_, _ = io.WriteString(w, `{"access_token":"at_TEST_fixture","expires_in":1800}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		amounts <- body
		_, _ = io.WriteString(w, `{"data":{}}`)
	}, nil)
	for _, operation := range []string{"capture", "refund"} {
		_, err := client.Do(context.Background(), Request{Method: "POST", Path: "/api/v1/payments/p/" + operation, Body: map[string]any{"amount": nil}, IdempotencyKey: "stable-operation-key"})
		if err == nil {
			t.Fatal("explicit JSON null accepted")
		}
	}
	if len(amounts) != 0 {
		t.Fatal("null request reached transport")
	}
	if _, err := client.Payments.Capture(context.Background(), "p", nil, "stable-capture-key"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Payments.Refund(context.Background(), "p", nil, "", "stable-refund-key"); err != nil {
		t.Fatal(err)
	}
	if len(amounts) != 2 {
		t.Fatal("omitted operations were not sent")
	}
	for range 2 {
		body := <-amounts
		if _, present := body["amount"]; present {
			t.Fatal("omission became JSON amount")
		}
	}
}

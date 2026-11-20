package lunixi

import (
	"context"
	"strings"
	"testing"
)

const testCardNumber = "5528790000000008"

func storedCardRequest() StoreCardRequest {
	buyer, billing := testBuyer(), testAddress()
	return StoreCardRequest{
		Card:           map[string]any{"cardHolderName": "Test Buyer", "cardNumber": testCardNumber, "expireMonth": "12", "expireYear": "2030", "cvcNumber": "123"},
		CardUserKey:    "buyer-42",
		CallbackURL:    "https://merchant.example/cards/return",
		Buyer:          &buyer,
		BillingAddress: &billing,
	}
}

func TestStoreCardRequiresKeyAndSendsBody(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	request := storedCardRequest()
	request.Currency = "try"
	if _, e := client.Payments.StoreCard(ctx, request, testKey); e != nil {
		t.Fatal(e)
	}
	_, e := client.Payments.StoreCard(ctx, storedCardRequest(), "")
	mustReject(t, "missing key", e)
	for name, mutate := range map[string]func(*StoreCardRequest){
		"no card":        func(r *StoreCardRequest) { r.Card = nil },
		"no user key":    func(r *StoreCardRequest) { r.CardUserKey = " " },
		"http callback":  func(r *StoreCardRequest) { r.CallbackURL = "http://merchant.example/return" },
		"bad currency":   func(r *StoreCardRequest) { r.Currency = "LIRA" },
		"empty callback": func(r *StoreCardRequest) { r.CallbackURL = "" },
		"no buyer":       func(r *StoreCardRequest) { r.Buyer = nil },
		"no billing":     func(r *StoreCardRequest) { r.BillingAddress = nil },
	} {
		bad := storedCardRequest()
		mutate(&bad)
		_, e := client.Payments.StoreCard(ctx, bad, testKey)
		mustReject(t, name, e)
		if strings.Contains(e.Error(), testCardNumber) || strings.Contains(e.Error(), "123") {
			t.Fatalf("%s: error message carried card data", name)
		}
	}
	sent := requests()
	if len(sent) != 1 {
		t.Fatalf("invalid requests reached the network: %d", len(sent))
	}
	r := sent[0]
	if r.Method != "POST" || r.URI != "/api/v1/payments/cards" || !r.Signed || r.IdempotencyKey != testKey {
		t.Fatalf("store card request %+v", r)
	}
	body := decodeBody(t, r.Body)
	card, _ := body["card"].(map[string]any)
	if card["cardNumber"] != testCardNumber || body["cardUserKey"] != "buyer-42" || body["callbackUrl"] != "https://merchant.example/cards/return" || body["currency"] != "TRY" {
		t.Fatalf("store card body %s", r.Body)
	}
}

func TestStoredCardReadsAndDeactivation(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.ListStoredCards(ctx, "buyer 42"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.GetStoredCard(ctx, "card_tok_1"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.DeactivateStoredCard(ctx, "card_tok_1", testKey); e != nil {
		t.Fatal(e)
	}
	_, e := client.Payments.DeactivateStoredCard(ctx, "card_tok_1", "")
	mustReject(t, "deactivate without key", e)
	_, e = client.Payments.GetStoredCard(ctx, "../card")
	mustReject(t, "token path", e)
	_, e = client.Payments.ListStoredCards(ctx, "")
	mustReject(t, "empty card user key", e)
	sent := requests()
	if len(sent) != 3 {
		t.Fatalf("invalid requests reached the network: %d", len(sent))
	}
	want := []recordedRequest{
		{Method: "GET", URI: "/api/v1/payments/cards?cardUserKey=buyer+42"},
		{Method: "GET", URI: "/api/v1/payments/cards/card_tok_1"},
		{Method: "DELETE", URI: "/api/v1/payments/cards/card_tok_1", IdempotencyKey: testKey},
	}
	for i, r := range sent {
		if r.Method != want[i].Method || r.URI != want[i].URI || r.IdempotencyKey != want[i].IdempotencyKey || !r.Signed || len(r.Body) != 0 {
			t.Fatalf("request %d: %+v", i, r)
		}
	}
}

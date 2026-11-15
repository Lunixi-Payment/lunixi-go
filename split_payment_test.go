package lunixi

import (
	"context"
	"testing"
)

func splitBody(legs ...map[string]any) map[string]any {
	list := make([]any, len(legs))
	for i, leg := range legs {
		list[i] = leg
	}
	return map[string]any{"totalAmount": "1000", "currency": "TRY", "ownerRef": "customer-1", "legs": list}
}

func TestSplitLegBlockRule(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	card := map[string]any{"amount": "500", "cardToken": "saved-test-token"}
	rejected := map[string]map[string]any{
		"bank transfer leg":      {"amount": "500", "paymentMethod": "BANK_TRANSFER", "bankTransfer": map[string]any{"bankCode": "0062"}},
		"bank transfer method":   {"amount": "500", "paymentMethod": "BANK_TRANSFER"},
		"open banking leg":       {"amount": "500", "paymentMethod": "OPEN_BANKING", "openBanking": map[string]any{"providerCode": "ob_provider"}},
		"open banking method":    {"amount": "500", "paymentMethod": "OPEN_BANKING"},
		"meal card leg":          {"amount": "500", "paymentMethod": "MEAL_CARD", "mealCard": map[string]any{"providerCode": "meal_provider"}},
		"shopping loan leg":      {"amount": "500", "paymentMethod": "SHOPPING_LOAN", "shoppingLoan": map[string]any{"providerCode": "loan_provider"}},
		"card with block":        {"amount": "500", "cardToken": "other-token", "mobileBilling": map[string]any{"providerCode": "mobile_provider"}},
		"card with bankTransfer": {"amount": "500", "cardToken": "other-token", "bankTransfer": map[string]any{"bankCode": "0062"}},
		"missing block":          {"amount": "500", "paymentMethod": "MOBILE_BILLING"},
		"other block":            {"amount": "500", "paymentMethod": "MOBILE_BILLING", "mobileBilling": map[string]any{"providerCode": "mobile_provider"}, "wallet": map[string]any{"providerCode": "wallet_provider"}},
		"missing providerCode":   {"amount": "500", "paymentMethod": "WALLET", "wallet": map[string]any{}},
		"card field on method":   {"amount": "500", "paymentMethod": "WALLET", "cardToken": "x", "wallet": map[string]any{"providerCode": "wallet_provider"}},
		"installment on method":  {"amount": "500", "paymentMethod": "WALLET", "installment": 3, "wallet": map[string]any{"providerCode": "wallet_provider"}},
		"note schedule":          {"amount": "500", "paymentMethod": "PROMISSORY_NOTE", "promissoryNote": map[string]any{"providerCode": "note_provider", "schedule": []any{map[string]any{"amount": "500", "dueDate": "2027-01-15"}}}},
		"note without count":     {"amount": "500", "paymentMethod": "PROMISSORY_NOTE", "promissoryNote": map[string]any{"providerCode": "note_provider"}},
		"unknown leg field":      {"amount": "500", "cardToken": "other-token", "surprise": true},
		"lowercase method":       {"amount": "500", "paymentMethod": "wallet", "wallet": map[string]any{"providerCode": "wallet_provider"}},
	}
	for name, leg := range rejected {
		_, e := client.Payments.Split(ctx, splitBody(card, leg), testKey)
		mustReject(t, name, e)
	}
	if len(requests()) != 0 {
		t.Fatal("invalid split leg reached network")
	}
	accepted := [][]map[string]any{
		{card, {"amount": "500", "paymentMethod": "MOBILE_BILLING", "mobileBilling": map[string]any{"providerCode": "mobile_provider", "gsmNumber": "+905321234567"}}},
		{card, {"amount": "500", "paymentMethod": "PROMISSORY_NOTE", "installment": 1, "promissoryNote": map[string]any{"providerCode": "note_provider", "noteCount": "3"}}},
		{card, {"amount": "500", "paymentMethod": "CARD", "cardToken": "second-token", "installment": 3, "installmentQuoteToken": "iq_1"}},
	}
	for i, legs := range accepted {
		if _, e := client.Payments.Split(ctx, splitBody(legs...), testKey); e != nil {
			t.Fatalf("accepted case %d: %v", i, e)
		}
	}
	for _, r := range requests() {
		if r.URI != "/api/v1/payments/split" || r.IdempotencyKey != testKey || !r.Signed {
			t.Fatalf("split request %+v", r)
		}
	}
}

func TestListSplits(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.ListSplits(ctx, SplitListFilters{
		Status:      []string{SplitPaymentStatusCompleted, SplitPaymentStatusVoided},
		Currency:    []string{"TRY"},
		CreatedFrom: "2026-11-01",
		CreatedTo:   "2026-11-15T23:59:59Z",
		OrderID:     "ORDER-1",
		OwnerRef:    "customer-1",
		Limit:       intPointer(25),
		After:       "cursor_1",
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ListSplits(ctx, SplitListFilters{}); e != nil {
		t.Fatal(e)
	}
	for name, filters := range map[string]SplitListFilters{
		"unknown status": {Status: []string{"DONE"}},
		"empty status":   {Status: []string{}},
		"bad currency":   {Currency: []string{"try"}},
		"bad date":       {CreatedFrom: "15.11.2026"},
		"limit 0":        {Limit: intPointer(0)},
		"limit 101":      {Limit: intPointer(101)},
		"blank orderId":  {OrderID: "   "},
	} {
		_, e := client.Payments.ListSplits(ctx, filters)
		mustReject(t, name, e)
	}
	sent := requests()
	want := []string{
		"/api/v1/payments/split?after=cursor_1&createdFrom=2026-11-01&createdTo=2026-11-15T23%3A59%3A59Z&currency=TRY&limit=25&orderId=ORDER-1&ownerRef=customer-1&status=COMPLETED&status=VOIDED",
		"/api/v1/payments/split",
	}
	if len(sent) != len(want) {
		t.Fatalf("requests %+v", sent)
	}
	for i, r := range sent {
		if r.URI != want[i] || r.Method != "GET" || !r.Signed {
			t.Fatalf("request %d: %+v", i, r)
		}
	}
}

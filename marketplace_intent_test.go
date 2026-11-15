package lunixi

import (
	"context"
	"testing"
)

func marketplaceIntent() MarketplaceIntentRequest {
	return MarketplaceIntentRequest{
		IntentRequest: validIntent(),
		Sellers: []MarketplaceSeller{
			{SubDealerID: "seller-a", AmountMinor: "6050"},
			{SubDealerID: "seller-b", AmountMinor: "4000", Scenario: map[string]any{"type": "DYNAMIC_RATE", "ratePercent": "5"}},
		},
	}
}

func TestMarketplaceIntentSellersMustAddUpToAmount(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.CreateMarketplaceIntent(ctx, marketplaceIntent(), testKey); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.CreateMarketplaceIntent(ctx, marketplaceIntent(), ""); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*MarketplaceIntentRequest){
		"total mismatch":   func(r *MarketplaceIntentRequest) { r.Sellers[1].AmountMinor = "3999" },
		"decimal share":    func(r *MarketplaceIntentRequest) { r.Sellers[1].AmountMinor = "40.00" },
		"zero share":       func(r *MarketplaceIntentRequest) { r.Sellers[0].AmountMinor = "0"; r.Sellers[1].AmountMinor = "10050" },
		"unknown policy":   func(r *MarketplaceIntentRequest) { r.MultiSellerPolicy = "SOME" },
		"no sellers":       func(r *MarketplaceIntentRequest) { r.Sellers = nil },
		"no sub dealer":    func(r *MarketplaceIntentRequest) { r.Sellers[0].SubDealerID = "" },
		"intent amount":    func(r *MarketplaceIntentRequest) { r.Amount = "0" },
		"installment rule": func(r *MarketplaceIntentRequest) { r.EnabledInstallments = []int{2} },
	} {
		bad := marketplaceIntent()
		mutate(&bad)
		_, e := client.Payments.CreateMarketplaceIntent(ctx, bad, testKey)
		mustReject(t, name, e)
	}
	_, e := client.Payments.CreateMarketplaceIntent(ctx, marketplaceIntent(), "short")
	mustReject(t, "short key", e)
	sent := requests()
	if len(sent) != 2 {
		t.Fatalf("invalid requests reached the network: %d", len(sent))
	}
	if sent[0].URI != "/api/v1/payments/marketplace/intents" || !sent[0].Signed || sent[0].IdempotencyKey != testKey || sent[1].IdempotencyKey != "" {
		t.Fatalf("marketplace requests %+v", sent)
	}
	body := decodeBody(t, sent[0].Body)
	sellers, _ := body["sellers"].([]any)
	first, _ := sellers[0].(map[string]any)
	if body["amount"] != "10050" || body["orderId"] != "order-123" || len(sellers) != 2 || first["amountMinor"] != "6050" || first["subDealerId"] != "seller-a" {
		t.Fatalf("marketplace body %s", sent[0].Body)
	}
	if _, has := body["price"]; has {
		t.Fatal("removed amount field sent")
	}
}

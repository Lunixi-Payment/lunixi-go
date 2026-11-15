package lunixi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Names and card data in the fixtures are fictional test values.

const testMarketplaceKey = "marketplace-key-0001"

func marketplaceLegs() []MarketplaceSeller {
	return []MarketplaceSeller{
		{SubDealerID: "d_alice", AmountMinor: "6050"},
		{SubDealerID: "d_bob", AmountMinor: "4000", Scenario: map[string]any{"type": "DYNAMIC_RATE", "ratePercent": "5"}},
	}
}

func marketplaceChargeRequest() map[string]any {
	return map[string]any{
		"amount":         "10050",
		"currency":       "TRY",
		"orderId":        "ORD-MP-1",
		"card":           map[string]any{"cardHolderName": "Ada Yilmaz", "cardNumber": "5400000000000004", "expireMonth": "12", "expireYear": "28", "cvcNumber": "123"},
		"buyer":          &Buyer{Name: "Ada", Surname: "Yilmaz"},
		"billingAddress": &Address{City: "Istanbul"},
		"basketItems":    []BasketItem{{ID: "SKU-1", Name: "Demo", ItemType: "PHYSICAL", Price: "10050"}},
	}
}

func TestMarketplaceOnlyTheChargesAreSigned(t *testing.T) {
	for name, route := range MarketplaceRoutes() {
		charge := strings.HasPrefix(route.Path, "/api/v1/payments/marketplace/")
		if route.StepUp != charge {
			t.Errorf("%s stepUp %v", name, route.StepUp)
		}
		if !charge && !strings.HasPrefix(route.Path, "/api/v1/marketplace/") {
			t.Errorf("%s path %s", name, route.Path)
		}
	}
}

func TestMarketplaceCheckoutIntentIsSignedWithTheLegs(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"paymentId":"pi_1","token":"tok_1","checkoutFormContent":"<script></script>"}`)
	})
	r, err := client.Marketplace.CreateCheckoutIntent(context.Background(), validIntent(), marketplaceLegs(), "", testKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Envelope), "tok_1") {
		t.Fatalf("answer %s", r.Envelope)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/payments/marketplace/intents" || !sent[0].Signed || sent[0].Key != testKey {
		t.Fatalf("request %+v", sent[0])
	}
	body := decodeBody(t, sent[0].Body)
	sellers, _ := body["sellers"].([]any)
	second, _ := sellers[1].(map[string]any)
	scenario, _ := second["scenario"].(map[string]any)
	if body["amount"] != "10050" || body["multiSellerPolicy"] != MarketplaceMultiSellerPolicyAllOrNothing || len(sellers) != 2 || scenario["type"] != "DYNAMIC_RATE" {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestMarketplaceCheckoutIntentWithoutTokenIsAnError(t *testing.T) {
	client, _ := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":false,"code":"marketplace.payment.seller_not_eligible","message":"seller not eligible"}`)
	})
	_, err := client.Marketplace.CreateCheckoutIntent(context.Background(), validIntent(), []MarketplaceSeller{{SubDealerID: "d_1", AmountMinor: "10050"}}, "", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "marketplace.payment.seller_not_eligible" {
		t.Fatalf("error %v", err)
	}
}

func TestMarketplaceLegsAreCheckedBeforeSending(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*Client) error{
		"no legs": func(c *Client) error {
			_, err := c.Marketplace.CreateCheckoutIntent(ctx, validIntent(), nil, "", "")
			return err
		},
		"zero leg": func(c *Client) error {
			legs := []MarketplaceSeller{{SubDealerID: "d_1", AmountMinor: "0"}, {SubDealerID: "d_2", AmountMinor: "10050"}}
			_, err := c.Marketplace.CreateCheckoutIntent(ctx, validIntent(), legs, "", "")
			return err
		},
		"total mismatch": func(c *Client) error {
			_, err := c.Marketplace.CreateCheckoutIntent(ctx, validIntent(), []MarketplaceSeller{{SubDealerID: "d_1", AmountMinor: "999"}}, "", "")
			return err
		},
		"policy": func(c *Client) error {
			_, err := c.Marketplace.CreateCheckoutIntent(ctx, validIntent(), []MarketplaceSeller{{SubDealerID: "d_1", AmountMinor: "10050"}}, "SOME", "")
			return err
		},
		"charge total mismatch": func(c *Client) error {
			legs := marketplaceLegs()
			legs[1].AmountMinor = "3999"
			_, err := c.Marketplace.ChargeMarketplaceCard2d(ctx, marketplaceChargeRequest(), legs, "", "")
			return err
		},
		"legs in the body": func(c *Client) error {
			request := marketplaceChargeRequest()
			request["sellers"] = []any{}
			_, err := c.Marketplace.ChargeMarketplaceCard2d(ctx, request, marketplaceLegs(), "", "")
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestMarketplaceCardChargesGoToTheirRoute(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"status":"success","data":{"paymentId":"pay_1","status":"SUCCEEDED"}}`)
	})
	ctx := context.Background()
	if _, err := client.Marketplace.ChargeMarketplaceCard2d(ctx, marketplaceChargeRequest(), marketplaceLegs(), MarketplaceMultiSellerPolicyPartialAllowed, testKey); err != nil {
		t.Fatal(err)
	}
	threeDS := marketplaceChargeRequest()
	threeDS["callbackUrl"] = "https://shop.example/3ds"
	if _, err := client.Marketplace.ChargeMarketplaceCard3d(ctx, threeDS, marketplaceLegs(), "", ""); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/payments/marketplace/2d" || sent[1].URI != "/api/v1/payments/marketplace/3d" || !sent[0].Signed || !sent[1].Signed || sent[0].Key != testKey {
		t.Fatalf("requests %+v", sent)
	}
	body := decodeBody(t, sent[0].Body)
	card, _ := body["card"].(map[string]any)
	if body["multiSellerPolicy"] != MarketplaceMultiSellerPolicyPartialAllowed || len(body["sellers"].([]any)) != 2 || card["cardNumber"] != "5400000000000004" {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestMarketplaceChargeErrorsNeverShowTheCard(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*Client) error{
		"no buyer": func(c *Client) error {
			request := marketplaceChargeRequest()
			delete(request, "buyer")
			_, err := c.Marketplace.ChargeMarketplaceCard2d(ctx, request, marketplaceLegs(), "", "")
			return err
		},
		"price": func(c *Client) error {
			request := marketplaceChargeRequest()
			request["price"] = "10050"
			_, err := c.Marketplace.ChargeMarketplaceCard2d(ctx, request, marketplaceLegs(), "", "")
			return err
		},
		"string preauth": func(c *Client) error {
			request := marketplaceChargeRequest()
			request["preauth"] = "false"
			_, err := c.Marketplace.ChargeMarketplaceCard2d(ctx, request, marketplaceLegs(), "", "")
			return err
		},
		"numeric amount": func(c *Client) error {
			request := marketplaceChargeRequest()
			request["amount"] = 10050
			_, err := c.Marketplace.ChargeMarketplaceCard(ctx, request, false, "")
			return err
		},
		"empty basket": func(c *Client) error {
			request := marketplaceChargeRequest()
			request["basketItems"] = []BasketItem{}
			_, err := c.Marketplace.ChargeMarketplaceCard3d(ctx, request, marketplaceLegs(), "", "")
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		err := call(client)
		expectLocalError(t, name, err, requests())
		if strings.Contains(err.Error(), "5400000000000004") || strings.Contains(err.Error(), "123") {
			t.Fatalf("%s: error shows card data: %v", name, err)
		}
	}
}

func TestMarketplaceDealerRoutesAreBearerOnly(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"id":"d_1","legalName":"Acme","status":"DRAFT"}}`)
	})
	ctx := context.Background()
	dealer, err := client.Marketplace.CreateDealer(ctx, map[string]any{"legalName": "Acme", "taxId": "123", "externalRef": "42"})
	if err != nil || !strings.Contains(string(dealer), "d_1") {
		t.Fatalf("dealer %s %v", dealer, err)
	}
	steps := []func() error{
		func() error {
			_, err := client.Marketplace.UpdateDealerStatus(ctx, "d_1", MarketplaceDealerStatusSuspended)
			return err
		},
		func() error { _, err := client.Marketplace.ActivateDealer(ctx, "d_1"); return err },
		func() error {
			_, err := client.Marketplace.SetDealerTerms(ctx, "d_1", map[string]any{"commissionBps": 250})
			return err
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	sent := requests()
	want := []struct{ method, uri, body string }{
		{"POST", "/api/v1/marketplace/dealers", `{"legalName":"Acme","taxId":"123","externalRef":"42"}`},
		{"PUT", "/api/v1/marketplace/dealers/d_1/status", `{"status":"SUSPENDED"}`},
		{"POST", "/api/v1/marketplace/dealers/d_1/activate", ``},
		{"POST", "/api/v1/marketplace/dealers/d_1/terms", `{"commissionBps":250}`},
	}
	for i, expected := range want {
		if sent[i].Method != expected.method || sent[i].URI != expected.uri || string(sent[i].Body) != expected.body || sent[i].Signed || !sent[i].Bearer {
			t.Fatalf("request %d: %+v (body %s)", i, sent[i], sent[i].Body)
		}
	}
	for name, call := range map[string]func(*Client) error{
		"status": func(c *Client) error { _, err := c.Marketplace.UpdateDealerStatus(ctx, "d_1", "PAUSED"); return err },
		"dealer type": func(c *Client) error {
			_, err := c.Marketplace.CreateDealer(ctx, map[string]any{"legalName": "Acme", "dealerType": "COMPANY"})
			return err
		},
		"legal name": func(c *Client) error {
			_, err := c.Marketplace.CreateDealer(ctx, map[string]any{"taxId": "123"})
			return err
		},
		"bulk empty": func(c *Client) error { _, err := c.Marketplace.BulkCreateDealers(ctx, nil); return err },
	} {
		fresh, recorded := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(fresh), recorded())
	}
}

func TestMarketplaceListsAndOnboarding(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			productJSON(w, 200, `{"status":"success","data":{"items":[{"id":"d_1","status":"ACTIVE"},{"id":"d_2","status":"DRAFT"}],"total":2,"pageInfo":{"hasMore":false,"nextCursor":null,"totalCount":2}}}`)
			return
		}
		productJSON(w, 201, `{"success":true,"token":"mp_xyz"}`)
	})
	ctx := context.Background()
	page, err := client.Marketplace.ListDealers(ctx, map[string]any{"status": MarketplaceDealerStatusActive})
	if err != nil || len(page.Items) != 2 || page.TotalCount == nil || *page.TotalCount != 2 || page.HasMore {
		t.Fatalf("page %+v %v", page, err)
	}
	link, err := client.Marketplace.OnboardingToken(ctx, "d_1")
	if err != nil {
		t.Fatal(err)
	}
	if got := MarketplaceOnboardingURL(link, ""); got != "https://altbayi.lunixi.com/m/mp_xyz" {
		t.Fatalf("onboarding url %s", got)
	}
	if got := MarketplaceOnboardingURL(json.RawMessage(`{"token":"mp_xyz","url":"https://onboard.example/m/mp_xyz"}`), ""); got != "https://onboard.example/m/mp_xyz" {
		t.Fatalf("onboarding url %s", got)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/marketplace/dealers?status=ACTIVE" || sent[1].URI != "/api/v1/marketplace/dealers/d_1/onboarding-token" {
		t.Fatalf("requests %+v", sent)
	}
}

func TestMarketplacePayoutsCommissionsAndAudit(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"items":[]}}`)
	})
	ctx := context.Background()
	steps := []func() error{
		func() error {
			_, err := client.Marketplace.CreatePayout(ctx, map[string]any{"subDealerId": "d_1", "amountMinor": "5000", "currency": "TRY"})
			return err
		},
		func() error {
			_, err := client.Marketplace.ResolveCommission(ctx, "d_1", "ELECTRONICS", "100000")
			return err
		},
		func() error {
			_, err := client.Marketplace.BulkMarkPayoutsPaid(ctx, []string{"po_1", "po_2"}, map[string]any{"reference": "R-1"})
			return err
		},
		func() error { _, err := client.Marketplace.AuditLog(ctx, map[string]any{"limit": 20}); return err },
		func() error { _, err := client.Marketplace.VerifyAuditLog(ctx); return err },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	sent := requests()
	want := []struct{ uri, body string }{
		{"/api/v1/marketplace/payouts", `{"amountMinor":"5000","currency":"TRY","subDealerId":"d_1"}`},
		{"/api/v1/marketplace/commission/resolve?categoryCode=ELECTRONICS&mtdVolumeMinor=100000&subDealerId=d_1", ``},
		{"/api/v1/marketplace/payouts/bulk-mark-paid", `{"payoutIds":["po_1","po_2"],"reference":"R-1"}`},
		{"/api/v1/marketplace/audit-log?limit=20", ``},
		{"/api/v1/marketplace/audit-log/verify", ``},
	}
	for i, expected := range want {
		if sent[i].URI != expected.uri || string(sent[i].Body) != expected.body {
			t.Fatalf("request %d: %+v (body %s)", i, sent[i], sent[i].Body)
		}
	}
	for name, call := range map[string]func(*Client) error{
		"numeric payout": func(c *Client) error {
			_, err := c.Marketplace.CreatePayout(ctx, map[string]any{"subDealerId": "d_1", "amountMinor": 5000})
			return err
		},
		"volume": func(c *Client) error { _, err := c.Marketplace.ResolveCommission(ctx, "d_1", "", "100.00"); return err },
		"filter": func(c *Client) error {
			_, err := c.Marketplace.CommissionRules(ctx, map[string]any{"includeArchived": true})
			return err
		},
	} {
		fresh, recorded := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(fresh), recorded())
	}
}

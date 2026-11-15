package lunixi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSubscriptionRoutesAreBearerOnly(t *testing.T) {
	for name, route := range SubscriptionRoutes() {
		if route.StepUp {
			t.Errorf("%s is signed", name)
		}
		if !strings.HasPrefix(route.Path, "/api/v1/subscriptions/") && !strings.HasPrefix(route.Path, "/api/panel/v1/subscriptions/portal/") {
			t.Errorf("%s path %s", name, route.Path)
		}
	}
	if SubscriptionRoutes()["create"].KeyRule() != "required" {
		t.Fatal("create must require a key")
	}
	if len(SubscriptionEventTypes()) != 23 || !IsSubscriptionActive(SubscriptionStatusPastDue) || IsSubscriptionActive(SubscriptionStatusPaused) || !IsSubscriptionTerminal(SubscriptionStatusCanceled) {
		t.Fatal("subscription values")
	}
}

func TestSubscriptionCreateSendsTheKeyInHeaderAndBody(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"id":"sub_1","status":"ACTIVE","priceSnapshotAmount":"9900"}}`)
	})
	data, err := client.Subscriptions.Create(context.Background(), map[string]any{
		"customerId": "cus_1", "planId": "plan_1", "paymentCardId": "card_1",
		"collectionMethod": SubscriptionCollectionMethodAutoCharge, "metadata": nil,
	}, "idem-sub-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"sub_1"`) {
		t.Fatalf("data %s", data)
	}
	sent := requests()
	if len(sent) != 1 || sent[0].Method != "POST" || sent[0].URI != "/api/v1/subscriptions/subscriptions" || sent[0].Key != "idem-sub-1" {
		t.Fatalf("request %+v", sent)
	}
	if sent[0].Signed || !sent[0].Bearer {
		t.Fatal("subscription calls are bearer-only")
	}
	if want := `{"customerId":"cus_1","planId":"plan_1","idempotencyKey":"idem-sub-1","paymentCardId":"card_1","collectionMethod":"AUTO_CHARGE"}`; string(sent[0].Body) != want {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestSubscriptionsRefuseBeforeSending(t *testing.T) {
	plan := func() map[string]any {
		return map[string]any{"productId": "prod_1", "code": "free", "name": "Free", "price": "0", "currency": "try", "billing": map[string]any{"frequency": "monthly"}}
	}
	ctx := context.Background()
	cases := map[string]func(*Client) error{
		"missing key": func(c *Client) error {
			_, err := c.Subscriptions.Create(ctx, map[string]any{"customerId": "cus_1", "planId": "plan_1"}, "")
			return err
		},
		"unknown field": func(c *Client) error {
			_, err := c.Subscriptions.Create(ctx, map[string]any{"customerId": "cus_1", "planId": "plan_1", "amount": "1"}, "k1")
			return err
		},
		"collection method": func(c *Client) error {
			_, err := c.Subscriptions.Create(ctx, map[string]any{"customerId": "cus_1", "planId": "plan_1", "collectionMethod": "CASH"}, "k1")
			return err
		},
		"keys differ": func(c *Client) error {
			_, err := c.Subscriptions.Create(ctx, map[string]any{"customerId": "cus_1", "planId": "plan_1", "idempotencyKey": "a"}, "b")
			return err
		},
		"key with space": func(c *Client) error {
			_, err := c.Subscriptions.Invoices.RetryCharge(ctx, "inv_1", "key with space")
			return err
		},
		"numeric price": func(c *Client) error {
			request := plan()
			request["price"] = 9900
			_, err := c.Subscriptions.Plans.Create(ctx, request, "")
			return err
		},
		"currency": func(c *Client) error {
			request := plan()
			request["currency"] = "XXX"
			_, err := c.Subscriptions.Plans.Create(ctx, request, "")
			return err
		},
		"frequency": func(c *Client) error {
			request := plan()
			request["billing"] = map[string]any{"frequency": "HOURLY"}
			_, err := c.Subscriptions.Plans.Create(ctx, request, "")
			return err
		},
		"interval": func(c *Client) error {
			request := plan()
			request["billing"] = map[string]any{"frequency": "DAILY", "interval": 0}
			_, err := c.Subscriptions.Plans.Create(ctx, request, "")
			return err
		},
		"unknown filter": func(c *Client) error {
			_, err := c.Subscriptions.Plans.List(ctx, map[string]any{"archived": true})
			return err
		},
		"credit amount": func(c *Client) error {
			_, err := c.Subscriptions.Customers.AdjustCredit(ctx, "cus_1", map[string]any{"amountMinor": 5})
			return err
		},
		"portal sdk mode": func(c *Client) error {
			_, err := c.Subscriptions.Portal.CreateSdkSession(ctx, "cus_1", "https://shop.example", SubscriptionPortalSessionOptions{SdkMode: "EMBED"})
			return err
		},
		"bad path id": func(c *Client) error {
			_, err := c.Subscriptions.Get(ctx, "../plans")
			return err
		},
		"nil update": func(c *Client) error {
			_, err := c.Subscriptions.Plans.Update(ctx, "plan_1", nil)
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestSubscriptionCardNumberIsRefusedWithoutEchoingIt(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	_, err := client.Subscriptions.Cards.Add(context.Background(), map[string]any{"customerId": "cus_1", "paymentMethodToken": "tok_abc", "cardNumber": "4111111111111111"}, "")
	expectLocalError(t, "card number", err, requests())
	if !strings.Contains(err.Error(), "cardNumber") || strings.Contains(err.Error(), "4111") {
		t.Fatalf("error %v", err)
	}
}

func TestSubscriptionOperations(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"id":"sub_1","status":"ACTIVE"}}`)
	})
	ctx := context.Background()
	steps := []func() error{
		func() error { _, err := client.Subscriptions.Cancel(ctx, "sub_1", true, "too expensive"); return err },
		func() error { _, err := client.Subscriptions.Pause(ctx, "sub_1", ""); return err },
		func() error { _, err := client.Subscriptions.Pause(ctx, "sub_1", "2026-12-01T00:00:00Z"); return err },
		func() error { _, err := client.Subscriptions.Resume(ctx, "sub_1"); return err },
		func() error {
			_, err := client.Subscriptions.ChangePlan(ctx, "sub_1", map[string]any{"newPlanId": "plan_2", "behavior": "next_period"}, "idem-cp-1")
			return err
		},
		func() error { _, err := client.Subscriptions.Cards.Deactivate(ctx, "card_1", ""); return err },
		func() error {
			_, err := client.Subscriptions.Invoices.RetryCharge(ctx, "inv_1", "idem-retry-1")
			return err
		},
		func() error { _, err := client.Subscriptions.Portal.RevokeLink(ctx, "lnk_1", ""); return err },
		func() error {
			_, err := client.Subscriptions.Portal.RevokeLink(ctx, "lnk_1", "customer left")
			return err
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	sent := requests()
	want := []struct{ method, uri, body, key string }{
		{"POST", "/api/v1/subscriptions/subscriptions/sub_1/cancel", `{"cancelAtPeriodEnd":true,"reason":"too expensive"}`, ""},
		{"POST", "/api/v1/subscriptions/subscriptions/sub_1/pause", ``, ""},
		{"POST", "/api/v1/subscriptions/subscriptions/sub_1/pause", `{"resumeAt":"2026-12-01T00:00:00Z"}`, ""},
		{"POST", "/api/v1/subscriptions/subscriptions/sub_1/resume", ``, ""},
		{"POST", "/api/v1/subscriptions/subscriptions/sub_1/change-plan", `{"newPlanId":"plan_2","behavior":"NEXT_PERIOD","idempotencyKey":"idem-cp-1"}`, "idem-cp-1"},
		{"DELETE", "/api/v1/subscriptions/cards/card_1", ``, ""},
		{"POST", "/api/v1/subscriptions/invoices/inv_1/retry-charge", ``, "idem-retry-1"},
		{"DELETE", "/api/panel/v1/subscriptions/portal/links/lnk_1", ``, ""},
		{"DELETE", "/api/panel/v1/subscriptions/portal/links/lnk_1", `{"reason":"customer left"}`, ""},
	}
	for i, expected := range want {
		got := sent[i]
		if got.Method != expected.method || got.URI != expected.uri || string(got.Body) != expected.body || got.Key != expected.key || got.Signed {
			t.Fatalf("request %d: %+v (body %s)", i, got, got.Body)
		}
	}
}

func TestSubscriptionPlanCreateNormalisesTheRule(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"id":"plan_1","price":"0"}}`)
	})
	_, err := client.Subscriptions.Plans.Create(context.Background(), map[string]any{
		"productId": "prod_1", "code": "free", "name": "Free", "price": "0", "currency": "try", "billing": map[string]any{"frequency": "monthly"},
	}, "idem-plan-1")
	if err != nil {
		t.Fatal(err)
	}
	sent := requests()
	body := decodeBody(t, sent[0].Body)
	billing, _ := body["billing"].(map[string]any)
	if sent[0].Key != "idem-plan-1" || body["currency"] != "TRY" || body["price"] != "0" || billing["frequency"] != "MONTHLY" || billing["interval"] != float64(1) {
		t.Fatalf("request %+v body %s", sent[0], sent[0].Body)
	}
}

func TestSubscriptionListsReadThePageToken(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.Contains(r.URL.RawQuery, "pageToken=tok_next") {
			productJSON(w, 200, `{"success":true,"data":[{"id":"plan_2","price":"4900"}],"pageSize":20}`)
			return
		}
		productJSON(w, 200, `{"success":true,"data":[{"id":"plan_1","price":"9900"}],"nextPageToken":"tok_next","pageSize":20}`)
	})
	ctx := context.Background()
	page, err := client.Subscriptions.Plans.List(ctx, map[string]any{"includeArchived": false, "productId": "prod_1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor != "tok_next" || !page.HasMore || page.PageSize == nil || *page.PageSize != 20 {
		t.Fatalf("page %+v", page)
	}
	if uri := requests()[0].URI; !strings.Contains(uri, "includeArchived=false") || !strings.Contains(uri, "productId=prod_1") {
		t.Fatalf("uri %s", uri)
	}
	var ids []string
	for row, err := range client.Subscriptions.Plans.Iterate(ctx, map[string]any{"productId": "prod_1"}) {
		if err != nil {
			t.Fatal(err)
		}
		var plan struct{ ID string }
		_ = json.Unmarshal(row, &plan)
		ids = append(ids, plan.ID)
	}
	if strings.Join(ids, ",") != "plan_1,plan_2" {
		t.Fatalf("iterated %v", ids)
	}
}

func TestSubscriptionCardListDefaultsIncludeInactive(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":[]}`)
	})
	if _, err := client.Subscriptions.Cards.List(context.Background(), "cus_1", map[string]any{"pageSize": 10}); err != nil {
		t.Fatal(err)
	}
	if uri := requests()[0].URI; uri != "/api/v1/subscriptions/cards?customerId=cus_1&includeInactive=false&pageSize=10" {
		t.Fatalf("uri %s", uri)
	}
}

func TestSubscriptionAnswerWithNumericAmountIsRefused(t *testing.T) {
	client, _ := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"id":"inv_1","totalAmount":9900}}`)
	})
	_, err := client.Subscriptions.Invoices.Get(context.Background(), "inv_1")
	mustReject(t, "numeric answer amount", err)
	if !strings.Contains(err.Error(), "totalAmount") {
		t.Fatalf("error %v", err)
	}
}

func TestSubscriptionPortal(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			productJSON(w, 200, `{"success":true,"code":"OK","portalToken":"cp_abc","sessionProof":"hmac_xyz","nonce":"nonce_1","expiresAt":"2026-06-16T03:00:00Z"}`)
			return
		}
		productJSON(w, 201, `{"success":true,"data":{"id":"lnk_1","token":"cp_xyz","status":"ACTIVE"}}`)
	})
	ctx := context.Background()
	session, err := client.Subscriptions.Portal.CreateSdkSession(ctx, "cus_1", "https://shop.example", SubscriptionPortalSessionOptions{SdkMode: SubscriptionPortalSdkModeHybrid})
	if err != nil {
		t.Fatal(err)
	}
	var proof struct{ PortalToken, SessionProof, ProofNonce, ProofExpiresAt string }
	_ = json.Unmarshal(session, &proof)
	if proof.PortalToken != "cp_abc" || proof.SessionProof != "hmac_xyz" || proof.ProofNonce != "nonce_1" || proof.ProofExpiresAt == "" {
		t.Fatalf("session %s", session)
	}
	link, err := client.Subscriptions.Portal.CreatePortalLink(ctx, "cus_1", map[string]any{"allowedPlanIds": []string{"plan_1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := SubscriptionPortalURL(link, ""); got != "https://portal.lunixi.com/?token=cp_xyz" {
		t.Fatalf("portal url %s", got)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/subscriptions/portal/sdk-session" || string(sent[0].Body) != `{"customerId":"cus_1","origin":"https://shop.example","sdkMode":"HYBRID"}` {
		t.Fatalf("session request %+v body %s", sent[0], sent[0].Body)
	}
	if sent[1].URI != "/api/panel/v1/subscriptions/portal/links" || string(sent[1].Body) != `{"allowedPlanIds":["plan_1"],"customerId":"cus_1"}` || sent[1].Signed {
		t.Fatalf("link request %+v body %s", sent[1], sent[1].Body)
	}
}

func TestSubscriptionCatalogUsageRoutes(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"items":[],"pageInfo":{"hasMore":false,"nextCursor":null}}}`)
	})
	ctx := context.Background()
	page, err := client.Subscriptions.Products.List(ctx, map[string]any{"limit": 5, "active": true})
	if err != nil || page.HasMore {
		t.Fatalf("products %+v %v", page, err)
	}
	steps := []func() error{
		func() error {
			_, err := client.Subscriptions.Products.UpsertTranslation(ctx, "prod_1", "en", map[string]any{"name": "Pro"})
			return err
		},
		func() error { _, err := client.Subscriptions.Features.Delete(ctx, "feat_1"); return err },
		func() error {
			_, err := client.Subscriptions.Features.List(ctx, map[string]any{"anything": "goes"})
			return err
		},
		func() error { _, err := client.Subscriptions.Addons.Deactivate(ctx, "addon_1"); return err },
		func() error {
			_, err := client.Subscriptions.Usage.Record(ctx, map[string]any{"subscriptionId": "sub_1", "featureKey": "api_calls", "quantity": 3})
			return err
		},
		func() error {
			_, err := client.Subscriptions.Usage.CheckEntitlement(ctx, map[string]any{"subscriptionId": "sub_1", "featureKey": "api_calls"})
			return err
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	sent := requests()
	want := []struct{ method, uri string }{
		{"GET", "/api/v1/subscriptions/products?active=true&limit=5"},
		{"PATCH", "/api/v1/subscriptions/products/prod_1/translations/en"},
		{"DELETE", "/api/v1/subscriptions/features/feat_1"},
		{"GET", "/api/v1/subscriptions/features?anything=goes"},
		{"DELETE", "/api/v1/subscriptions/addons/addon_1"},
		{"POST", "/api/v1/subscriptions/usage/record"},
		{"POST", "/api/v1/subscriptions/usage/check-entitlement"},
	}
	for i, expected := range want {
		if sent[i].Method != expected.method || sent[i].URI != expected.uri {
			t.Fatalf("request %d: %+v", i, sent[i])
		}
	}
}

func TestOnlyThePortalPanelFamilyIsReachable(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	ctx := context.Background()
	for _, path := range []string{"/api/panel/v1/merchants", "/api/panel/v1/subscriptions/portal", "/api/panel/v1/subscriptions/portal/../../merchants"} {
		_, err := client.Do(ctx, Request{Method: "GET", Path: path})
		mustReject(t, path, err)
	}
	if _, err := client.Do(ctx, Request{Method: "GET", Path: "/api/panel/v1/subscriptions/portal/settings"}); err != nil {
		t.Fatal(err)
	}
	if sent := requests(); len(sent) != 1 || !sent[0].Signed {
		t.Fatalf("requests %+v", sent)
	}
}

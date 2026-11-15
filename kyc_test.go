package lunixi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestKycRoutesAreBearerOnly(t *testing.T) {
	for name, route := range KycRoutes() {
		if route.StepUp || !strings.HasPrefix(route.Path, "/api/v1/kyc/") || strings.Contains(route.Path, "/sdk/init") {
			t.Errorf("%s: %+v", name, route)
		}
	}
	if len(KycEventTypes()) != 25 || len(KycSessionStatuses()) != 17 || KycEventMonitoringAlertCreated != "ongoing_monitoring.alert.created" {
		t.Fatal("kyc values")
	}
	if !IsKycSessionTerminal("manual_review") || IsKycSessionTerminal(KycSessionStatusProcessing) {
		t.Fatal("terminal states")
	}
	if !KycAssuranceMeets(KycAssuranceLevelHigh, KycAssuranceLevelSubstantial) || KycAssuranceMeets(KycAssuranceLevelNotPerformed, KycAssuranceLevelLow) {
		t.Fatal("assurance ladder")
	}
}

func TestKycCreateSessionBody(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"session":{"sessionId":"sess_1","status":"CREATED"}}}`)
	})
	data, err := client.Kyc.CreateSession(context.Background(), map[string]any{
		"subjectId": "42", "sessionType": "kyc", "externalCustomerId": "42", "jurisdiction": "TR",
		"person": map[string]any{"firstName": "Ada"}, "metadata": map[string]any{"orderId": "o-1"},
	}, "idem-kyc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "sess_1") {
		t.Fatalf("data %s", data)
	}
	sent := requests()
	if sent[0].Method != "POST" || sent[0].URI != "/api/v1/kyc/sessions" || sent[0].Key != "idem-kyc-1" || sent[0].Signed || !sent[0].Bearer {
		t.Fatalf("request %+v", sent[0])
	}
	want := `{"subjectId":"42","sessionType":"KYC","externalCustomerId":"42","customer":{"customerType":"PERSON","firstName":"Ada"},"jurisdiction":"TR","idempotencyKey":"idem-kyc-1","evidenceContextJson":"{\"orderId\":\"o-1\"}"}`
	if string(sent[0].Body) != want {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestKycRefusesBeforeSending(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*Client) error{
		"session type": func(c *Client) error {
			_, err := c.Kyc.CreateSession(ctx, map[string]any{"subjectId": "42", "sessionType": "AML"}, "")
			return err
		},
		"two identities": func(c *Client) error {
			_, err := c.Kyc.CreateSession(ctx, map[string]any{"subjectId": "42", "sessionType": KycSessionTypeKYB, "person": map[string]any{}, "company": map[string]any{}}, "")
			return err
		},
		"ttl": func(c *Client) error {
			_, err := c.Kyc.CreateSession(ctx, map[string]any{"subjectId": "42", "sessionType": KycSessionTypeKYC, "ttlMinutes": 2000}, "")
			return err
		},
		"unknown field": func(c *Client) error {
			_, err := c.Kyc.EvidenceRequest(ctx, map[string]any{"subjectId": "42", "sessionType": KycSessionTypeEvidence, "status": "APPROVED"}, "")
			return err
		},
		"metadata and context": func(c *Client) error {
			_, err := c.Kyc.CreateSession(ctx, map[string]any{"subjectId": "42", "sessionType": "KYC", "metadata": map[string]any{}, "evidenceContextJson": "{}"}, "")
			return err
		},
		"list filter": func(c *Client) error {
			_, err := c.Kyc.ListSessions(ctx, map[string]any{"status": "APPROVED"})
			return err
		},
		"blank jurisdiction": func(c *Client) error {
			_, err := c.Kyc.SetJurisdiction(ctx, "sess_1", " ")
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestKycUploadDocumentNeverEchoesTheImage(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	_, err := client.Kyc.UploadDocument(context.Background(), "sess_1", map[string]any{"documentType": "PASSPORT", "fileBase64": "SECRETIMAGEDATA", "mimeType": "image/jpeg", "extra": "SECRETIMAGEDATA"})
	expectLocalError(t, "upload", err, requests())
	if strings.Contains(err.Error(), "SECRETIMAGEDATA") {
		t.Fatalf("error echoes the image: %v", err)
	}
}

func TestKycListSessionsPaging(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			productJSON(w, 200, `{"status":"success","data":{"items":[{"sessionId":"s1"},{"sessionId":"s2"}],"nextCursor":"tok2"}}`)
			return
		}
		productJSON(w, 200, `{"status":"success","data":{"items":[{"sessionId":"s3"}],"nextCursor":"stale","pageInfo":{"hasMore":false,"nextCursor":null}}}`)
	})
	ctx := context.Background()
	page, err := client.Kyc.ListSessions(ctx, map[string]any{"pageSize": 2, "pageToken": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor != "tok2" || !page.HasMore {
		t.Fatalf("page %+v", page)
	}
	if uri := requests()[0].URI; uri != "/api/v1/kyc/sessions?cursor=abc&limit=2" {
		t.Fatalf("uri %s", uri)
	}
	last, err := client.Kyc.ListSessions(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if last.NextCursor != "" || last.HasMore {
		t.Fatalf("a pageInfo without a cursor is the last page: %+v", last)
	}
}

func TestKycSessionAndAdminRoutes(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	ctx := context.Background()
	steps := []func() error{
		func() error { _, err := client.Kyc.SetPrimaryDocument(ctx, "sess_1", "PASSPORT"); return err },
		func() error { _, err := client.Kyc.SetJurisdiction(ctx, "sess_1", "TR"); return err },
		func() error { _, err := client.Kyc.Submit(ctx, "sess_1"); return err },
		func() error { _, err := client.Kyc.ManualReview(ctx, "sess_1", "", ""); return err },
		func() error { _, err := client.Kyc.ManualReview(ctx, "sess_1", "DOC_BLUR", ""); return err },
		func() error { _, err := client.Kyc.MerchantPolicy(ctx); return err },
		func() error { _, err := client.Kyc.RotateWebhookSecret(ctx); return err },
		func() error {
			_, err := client.Kyc.WebhookOutbox(ctx, map[string]any{"status": "FAILED", "limit": 10})
			return err
		},
		func() error { _, err := client.Kyc.RetryWebhookEvent(ctx, "evt_1"); return err },
		func() error { _, err := client.Kyc.ErasureRequests(ctx, map[string]any{"limit": 5}); return err },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	sent := requests()
	want := []struct{ method, uri, body string }{
		{"POST", "/api/v1/kyc/sessions/sess_1/primary-document", `{"documentType":"PASSPORT"}`},
		{"POST", "/api/v1/kyc/sessions/sess_1/jurisdiction", `{"jurisdiction":"TR"}`},
		{"POST", "/api/v1/kyc/sessions/sess_1/submit", ``},
		{"POST", "/api/v1/kyc/sessions/sess_1/manual-review", ``},
		{"POST", "/api/v1/kyc/sessions/sess_1/manual-review", `{"reasonCode":"DOC_BLUR"}`},
		{"GET", "/api/v1/kyc/admin/merchant-policy", ``},
		{"POST", "/api/v1/kyc/admin/merchant-policy/webhook-secret/rotate", ``},
		{"GET", "/api/v1/kyc/admin/webhook-outbox?limit=10&status=FAILED", ``},
		{"POST", "/api/v1/kyc/admin/webhook-outbox/evt_1/retry", ``},
		{"GET", "/api/v1/kyc/admin/erasure-requests?limit=5", ``},
	}
	for i, expected := range want {
		if sent[i].Method != expected.method || sent[i].URI != expected.uri || string(sent[i].Body) != expected.body || sent[i].Signed {
			t.Fatalf("request %d: %+v (body %s)", i, sent[i], sent[i].Body)
		}
	}
}

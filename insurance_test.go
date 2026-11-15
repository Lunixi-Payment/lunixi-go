package lunixi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	testInsuranceKey     = "ins-order-0001"
	testInsurancePayment = "6f1c2b1e-1111-4222-8333-444455556666"
)

func TestInsuranceCreateQuoteBasketRule(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"options":[{"quoteId":"insq_1","premiumAmount":"4500"}]}}`)
	})
	items := []BasketItem{{ID: "line-1", Name: "Ticket", ItemType: "VIRTUAL", Price: "150000", InsuranceCategory: "TICKET"}}
	_, err := client.Insurance.CreateQuote(context.Background(), map[string]any{
		"insuranceType":   " TICKET_CANCELLATION ",
		"currency":        "try",
		"context":         map[string]any{"items": items},
		"merchantOrderId": "",
		"tickets":         []map[string]any{{"basketItemId": "line-1"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if len(sent) != 1 || sent[0].Method != "POST" || sent[0].URI != "/api/v1/insurance/quotes" || sent[0].Key != "" {
		t.Fatalf("request %+v", sent)
	}
	body := decodeBody(t, sent[0].Body)
	if body["insuranceType"] != "TICKET_CANCELLATION" || body["currency"] != "TRY" || body["merchantOrderId"] != nil || body["paymentIntentId"] != nil || body["tickets"] == nil {
		t.Fatalf("body %s", sent[0].Body)
	}
	if basket, ok := body["context"].(map[string]any); !ok || len(basket["items"].([]any)) != 1 {
		t.Fatalf("context %s", sent[0].Body)
	}

	client, requests = newProductClient(t, 0, nil)
	_, err = client.Insurance.CreateQuote(context.Background(), map[string]any{
		"insuranceType": "TICKET_CANCELLATION", "currency": "TRY", "paymentIntentId": testInsurancePayment, "context": map[string]any{"items": items},
	}, "")
	expectLocalError(t, "paymentIntentId and context.items", err, requests())
}

func TestInsuranceKeys(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	ctx := context.Background()
	if _, err := client.Insurance.CreateQuote(ctx, map[string]any{"insuranceType": "TRAVEL", "currency": "TRY"}, "trace-0001"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insurance.BindPayment(ctx, "inso_1", testInsurancePayment, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insurance.IssuePolicy(ctx, "inso_1", testInsuranceKey); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insurance.RefundPremium(ctx, "inso_1", nil, testInsuranceKey); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	want := []productRecorded{
		{Method: "POST", URI: "/api/v1/insurance/quotes", Key: "trace-0001", Body: []byte(`{"currency":"TRY","insuranceType":"TRAVEL"}`)},
		{Method: "POST", URI: "/api/v1/insurance/orders/inso_1/bind-payment", Body: []byte(`{"paymentIntentId":"` + testInsurancePayment + `"}`)},
		{Method: "POST", URI: "/api/v1/insurance/orders/inso_1/issue-policy", Key: testInsuranceKey},
		{Method: "POST", URI: "/api/v1/insurance/orders/inso_1/refund-premium", Key: testInsuranceKey, Body: []byte(`{}`)},
	}
	for i, request := range sent {
		if request.Method != want[i].Method || request.URI != want[i].URI || request.Key != want[i].Key || string(request.Body) != string(want[i].Body) {
			t.Fatalf("request %d %+v (%s), want %+v", i, request, request.Body, want[i])
		}
	}
	required := map[string]func(*Client, string) error{
		"createOrder": func(c *Client, key string) error {
			_, err := c.Insurance.CreateOrder(ctx, map[string]any{"quoteId": "insq_1", "consent": map[string]any{"termsVersion": "v1", "termsSha256": "abc", "acceptedAt": "2026-11-15T10:00:00Z"}}, key)
			return err
		},
		"confirmPayment": func(c *Client, key string) error {
			_, err := c.Insurance.ConfirmPayment(ctx, "inso_1", map[string]any{"externalPaymentId": "ext-1", "paidAmount": "4500", "paidCurrency": "TRY", "paidAt": "2026-11-15T10:00:00Z"}, key)
			return err
		},
		"issuePolicy": func(c *Client, key string) error {
			_, err := c.Insurance.IssuePolicy(ctx, "inso_1", key)
			return err
		},
		"cancelOrder": func(c *Client, key string) error {
			_, err := c.Insurance.CancelOrder(ctx, "inso_1", key)
			return err
		},
		"refundPremium": func(c *Client, key string) error {
			_, err := c.Insurance.RefundPremium(ctx, "inso_1", nil, key)
			return err
		},
		"createLead": func(c *Client, key string) error {
			_, err := c.Insurance.CreateLead(ctx, map[string]any{"insuranceType": "CARGO_TRANSIT"}, key)
			return err
		},
	}
	for name, call := range required {
		for _, key := range []string{"", "short", "key with space"} {
			client, requests := newProductClient(t, 0, nil)
			expectLocalError(t, name+" key "+key, call(client, key), requests())
		}
	}
	client, requests = newProductClient(t, 0, nil)
	_, err := client.Insurance.BindPayment(ctx, "inso_1", testInsurancePayment, "1234567")
	expectLocalError(t, "optional key format", err, requests())
}

func TestInsuranceRefusesBeforeSending(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*Client) error{
		"unknown quote field": func(c *Client) error {
			_, err := c.Insurance.CreateQuote(ctx, map[string]any{"insuranceType": "TRAVEL", "currency": "TRY", "amount": "100"}, "")
			return err
		},
		"empty context": func(c *Client) error {
			_, err := c.Insurance.CreateQuote(ctx, map[string]any{"insuranceType": "TRAVEL", "currency": "TRY", "context": map[string]any{"items": []any{}}}, "")
			return err
		},
		"payment intent id": func(c *Client) error {
			_, err := c.Insurance.CreateQuote(ctx, map[string]any{"insuranceType": "TRAVEL", "currency": "TRY", "paymentIntentId": "pi-1"}, "")
			return err
		},
		"consent field": func(c *Client) error {
			_, err := c.Insurance.CreateOrder(ctx, map[string]any{"quoteId": "insq_1", "consent": map[string]any{"termsVersion": "v1", "termsSha256": "abc", "acceptedAt": "2026-11-15T10:00:00Z", "ip": "1.2.3.4"}}, testInsuranceKey)
			return err
		},
		"missing consent": func(c *Client) error {
			_, err := c.Insurance.CreateOrder(ctx, map[string]any{"quoteId": "insq_1"}, testInsuranceKey)
			return err
		},
		"numeric paid amount": func(c *Client) error {
			_, err := c.Insurance.ConfirmPayment(ctx, "inso_1", map[string]any{"externalPaymentId": "ext-1", "paidAmount": 4500, "paidCurrency": "TRY", "paidAt": "2026-11-15T10:00:00Z"}, testInsuranceKey)
			return err
		},
		"refund field": func(c *Client) error {
			_, err := c.Insurance.RefundPremium(ctx, "inso_1", map[string]any{"externalRefundId": "r-1", "refundedAt": "2026-11-15T10:00:00Z", "amount": "1"}, testInsuranceKey)
			return err
		},
		"lead currency": func(c *Client) error {
			_, err := c.Insurance.CreateLead(ctx, map[string]any{"insuranceType": "CARGO_TRANSIT", "currency": "TRY"}, testInsuranceKey)
			return err
		},
		"lead note length": func(c *Client) error {
			_, err := c.Insurance.CreateLead(ctx, map[string]any{"insuranceType": "CARGO_TRANSIT", "note": strings.Repeat("ş", 1001)}, testInsuranceKey)
			return err
		},
		"products without currency": func(c *Client) error {
			_, err := c.Insurance.ListProducts(ctx, map[string]any{"insuranceType": "TRAVEL"})
			return err
		},
		"summary without to": func(c *Client) error {
			_, err := c.Insurance.GetReconciliationSummary(ctx, map[string]any{"from": time.Now()})
			return err
		},
		"lead status": func(c *Client) error {
			_, err := c.Insurance.ListLeads(ctx, map[string]any{"status": "DONE"})
			return err
		},
		"limit": func(c *Client) error {
			_, err := c.Insurance.ListOrders(ctx, map[string]any{"limit": 101})
			return err
		},
		"unknown filter": func(c *Client) error {
			_, err := c.Insurance.ListOrders(ctx, map[string]any{"status": "ACTIVE"})
			return err
		},
		"reference kind": func(c *Client) error {
			_, err := c.Insurance.ListReferenceValues(ctx, "ADDRESS_STREET", nil)
			return err
		},
		"search without items": func(c *Client) error {
			_, err := c.Insurance.SearchProducts(ctx, map[string]any{"currency": "TRY"})
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestInsuranceLeadNoteIsSentAsGiven(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	_, err := client.Insurance.CreateLead(context.Background(), map[string]any{"insuranceType": "CARGO_TRANSIT", "note": "  call after 5pm  ", "shipment": map[string]any{"origin": "IST"}}, "lead-key-0001")
	if err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/insurance/leads" || sent[0].Key != "lead-key-0001" || string(sent[0].Body) != `{"insuranceType":"CARGO_TRANSIT","note":"  call after 5pm  ","shipment":{"origin":"IST"}}` {
		t.Fatalf("request %+v %s", sent[0], sent[0].Body)
	}
}

func TestInsurancePolicyFilesAreBytes(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.HasSuffix(r.URL.Path, "/receipt") {
			productJSON(w, 412, `{"success":false,"code":"insurance.document.not_supported"}`)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7"))
	})
	document, err := client.Insurance.GetPolicyDocument(context.Background(), "insp_1")
	if err != nil {
		t.Fatal(err)
	}
	if document.ContentType != "application/pdf" || string(document.Bytes) != "%PDF-1.7" {
		t.Fatalf("document %+v", document)
	}
	if _, err = client.Insurance.GetPolicyCertificate(context.Background(), "insp_1"); err != nil {
		t.Fatal(err)
	}
	_, err = client.Insurance.GetPolicyReceipt(context.Background(), "insp_1")
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 412 || apiError.Code != "insurance.document.not_supported" {
		t.Fatalf("receipt error %v", err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/insurance/policies/insp_1/document" || sent[0].Accept != insurancePDFAccept || sent[1].URI != "/api/v1/insurance/policies/insp_1/certificate" {
		t.Fatalf("requests %+v", sent)
	}
}

func TestInsuranceReconciliationExportTrailer(t *testing.T) {
	bodies := []string{
		"\ufefforderId,premiumAmount\r\ninso_1,4500\r\n# truncated=true orderCap=20000 nextCursor=cur_2\r\n",
		"\ufefforderId,premiumAmount\ninso_1,4500\n# export-aborted=true\n",
		"\ufefforderId,premiumAmount\ninso_1,4500\n",
	}
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, call int) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(bodies[call]))
	})
	filters := map[string]any{"from": "2026-10-01T00:00:00Z", "to": "2026-11-01T00:00:00Z", "currency": "TRY"}
	want := []InsuranceReconciliationExport{{Truncated: true, NextCursor: "cur_2"}, {Aborted: true}, {}}
	for i := range bodies {
		export, err := client.Insurance.ExportReconciliation(context.Background(), filters)
		if err != nil {
			t.Fatal(err)
		}
		if export.Truncated != want[i].Truncated || export.NextCursor != want[i].NextCursor || export.Aborted != want[i].Aborted || string(export.Bytes) != bodies[i] {
			t.Fatalf("export %d %+v", i, export)
		}
	}
	if sent := requests(); sent[0].URI != "/api/v1/insurance/reconciliation/export?currency=TRY&from=2026-10-01T00%3A00%3A00Z&to=2026-11-01T00%3A00%3A00Z" || sent[0].Accept != insuranceCSVAccept {
		t.Fatalf("request %+v", sent[0])
	}
}

func TestInsuranceHelpers(t *testing.T) {
	insurance := &InsuranceClient{}
	option := json.RawMessage(`{"quoteId":"insq_1","terms":{"version":"2026-11","sha256":"9f86d081"},"premiumAmount":"4500"}`)
	at := time.Date(2026, 11, 15, 13, 0, 0, 0, time.FixedZone("TRT", 3*3600))
	consent, err := insurance.ConsentFromQuote(option, at)
	if err != nil {
		t.Fatal(err)
	}
	if consent["termsVersion"] != "2026-11" || consent["termsSha256"] != "9f86d081" || consent["acceptedAt"] != "2026-11-15T10:00:00.000Z" {
		t.Fatalf("consent %v", consent)
	}
	if _, err = insurance.ConsentFromQuote(json.RawMessage(`{"quoteId":"insq_1"}`), at); !errors.Is(err, ErrConfiguration) {
		t.Fatalf("option without terms: %v", err)
	}
	order := json.RawMessage(`{"orderId":"inso_1","premiumCollectedBy":"MERCHANT","premiumAmount":"4500"}`)
	item, err := insurance.PremiumBasketItem(order, "Ticket insurance", "Insurance", "")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "inso_1" || item.Name != "Ticket insurance" || item.ItemType != "VIRTUAL" || item.Price != "4500" || item.Category1 != "Insurance" || item.Category2 != "" || item.InsuranceCategory != "" {
		t.Fatalf("item %+v", item)
	}
	provider := json.RawMessage(`{"orderId":"inso_1","premiumCollectedBy":"PROVIDER","premiumAmount":"4500"}`)
	if _, err = insurance.PremiumBasketItem(provider, "Ticket insurance", "Insurance", ""); !errors.Is(err, ErrConfiguration) {
		t.Fatalf("provider order: %v", err)
	}
}

func TestInsuranceWebhookData(t *testing.T) {
	order := productWebhook(t, `{"id":"evt-1","type":"insurance.policy.failed","data":{"eventId":"evt-1","eventType":"insurance.policy.failed","merchantId":"m-1","insuranceOrderId":"inso_1","paymentIntentId":null,"merchantOrderId":"ORD-1","insuranceType":"TICKET_CANCELLATION","premiumCollectedBy":"MERCHANT","provider":{"code":"P1","name":"Provider","logoUrl":null,"connectionId":"con-1"},"status":"FAILED","collectionStatus":"FAILED","policyStatus":"NOT_REQUESTED","refundStatus":"NONE","documentStatus":"NOT_AVAILABLE","premiumAmount":"4500","currency":"TRY","policies":[{"policyId":"insp_1","basketItemId":"line-1","policyNumber":null,"policyStatus":"NOT_REQUESTED","documentStatus":"NOT_AVAILABLE","documentUrl":""}],"errorCode":"insurance.premium.not_captured","createdAt":"2026-11-15T10:00:00Z"}}`)
	data, err := ParseInsuranceWebhook(order)
	if err != nil {
		t.Fatal(err)
	}
	if data.InsuranceOrderID != "inso_1" || data.PaymentIntentID != nil || data.ErrorCode == nil || len(data.Policies) != 1 || data.Provider.ConnectionID != "con-1" {
		t.Fatalf("data %+v", data)
	}
	if _, err = ParseInsuranceLeadWebhook(order); !errors.Is(err, ErrWebhook) {
		t.Fatalf("order event read as a lead: %v", err)
	}
	lead := productWebhook(t, `{"id":"evt-1","type":"insurance.lead.submitted","data":{"eventId":"evt-1","eventType":"insurance.lead.submitted","merchantId":"m-1","lead":{"leadId":"insl_1","insuranceType":"CARGO_TRANSIT","schemaVersion":1,"status":"SUBMITTED","submittedAt":"2026-11-15T10:00:00Z","createdAt":"2026-11-15T09:59:00Z"},"createdAt":"2026-11-15T10:00:01Z"}}`)
	leadData, err := ParseInsuranceLeadWebhook(lead)
	if err != nil {
		t.Fatal(err)
	}
	if leadData.Lead.LeadID != "insl_1" || leadData.Lead.Status != InsuranceLeadStatusSubmitted || leadData.Lead.SubmittedAt == nil {
		t.Fatalf("lead %+v", leadData)
	}
	if _, err = ParseInsuranceWebhook(lead); !errors.Is(err, ErrWebhook) {
		t.Fatalf("lead event read as an order: %v", err)
	}
	numeric := productWebhook(t, `{"id":"evt-1","type":"insurance.policy.issued","data":{"premiumAmount":4500}}`)
	if _, err = ParseInsuranceWebhook(numeric); !errors.Is(err, ErrWebhook) {
		t.Fatalf("numeric premium: %v", err)
	}
}

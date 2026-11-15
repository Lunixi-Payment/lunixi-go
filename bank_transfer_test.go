package lunixi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func intPointer(value int) *int { return &value }

func TestBankTransferExceptionFilters(t *testing.T) {
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":"6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f","kind":"UNDERPAID","status":"PENDING_DECISION","amountMinor":"25000","currency":"TRY"}]}}`)
	})
	ctx := context.Background()
	for name, filters := range map[string]BankTransferExceptionFilters{
		"unknown environment": {Environment: "PROD"},
		"lowercase status":    {Status: "pending_decision"},
		"unknown status":      {Status: "OPEN"},
		"limit 0":             {Limit: intPointer(0)},
		"limit 201":           {Limit: intPointer(201)},
		"negative limit":      {Limit: intPointer(-1)},
	} {
		_, e := client.Payments.ListBankTransferExceptions(ctx, filters)
		mustReject(t, name, e)
	}
	if len(requests()) != 0 {
		t.Fatal("invalid filters reached network")
	}
	if _, e := client.Payments.ListBankTransferExceptions(ctx, BankTransferExceptionFilters{Environment: "LIVE", Status: BankTransferExceptionStatusPendingDecision, Limit: intPointer(50)}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ListBankTransferExceptions(ctx, BankTransferExceptionFilters{}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ListBankTransferExceptions(ctx, BankTransferExceptionFilters{Limit: intPointer(BankTransferExceptionLimitMax)}); e != nil {
		t.Fatal(e)
	}
	sent := requests()
	want := []string{
		"/api/v1/payments/bank-transfer/exceptions?environment=LIVE&limit=50&status=PENDING_DECISION",
		"/api/v1/payments/bank-transfer/exceptions",
		"/api/v1/payments/bank-transfer/exceptions?limit=200",
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

func TestBankTransferExceptionDecisions(t *testing.T) {
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":null}`)
	})
	ctx := context.Background()
	for _, id := range []string{"", "exception-1", "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5", "6f1c2b9e1d2a4c3b9e8f0a1b2c3d4e5f", "../6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f"} {
		_, e := client.Payments.AcceptBankTransferException(ctx, id)
		mustReject(t, "accept "+id, e)
		_, e = client.Payments.MarkBankTransferExceptionRefunded(ctx, id)
		mustReject(t, "refunded "+id, e)
	}
	if len(requests()) != 0 {
		t.Fatal("invalid exception id reached network")
	}
	response, e := client.Payments.AcceptBankTransferException(ctx, "6F1C2B9E-1D2A-4C3B-9E8F-0A1B2C3D4E5F")
	if e != nil {
		t.Fatal(e)
	}
	if string(response.Data) != "null" {
		t.Fatalf("data %s", response.Data)
	}
	if _, e = client.Payments.MarkBankTransferExceptionRefunded(ctx, "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f"); e != nil {
		t.Fatal(e)
	}
	sent := requests()
	if len(sent) != 2 || sent[0].URI != "/api/v1/payments/bank-transfer/exceptions/6F1C2B9E-1D2A-4C3B-9E8F-0A1B2C3D4E5F/accept" || sent[1].URI != "/api/v1/payments/bank-transfer/exceptions/6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f/refunded" {
		t.Fatalf("requests %+v", sent)
	}
	for _, r := range sent {
		if r.Method != "POST" || len(r.Body) != 0 || !r.Signed {
			t.Fatalf("decision request %+v", r)
		}
	}
}

func TestTurkishAddressReference(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Reference.ListProvinces(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Reference.ListDistricts(ctx, "06"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Reference.ListNeighborhoods(ctx, "1234"); e != nil {
		t.Fatal(e)
	}
	for _, code := range []string{"", "6", "00", "82", "6a"} {
		_, e := client.Reference.ListDistricts(ctx, code)
		mustReject(t, "province "+code, e)
	}
	for _, code := range []string{"", "12a", "1234567890123"} {
		_, e := client.Reference.ListNeighborhoods(ctx, code)
		mustReject(t, "district "+code, e)
	}
	sent := requests()
	want := []string{"/api/v1/reference/tr-address/provinces", "/api/v1/reference/tr-address/districts?provinceCode=06", "/api/v1/reference/tr-address/neighborhoods?districtCode=1234"}
	if len(sent) != len(want) {
		t.Fatalf("requests %+v", sent)
	}
	for i, r := range sent {
		if r.URI != want[i] || r.Method != "GET" {
			t.Fatalf("request %d: %+v", i, r)
		}
	}
}

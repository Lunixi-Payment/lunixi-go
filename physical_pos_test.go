package lunixi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testPosKey      = "pos-key-0001"
	testPosDeviceID = "5f0c2b1e-1111-4222-8333-444455556666"
	testPosPayment  = `{"success":true,"data":{"payment":{"paymentId":"pp-1","paymentStatus":"NOT_STARTED","amountMinor":"4500"},"replayed":false}}`
)

type posRecorded struct {
	Method, URI, Key string
	Body             []byte
}

// newPosClient is a signed test client with retries; reply answers the
// call-th physical POS request (0-based). A nil reply answers a new payment.
func newPosClient(t *testing.T, retries int, reply func(w http.ResponseWriter, r *http.Request, call int)) (*Client, func() []posRecorded) {
	t.Helper()
	var mu sync.Mutex
	var recorded []posRecorded
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"at_TEST_pos","expires_in":1800}`)
			return
		}
		mu.Lock()
		call := len(recorded)
		recorded = append(recorded, posRecorded{Method: r.Method, URI: r.URL.RequestURI(), Key: r.Header.Get("Idempotency-Key"), Body: raw})
		mu.Unlock()
		if reply != nil {
			reply(w, r, call)
			return
		}
		posJSON(w, 202, testPosPayment)
	}, func(c *Config) { c.MaxRetries = retries })
	return client, func() []posRecorded {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(recorded)
	}
}

func posJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprint(w, body)
}

// posCutBody answers 202 and drops the connection in the middle of the
// body, so the client cannot read the answer.
func posCutBody(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", "200")
	w.WriteHeader(202)
	_, _ = w.Write([]byte(`{"success":true,"data":{"pay`))
	w.(http.Flusher).Flush()
	panic(http.ErrAbortHandler)
}

func posSale() map[string]any {
	return map[string]any{
		"operation":   PhysicalPosOperationSale,
		"target":      map[string]any{"deviceId": testPosDeviceID},
		"amountMinor": "4500",
		"currency":    "TRY",
	}
}

func TestPhysicalPosCreatePaymentBodyKeyAndStatus(t *testing.T) {
	client, requests := newPosClient(t, 2, nil)
	request := posSale()
	request["merchantReference"] = "ORD-2026-000123"
	request["allowDuplicateReference"] = false
	request["installmentCount"] = nil
	request["instrument"] = ""
	request["items"] = []map[string]any{{
		"lineNo": 1, "name": "Simit 8691234567012", "quantityMilli": 2000, "unitCode": "C62",
		"unitPriceMinor": "2250", "taxGroupCode": "KDV10", "lineTotalMinor": "4500",
	}}
	result, err := client.PhysicalPos.CreatePayment(context.Background(), request, testPosKey)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 202 || !result.Accepted() || result.Replayed || result.IdempotencyKey != testPosKey {
		t.Fatalf("result %+v", result)
	}
	var payment map[string]any
	if json.Unmarshal(result.Payment(), &payment) != nil || payment["paymentId"] != "pp-1" {
		t.Fatalf("payment %s", result.Payment())
	}
	sent := requests()
	if len(sent) != 1 || sent[0].Method != "POST" || sent[0].URI != "/api/v1/physical-pos/payments" || sent[0].Key != testPosKey {
		t.Fatalf("request %+v", sent)
	}
	want := `{"operation":"SALE","target":{"deviceId":"` + testPosDeviceID + `"},"amountMinor":"4500","currency":"TRY","installmentCount":null,"merchantReference":"ORD-2026-000123","allowDuplicateReference":false,"items":[{"lineNo":1,"name":"Simit 8691234567012","quantityMilli":2000,"unitCode":"C62","unitPriceMinor":"2250","taxGroupCode":"KDV10","lineTotalMinor":"4500"}]}`
	if string(sent[0].Body) != want {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestPhysicalPosReplayIsTwoHundred(t *testing.T) {
	client, _ := newPosClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 200, `{"success":true,"data":{"payment":{"paymentId":"pp-1","paymentStatus":"APPROVED"},"replayed":true}}`)
	})
	result, err := client.PhysicalPos.CreatePayment(context.Background(), posSale(), testPosKey)
	if err != nil || result.StatusCode != 200 || !result.Replayed || result.Accepted() {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestPhysicalPosPaymentWritesRequireAKey(t *testing.T) {
	client, requests := newPosClient(t, 0, nil)
	ctx := context.Background()
	if _, err := client.PhysicalPos.CreatePayment(ctx, posSale(), ""); !errors.Is(err, ErrConfiguration) {
		t.Fatal("payment without key")
	}
	if _, err := client.PhysicalPos.CreateReversal(ctx, "pp-1", map[string]any{"operation": "VOID", "currency": "TRY"}, ""); !errors.Is(err, ErrConfiguration) {
		t.Fatal("reversal without key")
	}
	for _, key := range []string{"short", " pos-key-1", "pos key 01", "pos-key-ş1"} {
		if _, err := client.PhysicalPos.CreatePayment(ctx, posSale(), key); !errors.Is(err, ErrConfiguration) {
			t.Fatalf("key %q accepted", key)
		}
	}
	if len(requests()) != 0 {
		t.Fatal("sent without a valid key")
	}
}

func TestPhysicalPosCancelAndSettlementGenerateAKey(t *testing.T) {
	client, requests := newPosClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.HasSuffix(r.URL.Path, "/settlement") {
			posJSON(w, 202, `{"success":true,"data":{"run":{"runId":"run-1","runStatus":"PENDING"},"replayed":false}}`)
			return
		}
		posJSON(w, 200, testPosPayment)
	})
	ctx := context.Background()
	cancelled, err := client.PhysicalPos.CancelPayment(ctx, "pp-1", "")
	if err != nil || cancelled.StatusCode != 200 || !uuidPattern.MatchString(cancelled.IdempotencyKey) {
		t.Fatalf("%+v %v", cancelled, err)
	}
	run, err := client.PhysicalPos.StartSettlement(ctx, testPosDeviceID, "eod-key-0001")
	if err != nil || run.StatusCode != 202 || run.IdempotencyKey != "eod-key-0001" || !strings.Contains(string(run.Record("run")), "run-1") {
		t.Fatalf("%+v %v", run, err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/physical-pos/payments/pp-1/cancel" || string(sent[0].Body) != "{}" || sent[0].Key != cancelled.IdempotencyKey {
		t.Fatalf("cancel %+v", sent[0])
	}
	if sent[1].URI != "/api/v1/physical-pos/devices/"+testPosDeviceID+"/settlement" || sent[1].Key != "eod-key-0001" {
		t.Fatalf("settlement %+v", sent[1])
	}
}

func TestPhysicalPosReversalBody(t *testing.T) {
	client, requests := newPosClient(t, 0, nil)
	request := map[string]any{"operation": "REFUND", "currency": "TRY", "items": []any{map[string]any{"lineNo": 1, "quantityMilli": 1000}}, "target": nil}
	if _, err := client.PhysicalPos.CreateReversal(context.Background(), "pp-1", request, testPosKey); err != nil {
		t.Fatal(err)
	}
	sent := requests()[0]
	if sent.URI != "/api/v1/physical-pos/payments/pp-1/reversals" || string(sent.Body) != `{"operation":"REFUND","currency":"TRY","target":null,"items":[{"lineNo":1,"quantityMilli":1000}]}` {
		t.Fatalf("%s %s", sent.URI, sent.Body)
	}
}

func TestPhysicalPosRequestChecksBeforeSending(t *testing.T) {
	client, requests := newPosClient(t, 0, nil)
	ctx := context.Background()
	with := func(key string, value any) map[string]any {
		request := posSale()
		request[key] = value
		return request
	}
	for name, request := range map[string]map[string]any{
		"numeric amount":      with("amountMinor", 4500),
		"decimal amount":      with("amountMinor", "45.00"),
		"zero amount":         with("amountMinor", "0"),
		"null amount":         with("amountMinor", nil),
		"unknown field":       with("note", "x"),
		"text count":          with("installmentCount", "3"),
		"fraction count":      with("installmentCount", 1.5),
		"bool as text":        with("allowDuplicateReference", "false"),
		"empty items":         with("items", []any{}),
		"item missing field":  with("items", []any{map[string]any{"lineNo": 1, "name": "Tea"}}),
		"unknown item field":  with("items", []any{map[string]any{"lineNo": 1, "color": "red"}}),
		"unknown target":      with("target", map[string]any{"terminalId": "t"}),
		"missing operation":   {"target": map[string]any{"deviceId": "d"}, "amountMinor": "1", "currency": "TRY"},
		"empty required text": with("currency", ""),
	} {
		if _, err := client.PhysicalPos.CreatePayment(ctx, request, testPosKey); !errors.Is(err, ErrConfiguration) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	if _, err := client.PhysicalPos.CreatePayment(ctx, nil, testPosKey); !errors.Is(err, ErrConfiguration) {
		t.Error("nil request accepted")
	}
	if _, err := client.PhysicalPos.GetPayment(ctx, "../x"); !errors.Is(err, ErrConfiguration) {
		t.Error("path traversal accepted")
	}
	if len(requests()) != 0 {
		t.Fatal("a refused request was sent")
	}
}

func TestPhysicalPosCardDataIsRejectedLocally(t *testing.T) {
	client, requests := newPosClient(t, 0, nil)
	ctx := context.Background()
	cases := map[string]struct {
		request map[string]any
		field   string
	}{
		"pan key":         {map[string]any{"operation": "SALE", "pan": "x"}, "pan"},
		"nested cvv":      {map[string]any{"operation": "SALE", "items": []any{map[string]any{"name": "a"}, map[string]any{"cvv": "1"}}}, "items[1].cvv"},
		"reference value": {map[string]any{"operation": "SALE", "target": map[string]any{"deviceId": "d"}, "amountMinor": "1", "currency": "TRY", "merchantReference": "4111 1111 1111 1111"}, "merchantReference"},
		"item name value": {map[string]any{"operation": "SALE", "target": map[string]any{"deviceId": "d"}, "amountMinor": "1", "currency": "TRY", "items": []any{map[string]any{"lineNo": 1, "name": "4111111111111111", "quantityMilli": 1000, "unitCode": "C62", "unitPriceMinor": "1", "taxGroupCode": "K", "lineTotalMinor": "1"}}}, "items[0].name"},
	}
	for name, c := range cases {
		_, err := client.PhysicalPos.CreatePayment(ctx, c.request, testPosKey)
		var rejected *PhysicalPosCardDataRejectedError
		if !errors.As(err, &rejected) || rejected.Field != c.field || rejected.StatusCode != 0 || !errors.Is(err, ErrConfiguration) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "4111") {
			t.Errorf("%s: card number in the error", name)
		}
	}
	if _, err := client.PhysicalPos.ListPayments(ctx, map[string]any{"merchantReference": "4111111111111111"}); !errors.As(err, new(*PhysicalPosCardDataRejectedError)) {
		t.Error("card number filter accepted")
	}
	if _, err := client.PhysicalPosManagement.ListStores(ctx, map[string]any{"q": "kart 4111-1111-1111-1111"}); !errors.As(err, new(*PhysicalPosCardDataRejectedError)) {
		t.Error("card number q accepted")
	}
	if len(requests()) != 0 {
		t.Fatal("card data was sent")
	}
}

func TestPhysicalPosAmbiguousOutcomeKeepsKeyAndBody(t *testing.T) {
	client, requests := newPosClient(t, 2, func(w http.ResponseWriter, _ *http.Request, call int) {
		switch call {
		case 0:
			posCutBody(w)
		case 1:
			posJSON(w, 502, `{"success":false,"code":"BAD_GATEWAY"}`)
		default:
			posJSON(w, 409, `{"success":false,"code":"physical_pos.idempotency_mismatch"}`)
		}
	})
	_, err := client.PhysicalPos.CreatePayment(context.Background(), posSale(), testPosKey)
	var unknown *PhysicalPosOutcomeUnknownError
	if !errors.As(err, &unknown) || unknown.IdempotencyKey != testPosKey || unknown.Attempts != 3 || unknown.StatusCode != 409 {
		t.Fatalf("%v", err)
	}
	sent := requests()
	if len(sent) != 3 {
		t.Fatalf("attempts %d", len(sent))
	}
	for _, request := range sent {
		if request.Key != testPosKey || string(request.Body) != string(sent[0].Body) {
			t.Fatal("a retry changed the key or the body")
		}
	}
}

func TestPhysicalPosTwoHundredWithoutPaymentIsUnknown(t *testing.T) {
	client, requests := newPosClient(t, 1, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 202, `{"success":true,"data":{}}`)
	})
	_, err := client.PhysicalPos.CreatePayment(context.Background(), posSale(), testPosKey)
	if !errors.As(err, new(*PhysicalPosOutcomeUnknownError)) || len(requests()) != 2 {
		t.Fatalf("%v after %d attempts", err, len(requests()))
	}
}

func TestPhysicalPosNotStartedIsRetriedThenDefinite(t *testing.T) {
	client, requests := newPosClient(t, 1, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 503, `{"success":false,"code":"physical_pos.service.busy"}`)
	})
	_, err := client.PhysicalPos.CreatePayment(context.Background(), posSale(), testPosKey)
	var apiErr *APIError
	var write *PhysicalPosWriteError
	if errors.As(err, new(*PhysicalPosOutcomeUnknownError)) || !errors.As(err, &apiErr) || apiErr.Code != "physical_pos.service.busy" || !errors.As(err, &write) || write.IdempotencyKey != testPosKey {
		t.Fatalf("%v", err)
	}
	if len(requests()) != 2 {
		t.Fatalf("attempts %d", len(requests()))
	}
}

func TestPhysicalPosClientErrorStopsAtOnce(t *testing.T) {
	client, requests := newPosClient(t, 3, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 429, `{"success":false,"code":"RATE_LIMITED","details":{"retryAfterSeconds":7}}`)
	})
	_, err := client.PhysicalPos.CreatePayment(context.Background(), posSale(), testPosKey)
	var limited *PhysicalPosRateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter != 7*time.Second || limited.IdempotencyKey != testPosKey || len(requests()) != 1 {
		t.Fatalf("%v after %d attempts", err, len(requests()))
	}
}

func TestPhysicalPosTypedGatewayErrors(t *testing.T) {
	client, _ := newPosClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.HasSuffix(r.URL.Path, "/reversals") {
			posJSON(w, 409, `{"success":false,"code":"physical_pos.payment.void_after_settlement","details":{"allowedOperation":"REFUND"}}`)
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Retry-After", "3")
			posJSON(w, 429, `{"success":false,"code":"RATE_LIMITED"}`)
			return
		}
		posJSON(w, 400, `{"success":false,"code":"physical_pos.payment.card_data_rejected","details":{"field":"items[0].name"}}`)
	})
	ctx := context.Background()
	_, err := client.PhysicalPos.CreateReversal(ctx, "pp-1", map[string]any{"operation": "VOID", "currency": "TRY"}, testPosKey)
	var void *PhysicalPosVoidAfterSettlementError
	if !errors.As(err, &void) || void.AllowedOperation != "REFUND" || void.StatusCode != 409 || !errors.As(err, new(*APIError)) {
		t.Fatalf("void: %v", err)
	}
	_, err = client.PhysicalPos.CreatePayment(ctx, posSale(), testPosKey)
	var rejected *PhysicalPosCardDataRejectedError
	if !errors.As(err, &rejected) || rejected.Field != "items[0].name" || rejected.StatusCode != 400 || errors.Is(err, ErrConfiguration) {
		t.Fatalf("card data: %v", err)
	}
	_, err = client.PhysicalPos.GetPayment(ctx, "pp-1")
	var limited *PhysicalPosRateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter != 3*time.Second || limited.IdempotencyKey != "" {
		t.Fatalf("rate limit: %v", err)
	}
}

func TestPhysicalPosListsAndFilters(t *testing.T) {
	client, requests := newPosClient(t, 0, func(w http.ResponseWriter, r *http.Request, call int) {
		switch r.URL.Path {
		case "/api/v1/physical-pos/payments":
			if call == 0 {
				posJSON(w, 200, `{"success":true,"data":{"items":[{"paymentId":"a","amountMinor":"1"}]},"pageInfo":{"hasMore":true,"nextCursor":"c2"}}`)
				return
			}
			posJSON(w, 200, `{"success":true,"data":{"items":[{"paymentId":"b","amountMinor":"2"}]},"pageInfo":{"hasMore":false,"nextCursor":null}}`)
		case "/api/v1/physical-pos/reports/sales-summary":
			posJSON(w, 200, `{"success":true,"data":{"rows":[{"netMinor":"-500","saleMinor":"0"}]}}`)
		default:
			posJSON(w, 200, `{"success":true,"data":{"items":[]},"pageInfo":{"hasMore":false,"totalCount":4}}`)
		}
	})
	ctx := context.Background()
	var ids []string
	for item, err := range client.PhysicalPos.IteratePayments(ctx, map[string]any{"limit": 1, "paymentStatus": PhysicalPosStatusApproved, "createdFrom": time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}) {
		if err != nil {
			t.Fatal(err)
		}
		var payment struct{ PaymentID string }
		_ = json.Unmarshal(item, &payment)
		ids = append(ids, payment.PaymentID)
	}
	if !slices.Equal(ids, []string{"a", "b"}) {
		t.Fatalf("iterated %v", ids)
	}
	if _, err := client.PhysicalPos.GetSalesSummary(ctx, map[string]any{"dateFrom": time.Date(2026, 11, 1, 23, 0, 0, 0, time.UTC), "groupBy": "DAY"}); err != nil {
		t.Fatal(err)
	}
	runs, err := client.PhysicalPos.ListSettlementRuns(ctx, map[string]any{"includeTotal": true, "runStatus": "", "deviceId": nil})
	if err != nil || runs.TotalCount == nil || *runs.TotalCount != 4 || runs.HasMore {
		t.Fatalf("%+v %v", runs, err)
	}
	if _, err = client.PhysicalPos.ListTransactions(ctx, map[string]any{"amountFromMinor": "100", "origin": "API"}); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	wantURIs := []string{
		"/api/v1/physical-pos/payments?createdFrom=2026-11-01T12%3A00%3A00.000Z&limit=1&paymentStatus=APPROVED",
		"/api/v1/physical-pos/payments?createdFrom=2026-11-01T12%3A00%3A00.000Z&cursor=c2&limit=1&paymentStatus=APPROVED",
		"/api/v1/physical-pos/reports/sales-summary?dateFrom=2026-11-01&groupBy=DAY",
		"/api/v1/physical-pos/settlement-runs?includeTotal=true",
		"/api/v1/physical-pos/transactions?amountFromMinor=100&origin=API",
	}
	for i, want := range wantURIs {
		if sent[i].URI != want {
			t.Errorf("request %d: %s, want %s", i, sent[i].URI, want)
		}
	}
	for name, filters := range map[string]map[string]any{
		"unknown":        {"status": "APPROVED"},
		"limit zero":     {"limit": 0},
		"limit 101":      {"limit": 101},
		"limit text":     {"limit": "10"},
		"includeTotal":   {"includeTotal": "true"},
		"numeric amount": {"amountFromMinor": 100},
	} {
		route := client.PhysicalPos.ListPayments
		if name == "numeric amount" {
			route = client.PhysicalPos.ListTransactions
		}
		if name == "includeTotal" {
			route = client.PhysicalPos.ListSettlementRuns
		}
		if _, err := route(ctx, filters); !errors.Is(err, ErrConfiguration) {
			t.Errorf("%s accepted", name)
		}
	}
	if len(requests()) != len(wantURIs) {
		t.Fatal("a refused filter was sent")
	}
}

func TestPhysicalPosResponseMoneyIsChecked(t *testing.T) {
	client, _ := newPosClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 200, `{"success":true,"data":{"payment":{"paymentId":"pp-1","amountMinor":4500}}}`)
	})
	if _, err := client.PhysicalPos.GetPayment(context.Background(), "pp-1"); !errors.Is(err, ErrConfiguration) {
		t.Fatalf("numeric amount accepted: %v", err)
	}
}

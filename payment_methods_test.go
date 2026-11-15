package lunixi

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

const testKey = "attempt-0000000001"

type recordedRequest struct {
	Method         string
	URI            string
	Accept         string
	IdempotencyKey string
	Body           []byte
	Signed         bool
}

// newRecordingClient records every merchant API request, verifies its
// Ed25519 signature, bearer and digest, and answers with reply (an empty
// success envelope when reply is nil).
func newRecordingClient(t *testing.T, reply http.HandlerFunc) (*Client, func() []recordedRequest) {
	t.Helper()
	seed, _ := base64.StdEncoding.DecodeString(fixture(t).PrivateSeed)
	public := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	var mu sync.Mutex
	var requests []recordedRequest
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		canonical := r.Method + "\n" + r.URL.RequestURI() + "\nX-Date:" + r.Header.Get("X-Date") + "\nX-Nonce:" + r.Header.Get("X-Nonce")
		digestOK := true
		if len(raw) > 0 {
			digestOK = r.Header.Get("Digest") == Digest(raw)
			canonical += "\nDigest:" + r.Header.Get("Digest")
		}
		signature, e := base64.StdEncoding.DecodeString(r.Header.Get("X-Signature"))
		signed := e == nil && digestOK && ed25519.Verify(public, []byte(canonical), signature) &&
			r.Header.Get("X-Key-Id") == "mk_test_sk_example" && r.Header.Get("Authorization") == "Bearer at_TEST_key"
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, URI: r.URL.RequestURI(), Accept: r.Header.Get("Accept"), IdempotencyKey: r.Header.Get("Idempotency-Key"), Body: raw, Signed: signed})
		mu.Unlock()
		if reply != nil {
			reply(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{}}`)
	}, nil)
	return client, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(requests)
	}
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if e := json.Unmarshal(raw, &body); e != nil {
		t.Fatalf("body is not a JSON object: %v", e)
	}
	return body
}

func mustReject(t *testing.T, name string, e error) {
	t.Helper()
	if !errors.Is(e, ErrConfiguration) {
		t.Fatalf("%s: expected ErrConfiguration, got %v", name, e)
	}
}

func TestPaymentMethodConstantsMatchReference(t *testing.T) {
	methods := []string{PaymentMethodCard, PaymentMethodBNPL, PaymentMethodShoppingLoan, PaymentMethodPromissoryNote, PaymentMethodMealCard, PaymentMethodGiftCard, PaymentMethodCash, PaymentMethodCashOnDelivery, PaymentMethodWallet, PaymentMethodBankTransfer, PaymentMethodOpenBanking, PaymentMethodMobileBilling, PaymentMethodQRCode, PaymentMethodCrypto, PaymentMethodOther}
	want := []string{"CARD", "BNPL", "SHOPPING_LOAN", "PROMISSORY_NOTE", "MEAL_CARD", "GIFT_CARD", "CASH", "CASH_ON_DELIVERY", "WALLET", "BANK_TRANSFER", "OPEN_BANKING", "MOBILE_BILLING", "QR_CODE", "CRYPTO", "OTHER"}
	if !slices.Equal(methods, want) {
		t.Fatalf("payment methods %v", methods)
	}
	if got := []string{PaymentActionTypeOTP, PaymentActionTypeRedirect, PaymentActionTypeDisplay, PaymentActionTypeAppApproval, PaymentActionTypeInput}; !slices.Equal(got, []string{"OTP", "REDIRECT", "DISPLAY", "APP_APPROVAL", "INPUT"}) {
		t.Fatalf("action types %v", got)
	}
	if ProviderSelectionRouted != "ROUTED" || ProviderSelectionOptional != "OPTIONAL" || ProviderSelectionRequired != "REQUIRED" || ProviderSelectionModeBuyer != "BUYER" || ProviderSelectionModePlatform != "PLATFORM" {
		t.Fatal("provider selection constants")
	}
	blocks := MethodBlockFields()
	if !slices.Equal(blocks, []string{"mobileBilling", "wallet", "promissoryNote", "mealCard", "openBanking", "shoppingLoan", "bankTransfer"}) {
		t.Fatalf("method blocks %v", blocks)
	}
	blocks[0] = "changed"
	if MethodBlockFields()[0] != "mobileBilling" {
		t.Fatal("MethodBlockFields exposed shared state")
	}
	fields := DirectMethodPaymentFields()
	if len(fields) != 27 || !slices.Contains(fields, "bankTransfer") || !slices.Contains(fields, "card") || slices.Contains(fields, "price") {
		t.Fatalf("direct fields %v", fields)
	}
	if !slices.Equal(CardOnlyFields(), []string{"card", "installment", "installmentQuoteToken", "force3D"}) {
		t.Fatal("card only fields")
	}
	if !slices.Equal(PaymentActionConfirmFields(), []string{"otp", "consentAccepted", "fields"}) {
		t.Fatal("confirm fields")
	}
	if !slices.Equal(SplitLegBlockFields(), []string{"mobileBilling", "wallet", "promissoryNote"}) {
		t.Fatal("split leg blocks")
	}
	if !slices.Equal(ListSplitPaymentsFilters(), []string{"status", "currency", "createdFrom", "createdTo", "orderId", "ownerRef", "limit", "after"}) {
		t.Fatal("split list filters")
	}
	if !slices.Contains(SplitPaymentLegFields(), "promissoryNote") || slices.Contains(SplitPaymentLegFields(), "bankTransfer") {
		t.Fatal("split leg fields")
	}
	if ShoppingLoanMaxBasketItems != 100 || BankTransferOtherBanksCode != "9999" || BankTransferExceptionLimitMax != 200 || PromissoryNoteMaxScheduleItems != 24 {
		t.Fatal("limits")
	}
	if got := []string{BankTransferExceptionKindLate, BankTransferExceptionKindUnderpaid, BankTransferExceptionKindOverpaid}; !slices.Equal(got, []string{"LATE", "UNDERPAID", "OVERPAID"}) {
		t.Fatal("exception kinds")
	}
	if got := []string{BankTransferExceptionStatusPendingDecision, BankTransferExceptionStatusAccepted, BankTransferExceptionStatusRefundPending, BankTransferExceptionStatusRefunded}; !slices.Equal(got, []string{"PENDING_DECISION", "ACCEPTED", "REFUND_PENDING", "REFUNDED"}) {
		t.Fatal("exception statuses")
	}
	events := []string{
		PaymentBankTransferEventPartiallyReceived, PaymentBankTransferEventUnderpaid, PaymentBankTransferEventOverpaid, PaymentBankTransferEventLateArrival,
		PromissoryNoteEventPaid, PromissoryNoteEventProtested, PromissoryNoteEventCancelled,
		SplitPaymentEventCompleted, SplitPaymentEventVoided, SplitPaymentEventCompensating, SplitPaymentEventOutcomePending, SplitPaymentEventReconciliationRequired, SplitPaymentEventLegRefundPending,
		SplitLegEventsSubscription,
	}
	wantEvents := []string{
		"payment.bank_transfer.partially_received", "payment.bank_transfer.underpaid", "payment.bank_transfer.overpaid", "payment.bank_transfer.late_arrival",
		"payment.promissory_note.paid", "payment.promissory_note.protested", "payment.promissory_note.cancelled",
		"payment.split.completed", "payment.split.voided", "payment.split.compensating", "payment.split.outcome_pending", "payment.split.reconciliation_required", "payment.split.leg_refund_pending",
		"payment.split.leg_events",
	}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events %v", events)
	}
	if got := []string{PromissoryNoteStatusIssued, PromissoryNoteStatusPaid, PromissoryNoteStatusProtested, PromissoryNoteStatusCancelled, PromissoryNoteDocumentTypePromissoryNote, PromissoryNoteDocumentTypePaymentOrder}; !slices.Equal(got, []string{"ISSUED", "PAID", "PROTESTED", "CANCELLED", "PROMISSORY_NOTE", "PAYMENT_ORDER"}) {
		t.Fatal("promissory note statuses")
	}
	if got := []string{PromissoryNoteCancelResultCancelled, PromissoryNoteCancelResultAlreadyClosed, PromissoryNoteCancelResultNotCancellable, PromissoryNoteCancelResultFailed}; !slices.Equal(got, []string{"CANCELLED", "ALREADY_CLOSED", "NOT_CANCELLABLE", "FAILED"}) {
		t.Fatal("cancel results")
	}
	if PromissoryNoteCancelErrorNotCancellable != "payment.promissory_note.not_cancellable" || PromissoryNoteCancelErrorProviderUnavailable != "payment.promissory_note.provider_unavailable" || PromissoryNoteCancelErrorCancelRejected != "payment.promissory_note.cancel_rejected" {
		t.Fatal("cancel error codes")
	}
	if PromissoryNoteDocumentLocaleTR != "tr-TR" || PromissoryNoteDocumentLocaleEN != "en-GB" {
		t.Fatal("document locales")
	}
}

func TestBlockNameAndMethodNormalization(t *testing.T) {
	for method, block := range map[string]string{"BANK_TRANSFER": "bankTransfer", "MOBILE_BILLING": "mobileBilling", "CARD": "card", "QR": "qrCode", "PROMISSORY_NOTE": "promissoryNote"} {
		if got, e := BlockNameOf(method); e != nil || got != block {
			t.Fatalf("%s: %q %v", method, got, e)
		}
	}
	if code, e := NormalizePaymentMethod("QR"); e != nil || code != "QR_CODE" {
		t.Fatal("QR alias")
	}
	for _, bad := range []string{"", "mobile_billing", "1CARD", strings.Repeat("A", 33)} {
		_, e := NormalizePaymentMethod(bad)
		mustReject(t, bad, e)
	}
}

func TestListPaymentMethodsFilterIsSignedAndNormalized(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.ListPaymentMethods(ctx, "QR"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ListPaymentMethods(ctx, ""); e != nil {
		t.Fatal(e)
	}
	_, e := client.Payments.ListPaymentMethods(ctx, "mobile_billing")
	mustReject(t, "lowercase method", e)
	sent := requests()
	if len(sent) != 2 || sent[0].URI != "/api/v1/payments/methods?paymentMethod=QR_CODE" || sent[1].URI != "/api/v1/payments/methods" {
		t.Fatalf("unexpected requests %+v", sent)
	}
	for _, r := range sent {
		if !r.Signed || r.Method != "GET" {
			t.Fatalf("request not signed: %+v", r)
		}
	}
}

const bankTransferAnswer = `{"success":true,"data":{"paymentId":"pay_1","orderId":"ORDER-2041","status":"PENDING","paymentMethod":"BANK_TRANSFER","action":{"id":"act_1","type":"DISPLAY","bankTransfer":{"bankCode":"0062","bankName":"Example Bank","holderName":"EXAMPLE TRADING LTD","iban":"TR000006200000000000000001","amountMinor":"125000","currency":"TRY","paymentCode":"LNXK7Q3P9","expiresAt":"2026-11-18T20:59:59.999Z","otherBank":false}}}}`

func TestBankTransferDirectPaymentBodyAndInstruction(t *testing.T) {
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, bankTransferAnswer)
	})
	request := NewDirectMethodPaymentRequest("125000", "TRY", "ORDER-2041").WithMethodFields(PaymentMethodBankTransfer, "", map[string]any{"bankCode": "0062"})
	body, e := request.Body()
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]any{"paymentMethod": "BANK_TRANSFER", "amount": "125000", "currency": "TRY", "orderId": "ORDER-2041", "bankTransfer": map[string]any{"bankCode": "0062"}}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("body %v", body)
	}
	response, e := client.Payments.DirectMethodPayment(context.Background(), request, testKey)
	if e != nil {
		t.Fatal(e)
	}
	sent := requests()
	if len(sent) != 1 || sent[0].URI != "/api/v1/payments/direct/payment" || sent[0].IdempotencyKey != testKey || !sent[0].Signed {
		t.Fatalf("unexpected request %+v", sent)
	}
	if got := decodeBody(t, sent[0].Body); !reflect.DeepEqual(got, want) {
		t.Fatalf("sent %v", got)
	}
	instruction := response.BankTransferInstruction()
	if instruction == nil || instruction.IBAN != "TR000006200000000000000001" || instruction.AmountMinor != "125000" || instruction.PaymentCode != "LNXK7Q3P9" || instruction.OtherBank {
		t.Fatalf("instruction %+v", instruction)
	}

	other, e := NewDirectMethodPaymentRequest("125000", "TRY", "ORDER-2042").WithMethodFields(PaymentMethodBankTransfer, "", map[string]any{"bankCode": BankTransferOtherBanksCode}).Body()
	if e != nil || other["bankTransfer"].(map[string]any)["bankCode"] != "9999" {
		t.Fatalf("other banks body %v %v", other, e)
	}
}

func TestBankTransferInstructionOnlyForDisplay(t *testing.T) {
	for _, data := range []string{`{"paymentId":"pay_1"}`, `{"action":{"type":"OTP","bankTransfer":{"iban":"TR00"}}}`, `{"action":{"type":"DISPLAY","display":{"qrCode":"x"}}}`, `[]`} {
		if (&Response{Data: json.RawMessage(data)}).BankTransferInstruction() != nil {
			t.Fatalf("instruction read from %s", data)
		}
	}
	var missing *Response
	if missing.BankTransferInstruction() != nil {
		t.Fatal("nil response")
	}
}

func TestBankTransferRejectsProviderCode(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	_, e := NewDirectMethodPaymentRequest("125000", "TRY", "ORDER-2041").WithMethodFields(PaymentMethodBankTransfer, "bank_provider", map[string]any{"bankCode": "0062"}).Body()
	mustReject(t, "builder providerCode", e)
	_, e = NewDirectMethodPaymentRequest("125000", "TRY", "ORDER-2041").WithMethodFields(PaymentMethodBankTransfer, "", map[string]any{"bankCode": "0062", "providerCode": "bank_provider"}).Body()
	mustReject(t, "providerCode in fields", e)
	for _, block := range []map[string]any{{"bankCode": "0062", "providerCode": "bank_provider"}, {"bankCode": "0062", "providerCode": nil}} {
		body := map[string]any{"paymentMethod": "BANK_TRANSFER", "amount": "125000", "currency": "TRY", "orderId": "ORDER-2041", "bankTransfer": block}
		_, e = client.Do(context.Background(), Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: body, IdempotencyKey: testKey})
		mustReject(t, "raw providerCode", e)
	}
	if len(requests()) != 0 {
		t.Fatal("rejected bank transfer reached network")
	}
}

func TestDirectMethodPaymentRejectsInvalidBodiesBeforeNetwork(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	base := func(extra map[string]any) map[string]any {
		body := map[string]any{"paymentMethod": "MOBILE_BILLING", "amount": "1000", "currency": "TRY", "orderId": "ORDER-1"}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	gsm := map[string]any{"providerCode": "mobile_provider", "gsmNumber": "+905321234567"}
	cases := map[string]map[string]any{
		"another method's block": base(map[string]any{"mobileBilling": gsm, "wallet": map[string]any{}}),
		"card field":             base(map[string]any{"mobileBilling": gsm, "force3D": true}),
		"card block":             base(map[string]any{"mobileBilling": gsm, "card": map[string]any{"cardToken": "x"}}),
		"unknown field":          base(map[string]any{"mobileBilling": gsm, "surprise": "x"}),
		"missing block":          base(nil),
		"price":                  base(map[string]any{"mobileBilling": gsm, "price": "1000"}),
		"paidPrice":              base(map[string]any{"mobileBilling": gsm, "paidPrice": "1000"}),
		"string preauth":         base(map[string]any{"mobileBilling": gsm, "preauth": "false"}),
		"method without block":   {"paymentMethod": "GIFT_CARD", "amount": "1000", "currency": "TRY", "orderId": "ORDER-1"},
		"lowercase method":       {"paymentMethod": "mobile_billing", "amount": "1000", "currency": "TRY", "orderId": "ORDER-1", "mobileBilling": gsm},
		"missing orderId":        {"paymentMethod": "MOBILE_BILLING", "amount": "1000", "currency": "TRY", "mobileBilling": gsm},
		"bad providerCode":       base(map[string]any{"mobileBilling": map[string]any{"providerCode": "bad code"}}),
		"card without card":      {"paymentMethod": "CARD", "amount": "1000", "currency": "TRY", "orderId": "ORDER-1"},
	}
	for name, body := range cases {
		_, e := client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: body, IdempotencyKey: testKey})
		mustReject(t, name, e)
	}
	valid := base(map[string]any{"mobileBilling": gsm})
	_, e := client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: valid})
	mustReject(t, "raw body without Idempotency-Key", e)
	request := NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodMobileBilling, "mobile_provider", map[string]any{"gsmNumber": "+905321234567"})
	_, e = client.Payments.DirectMethodPayment(ctx, request, "")
	mustReject(t, "typed without Idempotency-Key", e)

	builders := map[string]*DirectMethodPaymentRequest{
		"card through method fields": NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodCard, "", nil),
		"method without block":       NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodGiftCard, "", nil),
		"force3D without card":       NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodMobileBilling, "", nil).WithForce3D(true),
		"installment without card":   NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodWallet, "", nil).WithInstallment(3, "quote"),
		"no method":                  NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1"),
		"decimal amount":             NewDirectMethodPaymentRequest("10.00", "TRY", "ORDER-1").WithMethodFields(PaymentMethodWallet, "", nil),
		"plan without note method":   NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodWallet, "", nil).WithPromissoryNoteCount(2),
		"empty card":                 NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithCard(nil),
	}
	for name, builder := range builders {
		_, e = client.Payments.DirectMethodPayment(ctx, builder, testKey)
		mustReject(t, name, e)
	}
	if len(requests()) != 0 {
		t.Fatal("invalid direct method payment reached network")
	}
}

func TestDirectCardPaymentKeepsCardFields(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	request := NewDirectMethodPaymentRequest("1000", "try", "ORDER-1").
		WithCard(map[string]any{"cardUserKey": "cuk_1", "cardToken": "ct_1"}).
		WithForce3D(true).
		WithInstallment(3, "iq_1").
		WithCallbackURL("https://merchant.example/return")
	if _, e := client.Payments.DirectMethodPayment(context.Background(), request, testKey); e != nil {
		t.Fatal(e)
	}
	body := decodeBody(t, requests()[0].Body)
	if body["paymentMethod"] != "CARD" || body["currency"] != "TRY" || body["force3D"] != true || body["installment"] != float64(3) || body["installmentQuoteToken"] != "iq_1" {
		t.Fatalf("card body %v", body)
	}
	if card, ok := body["card"].(map[string]any); !ok || card["cardToken"] != "ct_1" {
		t.Fatalf("card block %v", body["card"])
	}
}

func TestShoppingLoanBasketBounds(t *testing.T) {
	items := func(n int) []BasketItem {
		list := make([]BasketItem, n)
		for i := range list {
			list[i] = BasketItem{ID: fmt.Sprintf("sku-%d", i), Name: "Example", ItemType: "PHYSICAL", Price: "10", ProductCategory: ProductCategoryComputer}
		}
		return list
	}
	loan := func(n int) *DirectMethodPaymentRequest {
		return NewDirectMethodPaymentRequest(fmt.Sprint(10*max(n, 1)), "TRY", "ORDER-1").WithMethodFields(PaymentMethodShoppingLoan, "", map[string]any{"bankCode": "0062"}).WithBasketItems(items(n))
	}
	for _, n := range []int{0, 101} {
		_, e := loan(n).Body()
		mustReject(t, fmt.Sprintf("%d items", n), e)
	}
	for _, n := range []int{1, 100} {
		if _, e := loan(n).Body(); e != nil {
			t.Fatalf("%d items: %v", n, e)
		}
	}
	client, requests := newRecordingClient(t, nil)
	raw := map[string]any{"paymentMethod": "SHOPPING_LOAN", "amount": "1010", "currency": "TRY", "orderId": "ORDER-1", "shoppingLoan": map[string]any{}, "basketItems": items(101)}
	_, e := client.Do(context.Background(), Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: raw, IdempotencyKey: testKey})
	mustReject(t, "raw 101 items", e)
	if len(requests()) != 0 {
		t.Fatal("oversized basket reached network")
	}
}

func TestMethodBlockStructure(t *testing.T) {
	many := func(n int) map[string]any {
		fields := map[string]any{}
		for i := 0; i < n; i++ {
			fields[fmt.Sprintf("field%d", i)] = "x"
		}
		return fields
	}
	group := func(n int) []any {
		list := make([]any, n)
		for i := range list {
			list[i] = map[string]any{"name": "x", "address": map[string]any{"provinceCode": "06"}}
		}
		return list
	}
	rejected := map[string]map[string]any{
		"33 keys":           many(33),
		"1025 characters":   {"note": strings.Repeat("a", 1025)},
		"11 group items":    {"guarantors": group(11)},
		"__proto__":         {"__proto__": "x"},
		"constructor":       {"constructor": "x"},
		"prototype":         {"prototype": "x"},
		"underscore name":   {"gsm_number": "x"},
		"17 address keys":   {"address": many(17)},
		"list in address":   {"address": map[string]any{"lines": []any{"a"}}},
		"list in group":     {"guarantors": []any{map[string]any{"phones": []any{"1"}}}},
		"scalar group item": {"guarantors": []any{"x"}},
		"null value":        {"gsmNumber": nil},
		"providerCode":      {"providerCode": "mobile_provider"},
	}
	for name, fields := range rejected {
		_, e := NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodMobileBilling, "", fields).Body()
		mustReject(t, name, e)
	}
	accepted := many(32)
	accepted["field0"] = strings.Repeat("ş", 1024)
	accepted["field1"] = map[string]any{"provinceCode": "06", "districtCode": "1234", "neighborhoodCode": "5", "line": "Example 1"}
	accepted["field2"] = group(10)
	accepted["field3"] = 12.5
	accepted["field4"] = true
	if _, e := NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodMobileBilling, "mobile_provider", accepted).Body(); e != nil {
		t.Fatal(e)
	}
	client, requests := newRecordingClient(t, nil)
	block := many(33)
	block["providerCode"] = "mobile_provider"
	raw := map[string]any{"paymentMethod": "MOBILE_BILLING", "amount": "1000", "currency": "TRY", "orderId": "ORDER-1", "mobileBilling": block}
	_, e := client.Do(context.Background(), Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: raw, IdempotencyKey: testKey})
	mustReject(t, "raw 33 keys", e)
	if len(requests()) != 0 {
		t.Fatal("invalid block reached network")
	}
}

func TestConfirmAndResendPaymentAction(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	accepted := true
	if _, e := client.Payments.ConfirmPaymentAction(ctx, "pay_1", "act_1", PaymentActionConfirmRequest{OTP: "123456", ConsentAccepted: &accepted}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ConfirmPaymentAction(ctx, "pay_1", "act_2", PaymentActionConfirmRequest{Fields: map[string]any{"birthDate": "1990-01-31"}}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ResendPaymentAction(ctx, "pay_1", "act_1"); e != nil {
		t.Fatal(e)
	}
	sent := requests()
	if len(sent) != 3 {
		t.Fatalf("requests %+v", sent)
	}
	if sent[0].URI != "/api/v1/payments/pay_1/actions/act_1/confirm" || !reflect.DeepEqual(decodeBody(t, sent[0].Body), map[string]any{"otp": "123456", "consentAccepted": true}) {
		t.Fatalf("confirm %s %s", sent[0].URI, sent[0].Body)
	}
	if !reflect.DeepEqual(decodeBody(t, sent[1].Body), map[string]any{"fields": map[string]any{"birthDate": "1990-01-31"}}) {
		t.Fatalf("confirm fields %s", sent[1].Body)
	}
	if sent[2].URI != "/api/v1/payments/pay_1/actions/act_1/resend" || len(sent[2].Body) != 0 || sent[2].Method != "POST" {
		t.Fatalf("resend %+v", sent[2])
	}
	for _, r := range sent {
		if !r.Signed {
			t.Fatalf("not signed: %+v", r)
		}
	}

	_, e := client.Payments.ConfirmPaymentAction(ctx, "pay_1", "act_1", PaymentActionConfirmRequest{OTP: "12a4"})
	mustReject(t, "letters in otp", e)
	_, e = client.Payments.ConfirmPaymentAction(ctx, "pay_1", "act_1", PaymentActionConfirmRequest{OTP: strings.Repeat("1", 17)})
	mustReject(t, "17 digit otp", e)
	_, e = client.Payments.ConfirmPaymentAction(ctx, "pay_1", "act_1", PaymentActionConfirmRequest{})
	mustReject(t, "empty confirm", e)
	_, e = client.Payments.ConfirmPaymentAction(ctx, "pay_1", "act_1", PaymentActionConfirmRequest{Fields: map[string]any{"providerCode": "x"}})
	mustReject(t, "providerCode in fields", e)
	_, e = client.Payments.ConfirmPaymentAction(ctx, "", "act_1", PaymentActionConfirmRequest{OTP: "1234"})
	mustReject(t, "empty paymentId", e)
	_, e = client.Payments.ResendPaymentAction(ctx, "pay_1", "")
	mustReject(t, "empty actionId", e)
	for name, body := range map[string]map[string]any{
		"token":                {"otp": "1234", "token": "ct_example"},
		"checkoutSessionProof": {"otp": "1234", "checkoutSessionProof": "proof"},
		"unknown field":        {"pin": "1234"},
		"string consent":       {"consentAccepted": "true"},
	} {
		_, e = client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/pay_1/actions/act_1/confirm", Body: body})
		mustReject(t, name, e)
	}
	if len(requests()) != 3 {
		t.Fatal("invalid confirm reached network")
	}
}

package lunixi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testLinkKey  = "plk-0001"
	testAssetID  = "0b6f3f5e-4c1a-4e2b-9a57-1f2d3c4b5a69"
	testImageURL = "https://img.lunixi.example/" + testAssetID
)

func fixedLink() PaymentLinkRequest {
	return PaymentLinkRequest{Usage: PaymentLinkUsageSingleUse, AmountMode: PaymentLinkAmountModeFixed, Currency: "TRY", AmountMinor: "150000", Title: "Invoice 42"}
}

func TestPaymentLinkValuesMatchReference(t *testing.T) {
	if !slices.Equal(PaymentLinkImmutableFields(), []string{"usage", "amountMode", "currency"}) {
		t.Fatal("immutable fields")
	}
	fields := PaymentLinkSpecFields()
	if len(fields) != 39 || fields[0] != "usage" || fields[len(fields)-1] != "metadata" || !slices.Contains(fields, "checkout") {
		t.Fatalf("spec fields %v", fields)
	}
	fields[0] = "changed"
	if PaymentLinkSpecFields()[0] != "usage" {
		t.Fatal("PaymentLinkSpecFields exposed shared state")
	}
	if got := []string{PaymentLinkStateActive, PaymentLinkStatePaused, PaymentLinkStateCompleted, PaymentLinkStateExpired, PaymentLinkStateDeactivated, PaymentLinkStateTakenDown}; !slices.Equal(got, []string{"ACTIVE", "PAUSED", "COMPLETED", "EXPIRED", "DEACTIVATED", "TAKEN_DOWN"}) {
		t.Fatal("states")
	}
	if got := []string{CustomFieldTypeText, CustomFieldTypeNumber, CustomFieldTypeEmail, CustomFieldTypePhone, CustomFieldTypeDate, CustomFieldTypeSelect, CustomFieldTypeCheckbox, CustomFieldTypeTCKN, CustomFieldTypeVKN}; !slices.Equal(got, []string{"TEXT", "NUMBER", "EMAIL", "PHONE", "DATE", "SELECT", "CHECKBOX", "TCKN", "VKN"}) {
		t.Fatal("custom field types")
	}
	if PaymentLinkImageContentTypeWebP != "image/webp" || PaymentLinkQRFormatPNG != "png" || PaymentLinkLocaleEN != "en" || PaymentLinkAttemptStateNeedsReview != "NEEDS_REVIEW" || PaymentLinkButtonLabelBook != "BOOK" || BuyerFieldRequirementRequired != "REQUIRED" {
		t.Fatal("value sets")
	}
}

func TestPaymentLinkCreateBodyOrderAndKey(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	request := fixedLink()
	request.Environment = PaymentLinkEnvironmentTest
	request.Currency = "try"
	request.Checkout = &PaymentLinkCheckout{EnabledInstallments: []int{3, 1}}
	request.Recipient = &PaymentLinkRecipient{Email: "buyer@example.com"}
	request.Verification = &PaymentLinkVerification{Required: true}
	if _, e := client.PaymentLinks.Create(ctx, request, testLinkKey); e != nil {
		t.Fatal(e)
	}
	if request.Checkout.PaymentMethods != nil || !slices.Equal(request.Checkout.EnabledInstallments, []int{3, 1}) {
		t.Fatal("caller request was changed")
	}
	sent := requests()
	if len(sent) != 1 {
		t.Fatalf("requests %+v", sent)
	}
	want := `{"environment":"TEST","usage":"SINGLE_USE","amountMode":"FIXED","currency":"TRY","amountMinor":"150000","title":"Invoice 42",` +
		`"recipient":{"email":"buyer@example.com"},"verification":{"required":true,"channel":"EMAIL"},` +
		`"checkout":{"requireThreeDs":false,"paymentMethods":[],"enabledInstallments":[1,3]}}`
	if r := sent[0]; r.Method != "POST" || r.URI != "/api/v1/payments/links" || !r.Signed || r.IdempotencyKey != testLinkKey || string(r.Body) != want {
		t.Fatalf("create request %+v\n%s", r, r.Body)
	}
}

func TestPaymentLinkCreateRejectsBeforeNetwork(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	_, e := client.PaymentLinks.Create(ctx, fixedLink(), "")
	mustReject(t, "missing key", e)
	_, e = client.PaymentLinks.Create(ctx, fixedLink(), "seven77")
	mustReject(t, "7 character key", e)
	_, e = client.PaymentLinks.Create(ctx, fixedLink(), strings.Repeat("k", 201))
	mustReject(t, "201 character key", e)
	for name, mutate := range map[string]func(*PaymentLinkRequest){
		"usage":                func(r *PaymentLinkRequest) { r.Usage = "ONCE" },
		"amount mode":          func(r *PaymentLinkRequest) { r.AmountMode = "FREE" },
		"environment":          func(r *PaymentLinkRequest) { r.Environment = "PROD" },
		"title":                func(r *PaymentLinkRequest) { r.Title = " " },
		"decimal amount":       func(r *PaymentLinkRequest) { r.AmountMinor = "1500.00" },
		"suggested amount":     func(r *PaymentLinkRequest) { r.SuggestedAmountsMinor = []string{"100", "1.5"} },
		"image asset":          func(r *PaymentLinkRequest) { r.ImageAssetID = "asset-1" },
		"locale":               func(r *PaymentLinkRequest) { r.Locale = "de" },
		"button label":         func(r *PaymentLinkRequest) { r.ButtonLabelKey = "GIVE" },
		"buyer field":          func(r *PaymentLinkRequest) { r.BuyerFields = &PaymentLinkBuyerFields{Email: "MANDATORY"} },
		"hidden buyer field":   func(r *PaymentLinkRequest) { r.BuyerFields = &PaymentLinkBuyerFields{Name: "HIDDEN"} },
		"optional buyer field": func(r *PaymentLinkRequest) { r.BuyerFields = &PaymentLinkBuyerFields{Phone: "OPTIONAL"} },
		"custom field type": func(r *PaymentLinkRequest) {
			r.CustomFields = []PaymentLinkCustomField{{Key: "f", Label: "F", FieldType: "FILE"}}
		},
		"verification": func(r *PaymentLinkRequest) {
			r.Verification = &PaymentLinkVerification{Required: true, Channel: "PUSH"}
		},
		"item price":         func(r *PaymentLinkRequest) { r.Items = []PaymentLinkItem{{Name: "Ticket", UnitPriceMinor: "12.5"}} },
		"item name":          func(r *PaymentLinkRequest) { r.Items = []PaymentLinkItem{{UnitPriceMinor: "1250"}} },
		"checkout without 1": func(r *PaymentLinkRequest) { r.Checkout = &PaymentLinkCheckout{EnabledInstallments: []int{2, 3}} },
	} {
		bad := fixedLink()
		mutate(&bad)
		_, e := client.PaymentLinks.Create(ctx, bad, testLinkKey)
		mustReject(t, name, e)
	}
	if sent := requests(); len(sent) != 0 {
		t.Fatalf("invalid links reached the network: %+v", sent)
	}
}

func TestPaymentLinkUpdateBody(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	expires := time.Date(2026, 12, 31, 23, 0, 0, 0, time.FixedZone("TRT", 3*3600))
	changes := map[string]any{"title": "New title", "imageAssetId": nil, "expiresAt": expires, "amountMinor": "175000"}
	if _, e := client.PaymentLinks.Update(ctx, "plink_1", 3, changes, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.Update(ctx, "plink_1", 4, map[string]any{"tags": []string{"vip"}}, testLinkKey); e != nil {
		t.Fatal(e)
	}
	for name, bad := range map[string]map[string]any{
		"immutable": {"currency": "EUR"},
		"unknown":   {"price": "100"},
		"empty":     {},
		"bad time":  {"startsAt": 42},
	} {
		_, e := client.PaymentLinks.Update(ctx, "plink_1", 3, bad, "")
		mustReject(t, name, e)
	}
	_, e := client.PaymentLinks.Update(ctx, "plink_1", -1, map[string]any{"title": "x"}, "")
	mustReject(t, "negative row version", e)
	sent := requests()
	if len(sent) != 2 {
		t.Fatalf("requests %+v", sent)
	}
	want := `{"expectedRowVersion":3,"amountMinor":"175000","title":"New title","imageAssetId":null,"expiresAt":"2026-12-31T20:00:00Z"}`
	if r := sent[0]; r.Method != "PATCH" || r.URI != "/api/v1/payments/links/plink_1" || !r.Signed || r.IdempotencyKey != "" || string(r.Body) != want {
		t.Fatalf("update request %+v\n%s", r, r.Body)
	}
	if sent[1].IdempotencyKey != testLinkKey || string(sent[1].Body) != `{"expectedRowVersion":4,"tags":["vip"]}` {
		t.Fatalf("second update %+v %s", sent[1], sent[1].Body)
	}
}

func TestPaymentLinkActionsAndSend(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	version := int64(7)
	if _, e := client.PaymentLinks.Pause(ctx, "plink_1", PaymentLinkActionOptions{}, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.Resume(ctx, "plink_1", PaymentLinkActionOptions{}, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.Deactivate(ctx, "plink_1", PaymentLinkActionOptions{ExpectedRowVersion: &version, Reason: "Sold out"}, testLinkKey); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.Send(ctx, "plink_1", PaymentLinkSendRequest{Channel: PaymentLinkDeliveryChannelSMS, RecipientPhone: "+905321234567", Locale: PaymentLinkLocaleEN}, testLinkKey); e != nil {
		t.Fatal(e)
	}
	_, e := client.PaymentLinks.Action(ctx, "plink_1", "archive", PaymentLinkActionOptions{}, "")
	mustReject(t, "unknown action", e)
	_, e = client.PaymentLinks.Send(ctx, "plink_1", PaymentLinkSendRequest{Channel: PaymentLinkDeliveryChannelEmail}, "")
	mustReject(t, "send without key", e)
	_, e = client.PaymentLinks.Send(ctx, "plink_1", PaymentLinkSendRequest{Channel: "FAX"}, testLinkKey)
	mustReject(t, "send channel", e)
	_, e = client.PaymentLinks.Pause(ctx, "", PaymentLinkActionOptions{}, "")
	mustReject(t, "empty link id", e)
	sent := requests()
	want := []struct{ uri, body, key string }{
		{"/api/v1/payments/links/plink_1/pause", `{}`, ""},
		{"/api/v1/payments/links/plink_1/resume", `{}`, ""},
		{"/api/v1/payments/links/plink_1/deactivate", `{"expectedRowVersion":7,"reason":"Sold out"}`, testLinkKey},
		{"/api/v1/payments/links/plink_1/send", `{"channel":"SMS","recipientPhone":"+905321234567","locale":"en"}`, testLinkKey},
	}
	if len(sent) != len(want) {
		t.Fatalf("requests %+v", sent)
	}
	for i, r := range sent {
		if r.Method != "POST" || r.URI != want[i].uri || string(r.Body) != want[i].body || r.IdempotencyKey != want[i].key || !r.Signed {
			t.Fatalf("request %d: %+v %s", i, r, r.Body)
		}
	}
}

func TestPaymentLinkListsAndQR(t *testing.T) {
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/qr") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"items":[]},"pageInfo":{"hasMore":false}}`)
	})
	ctx := context.Background()
	filters := PaymentLinkListFilters{PaymentLinkPage: PaymentLinkPage{Limit: 20, IncludeTotal: true}, State: []string{PaymentLinkStateActive, PaymentLinkStatePaused}, AmountMode: []string{PaymentLinkAmountModeFixed}, Query: "Invoice"}
	if _, e := client.PaymentLinks.List(ctx, filters); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.Get(ctx, "plink_1"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.ListAttempts(ctx, "plink_1", PaymentLinkAttemptFilters{State: []string{PaymentLinkAttemptStateFailed}}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.PaymentLinks.ListPayments(ctx, "plink_1", PaymentLinkPage{Cursor: "c2"}); e != nil {
		t.Fatal(e)
	}
	qr, e := client.PaymentLinks.QR(ctx, "plink_1", PaymentLinkQRFormatPNG, 256)
	if e != nil || string(qr.Bytes) != "\x89PNG\r\n" || qr.ContentType != "image/png" {
		t.Fatalf("qr %+v %v", qr, e)
	}
	for name, call := range map[string]func() error{
		"list state": func() error {
			_, e := client.PaymentLinks.List(ctx, PaymentLinkListFilters{State: []string{"OPEN"}})
			return e
		},
		"list limit": func() error {
			_, e := client.PaymentLinks.List(ctx, PaymentLinkListFilters{PaymentLinkPage: PaymentLinkPage{Limit: -1}})
			return e
		},
		"attempt state": func() error {
			_, e := client.PaymentLinks.ListAttempts(ctx, "plink_1", PaymentLinkAttemptFilters{State: []string{"ACTIVE"}})
			return e
		},
		"qr size":   func() error { _, e := client.PaymentLinks.QR(ctx, "plink_1", "", 100); return e },
		"qr format": func() error { _, e := client.PaymentLinks.QR(ctx, "plink_1", "gif", 0); return e },
	} {
		mustReject(t, name, call())
	}
	sent := requests()
	want := []string{
		"/api/v1/payments/links?amountMode=FIXED&includeTotal=true&limit=20&query=Invoice&state=ACTIVE%2CPAUSED",
		"/api/v1/payments/links/plink_1",
		"/api/v1/payments/links/plink_1/attempts?state=FAILED",
		"/api/v1/payments/links/plink_1/payments?cursor=c2",
		"/api/v1/payments/links/plink_1/qr?format=png&size=256",
	}
	if len(sent) != len(want) {
		t.Fatalf("requests %+v", sent)
	}
	for i, r := range sent {
		if r.Method != "GET" || r.URI != want[i] || !r.Signed {
			t.Fatalf("request %d: %+v", i, r)
		}
	}
	if sent[4].Accept != "image/svg+xml, image/png, application/json" {
		t.Fatalf("qr accept %q", sent[4].Accept)
	}
}

type storageRecorder struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
}

func (s *storageRecorder) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r)
	s.bodies = append(s.bodies, body)
}

func (s *storageRecorder) calls() ([]*http.Request, [][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests), slices.Clone(s.bodies)
}

// newImageUploadClient serves the gateway image routes and a separate storage
// host. target edits the upload target the gateway returns; storageStatus is
// the storage answer.
func newImageUploadClient(t *testing.T, target func(map[string]any), storageStatus int, confirmState string) (*Client, func() []recordedRequest, *storageRecorder) {
	t.Helper()
	storage := &storageRecorder{}
	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storage.record(r)
		if storageStatus != http.StatusOK {
			w.WriteHeader(storageStatus)
			fmt.Fprint(w, `<?xml version="1.0"?><Error><Code>AccessDenied</Code><Message>Request has expired</Message></Error>`)
		}
	}))
	t.Cleanup(storageServer.Close)
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/payments/links/images" {
			upload := map[string]any{"assetId": testAssetID, "uploadUrl": storageServer.URL + "/bucket/" + testAssetID + "?X-Amz-Signature=presigned", "uploadMethod": "PUT", "uploadHeaders": map[string]any{"Content-Type": "image/png"}, "expiresAt": "2026-11-15T10:15:00Z"}
			if target != nil {
				target(upload)
			}
			raw, _ := CanonicalJSON(map[string]any{"success": true, "data": upload})
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(raw)
			return
		}
		fmt.Fprintf(w, `{"success":true,"data":{"assetId":%q,"url":%q,"state":%q}}`, testAssetID, testImageURL, confirmState)
	})
	return client, requests, storage
}

func TestPaymentLinkUploadImageNeverSendsCredentialsToStorage(t *testing.T) {
	client, requests, storage := newImageUploadClient(t, nil, http.StatusOK, "PUBLISHED")
	content := []byte("\x89PNG\r\n\x1a\nimage-bytes")
	image, e := client.PaymentLinks.UploadImage(context.Background(), "logo.png", PaymentLinkImageContentTypePNG, content)
	if e != nil {
		t.Fatal(e)
	}
	if image.AssetID != testAssetID || image.URL != testImageURL || image.State != "PUBLISHED" {
		t.Fatalf("image %+v", image)
	}
	puts, bodies := storage.calls()
	if len(puts) != 1 {
		t.Fatalf("storage requests %d", len(puts))
	}
	put := puts[0]
	if put.Method != "PUT" || put.URL.Path != "/bucket/"+testAssetID || put.URL.Query().Get("X-Amz-Signature") != "presigned" || string(bodies[0]) != string(content) {
		t.Fatalf("storage PUT %s %s", put.Method, put.URL)
	}
	if put.Header.Get("Content-Type") != "image/png" {
		t.Fatal("upload header missing")
	}
	for _, header := range []string{"Authorization", "X-Key-Id", "X-Date", "X-Nonce", "X-Signature", "Digest", "Idempotency-Key"} {
		if put.Header.Get(header) != "" {
			t.Fatalf("storage PUT carried %s", header)
		}
	}
	sent := requests()
	if len(sent) != 2 {
		t.Fatalf("gateway requests %+v", sent)
	}
	if r := sent[0]; r.Method != "POST" || r.URI != "/api/v1/payments/links/images" || !r.Signed || string(r.Body) != fmt.Sprintf(`{"fileName":"logo.png","contentType":"image/png","sizeBytes":%d}`, len(content)) {
		t.Fatalf("create upload %+v %s", r, r.Body)
	}
	if r := sent[1]; r.Method != "POST" || r.URI != "/api/v1/payments/links/images/"+testAssetID+"/confirm" || !r.Signed || string(r.Body) != `{}` {
		t.Fatalf("confirm %+v %s", r, r.Body)
	}
}

func TestPaymentLinkUploadImageFailures(t *testing.T) {
	ctx := context.Background()
	content := []byte("image")

	client, requests, storage := newImageUploadClient(t, nil, http.StatusForbidden, "PUBLISHED")
	_, e := client.PaymentLinks.UploadImage(ctx, "logo.png", PaymentLinkImageContentTypePNG, content)
	var apiErr *APIError
	if !errors.As(e, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Code != "AccessDenied" {
		t.Fatalf("storage failure %v", e)
	}
	if puts, _ := storage.calls(); len(requests()) != 1 || len(puts) != 1 {
		t.Fatal("a failed PUT must not be retried or confirmed")
	}

	client, _, storage = newImageUploadClient(t, func(target map[string]any) { target["uploadMethod"] = "POST" }, http.StatusOK, "PUBLISHED")
	_, e = client.PaymentLinks.UploadImage(ctx, "logo.png", PaymentLinkImageContentTypePNG, content)
	if puts, _ := storage.calls(); !errors.As(e, &apiErr) || apiErr.Code != "INVALID_UPLOAD_TARGET" || len(puts) != 0 {
		t.Fatalf("unusable target %v", e)
	}

	client, _, storage = newImageUploadClient(t, func(target map[string]any) { target["uploadUrl"] = "ftp://storage.example/x" }, http.StatusOK, "PUBLISHED")
	_, e = client.PaymentLinks.UploadImage(ctx, "logo.png", PaymentLinkImageContentTypePNG, content)
	if puts, _ := storage.calls(); !errors.As(e, &apiErr) || apiErr.Code != "INVALID_UPLOAD_TARGET" || len(puts) != 0 {
		t.Fatalf("non-http upload URL %v", e)
	}

	client, _, _ = newImageUploadClient(t, nil, http.StatusOK, "PENDING")
	_, e = client.PaymentLinks.UploadImage(ctx, "logo.png", PaymentLinkImageContentTypePNG, content)
	if !errors.As(e, &apiErr) || apiErr.Code != "IMAGE_NOT_PUBLISHED" {
		t.Fatalf("unpublished image %v", e)
	}

	client, requests, _ = newImageUploadClient(t, nil, http.StatusOK, "PUBLISHED")
	_, e = client.PaymentLinks.UploadImage(ctx, "logo.svg", "image/svg+xml", content)
	mustReject(t, "svg", e)
	_, e = client.PaymentLinks.UploadImage(ctx, "", PaymentLinkImageContentTypePNG, content)
	mustReject(t, "file name", e)
	_, e = client.PaymentLinks.UploadImage(ctx, "logo.png", PaymentLinkImageContentTypePNG, nil)
	mustReject(t, "empty content", e)
	_, e = client.PaymentLinks.CreateImageUpload(ctx, "logo.png", PaymentLinkImageContentTypePNG, 0)
	mustReject(t, "size", e)
	_, e = client.PaymentLinks.ConfirmImageUpload(ctx, "asset-1")
	mustReject(t, "asset id", e)
	if sent := requests(); len(sent) != 0 {
		t.Fatalf("invalid uploads reached the network: %+v", sent)
	}
}

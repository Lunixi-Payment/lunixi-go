package lunixi

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
)

const testWalletKey = "wallet-key-0001"

func TestWalletCreateEndUserSendsRouteFieldsInOrder(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"endUserId":"eu-1"}}`)
	})
	data, err := client.Wallet.EndUsers.Create(context.Background(), map[string]any{
		"displayName": "Ada",
		"phone":       "+905551112233",
		"accountType": WalletAccountTypePersonal,
		"programId":   "prg-1",
		"kycRef":      nil,
	}, testWalletKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"endUserId":"eu-1"}` {
		t.Fatalf("data %s", data)
	}
	sent := requests()
	if len(sent) != 1 || sent[0].Method != "POST" || sent[0].URI != "/api/v1/wallet/end-users" || sent[0].Key != testWalletKey {
		t.Fatalf("request %+v", sent)
	}
	if want := `{"programId":"prg-1","accountType":"PERSONAL","phone":"+905551112233","displayName":"Ada"}`; string(sent[0].Body) != want {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestWalletRefusesBeforeSending(t *testing.T) {
	transfer := func() map[string]any {
		return map[string]any{"endUserId": "eu-1", "sourceWalletAccountId": "wa-1", "amount": "12550", "currency": "TRY"}
	}
	cases := map[string]func(*Client) error{
		"unknown field": func(c *Client) error {
			request := transfer()
			request["businessAccountId"] = "ba-1"
			_, err := c.Wallet.Transfers.InitiateW2W(context.Background(), request, testWalletKey)
			return err
		},
		"missing required": func(c *Client) error {
			request := transfer()
			delete(request, "currency")
			_, err := c.Wallet.Transfers.InitiateW2W(context.Background(), request, testWalletKey)
			return err
		},
		"blank required": func(c *Client) error {
			request := transfer()
			request["endUserId"] = ""
			_, err := c.Wallet.Transfers.QuoteW2W(context.Background(), request)
			return err
		},
		"numeric amount": func(c *Client) error {
			request := transfer()
			request["amount"] = 12550
			_, err := c.Wallet.Transfers.QuoteW2W(context.Background(), request)
			return err
		},
		"decimal amount": func(c *Client) error {
			request := transfer()
			request["amount"] = "125.50"
			_, err := c.Wallet.Transfers.QuoteW2W(context.Background(), request)
			return err
		},
		"zero amount": func(c *Client) error {
			request := transfer()
			request["amount"] = "0"
			_, err := c.Wallet.Transfers.QuoteW2W(context.Background(), request)
			return err
		},
		"missing key": func(c *Client) error {
			_, err := c.Wallet.Transfers.InitiateW2W(context.Background(), transfer(), "")
			return err
		},
		"key with space": func(c *Client) error {
			_, err := c.Wallet.Transfers.InitiateW2W(context.Background(), transfer(), "wallet key")
			return err
		},
		"key too long": func(c *Client) error {
			_, err := c.Wallet.Transfers.InitiateW2W(context.Background(), transfer(), strings.Repeat("k", 129))
			return err
		},
		"unknown filter": func(c *Client) error {
			_, err := c.Wallet.Operations.List(context.Background(), map[string]any{"merchantId": "m-1"})
			return err
		},
		"bad path id": func(c *Client) error {
			_, err := c.Wallet.EndUsers.Get(context.Background(), "../ping")
			return err
		},
		"empty payout items": func(c *Client) error {
			_, err := c.Wallet.BulkPayouts.Create(context.Background(), map[string]any{"programId": "p", "sourceWalletAccountId": "wa", "currency": "TRY", "items": []any{}}, testWalletKey)
			return err
		},
		"payout item amount": func(c *Client) error {
			items := []map[string]any{{"targetType": WalletBulkTargetTypeIBAN, "targetRef": "TR00", "amount": 100}}
			_, err := c.Wallet.BulkPayouts.Create(context.Background(), map[string]any{"programId": "p", "sourceWalletAccountId": "wa", "currency": "TRY", "items": items}, testWalletKey)
			return err
		},
		"employee row field": func(c *Client) error {
			_, err := c.Wallet.Corporate.ImportEmployees(context.Background(), "ca-1", []map[string]any{{"phone": "+90555", "salary": "1"}}, testWalletKey)
			return err
		},
		"QR image scale": func(c *Client) error {
			_, err := c.Wallet.QR.Image(context.Background(), "qr-1", WalletQRImageFormatPNG, 21)
			return err
		},
		"QR image format": func(c *Client) error {
			_, err := c.Wallet.QR.Image(context.Background(), "qr-1", "gif", 0)
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestWalletCompletePutsOperationIDInTheBody(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	_, err := client.Wallet.Transfers.CompleteW2Iban(context.Background(), "op-1", map[string]any{"endUserId": "eu-1", "integrityHash": "h", "nonce": "n", "otpCode": "123456"}, testWalletKey)
	if err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/wallet/transfers/w2iban/op-1/complete" || sent[0].Key != testWalletKey {
		t.Fatalf("request %+v", sent[0])
	}
	if want := `{"endUserId":"eu-1","operationId":"op-1","integrityHash":"h","nonce":"n","otpCode":"123456"}`; string(sent[0].Body) != want {
		t.Fatalf("body %s", sent[0].Body)
	}
	client, requests = newProductClient(t, 0, nil)
	_, err = client.Wallet.Withdrawals.Complete(context.Background(), "op-1", map[string]any{"endUserId": "eu-1", "operationId": "op-2", "integrityHash": "h", "nonce": "n"}, testWalletKey)
	expectLocalError(t, "operation mismatch", err, requests())
}

func TestWalletRoutesWithoutKeyOrBody(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	ctx := context.Background()
	if _, err := client.Wallet.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Wallet.BulkPayouts.Submit(ctx, "batch-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Wallet.BulkPayouts.Approve(ctx, "batch-1", false, "duplicate"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Wallet.EndUsers.GetByExternalID(ctx, "cust-42", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Wallet.EndUsers.GetLimits(ctx, "eu-1", map[string]any{"operationType": "W2W", "currency": "TRY"}); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	want := []productRecorded{
		{Method: "GET", URI: "/api/v1/wallet/ping"},
		{Method: "POST", URI: "/api/v1/wallet/bulk-payouts/batch-1/submit"},
		{Method: "POST", URI: "/api/v1/wallet/bulk-payouts/batch-1/approve", Body: []byte(`{"approve":false,"reason":"duplicate"}`)},
		{Method: "GET", URI: "/api/v1/wallet/end-users/by-external/cust-42"},
		{Method: "GET", URI: "/api/v1/wallet/end-users/eu-1/limits?currency=TRY&operationType=W2W"},
	}
	for i, request := range sent {
		if request.Method != want[i].Method || request.URI != want[i].URI || request.Key != "" || string(request.Body) != string(want[i].Body) {
			t.Fatalf("request %d %+v, want %+v", i, request, want[i])
		}
	}
}

func TestWalletOperationsPageAndIterate(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			productJSON(w, 200, `{"success":true,"data":{"operations":[{"operationId":"op-1","amount":"100"}],"nextCursor":"c2"}}`)
			return
		}
		productJSON(w, 200, `{"success":true,"data":{"operations":[{"operationId":"op-2","amount":"200"}],"nextCursor":null}}`)
	})
	var ids []string
	for item, err := range client.Wallet.Operations.Iterate(context.Background(), map[string]any{"endUserId": "eu-1", "limit": 1}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, decodeBody(t, item)["operationId"].(string))
	}
	if !slices.Equal(ids, []string{"op-1", "op-2"}) {
		t.Fatalf("ids %v", ids)
	}
	sent := requests()
	if len(sent) != 2 || sent[0].URI != "/api/v1/wallet/operations?endUserId=eu-1&limit=1" || sent[1].URI != "/api/v1/wallet/operations?cursor=c2&endUserId=eu-1&limit=1" {
		t.Fatalf("requests %+v", sent)
	}
}

func TestWalletQRImageIsBytes(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	})
	r, err := client.Wallet.QR.Image(context.Background(), "qr-1", WalletQRImageFormatPNG, 4)
	if err != nil {
		t.Fatal(err)
	}
	if r.ContentType != "image/png" || string(r.Bytes) != "\x89PNG" {
		t.Fatalf("response %+v", r)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/wallet/qr/qr-1/image?format=png&scale=4" || !strings.Contains(sent[0].Accept, "image/png") {
		t.Fatalf("request %+v", sent[0])
	}
}

func TestWalletRawDoAcceptsShortKeys(t *testing.T) {
	client, requests := newProductClient(t, 0, nil)
	_, err := client.Do(context.Background(), Request{Method: "POST", Path: "/api/v1/wallet/end-users/eu-1/freeze", Body: map[string]any{}, IdempotencyKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if sent := requests(); len(sent) != 1 || sent[0].Key != "k1" {
		t.Fatalf("requests %+v", sent)
	}
}

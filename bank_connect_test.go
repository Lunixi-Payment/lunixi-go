package lunixi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestBankConnectTransactionFilters(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"items":[{"transactionId":"tx-1","amountMinor":"150075"}]},"pageInfo":{"hasMore":true,"nextCursor":"c2","totalCount":null}}`)
	})
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("TRT", 3*3600))
	page, err := client.BankConnect.ListTransactions(context.Background(), map[string]any{
		"accountId":      []string{"acc-1", " acc-2 "},
		"bookedFrom":     from,
		"bookedTo":       "2026-11-01T00:00:00+03:00",
		"direction":      BankTransactionDirectionCredit,
		"canonicalType":  []string{BankTransactionTypeTransferIn, BankTransactionTypeFX},
		"minAmountMinor": "0",
		"maxAmountMinor": "500000",
		"limit":          50,
		"cursor":         "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || !page.HasMore || page.NextCursor != "c2" || page.TotalCount != nil {
		t.Fatalf("page %+v", page)
	}
	sent := requests()
	want := "/api/v1/bank-connect/transactions?accountId=acc-1%2Cacc-2&bookedFrom=2026-09-30T21%3A00%3A00.000Z&bookedTo=2026-11-01T00%3A00%3A00%2B03%3A00&canonicalType=TRANSFER_IN%2CFX&direction=CREDIT&limit=50&maxAmountMinor=500000&minAmountMinor=0"
	if len(sent) != 1 || sent[0].Method != "GET" || sent[0].URI != want || sent[0].Key != "" {
		t.Fatalf("request %+v", sent)
	}
}

func TestBankConnectFilterRules(t *testing.T) {
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "acc"
	}
	cases := map[string]func(*Client) error{
		"unknown filter": func(c *Client) error {
			_, err := c.BankConnect.ListAccounts(context.Background(), map[string]any{"environment": "LIVE"})
			return err
		},
		"limit zero": func(c *Client) error {
			_, err := c.BankConnect.ListBanks(context.Background(), map[string]any{"limit": 0})
			return err
		},
		"limit 101": func(c *Client) error {
			_, err := c.BankConnect.ListBanks(context.Background(), map[string]any{"limit": 101})
			return err
		},
		"account status set": func(c *Client) error {
			_, err := c.BankConnect.ListAccounts(context.Background(), map[string]any{"status": BankConnectionStatusDegraded})
			return err
		},
		"connection status list": func(c *Client) error {
			_, err := c.BankConnect.ListConnections(context.Background(), map[string]any{"status": []string{BankConnectionStatusActive, "BROKEN"}})
			return err
		},
		"101 account ids": func(c *Client) error {
			_, err := c.BankConnect.ListBalances(context.Background(), map[string]any{"accountId": ids})
			return err
		},
		"account id with comma": func(c *Client) error {
			_, err := c.BankConnect.ListBalances(context.Background(), map[string]any{"accountId": "a,b"})
			return err
		},
		"numeric amount": func(c *Client) error {
			_, err := c.BankConnect.ListTransactions(context.Background(), map[string]any{"minAmountMinor": 100})
			return err
		},
		"min above max": func(c *Client) error {
			_, err := c.BankConnect.ListTransactions(context.Background(), map[string]any{"minAmountMinor": "500", "maxAmountMinor": "100"})
			return err
		},
		"bookedTo alone": func(c *Client) error {
			_, err := c.BankConnect.ListTransactions(context.Background(), map[string]any{"bookedTo": "2026-11-01T00:00:00Z"})
			return err
		},
		"from after to": func(c *Client) error {
			_, err := c.BankConnect.ListTransactions(context.Background(), map[string]any{"bookedFrom": "2026-11-02T00:00:00Z", "bookedTo": "2026-11-01T00:00:00Z"})
			return err
		},
		"window over 92 days": func(c *Client) error {
			_, err := c.BankConnect.ListTransactions(context.Background(), map[string]any{"bookedFrom": "2026-01-01T00:00:00Z", "bookedTo": "2026-04-03T00:00:01Z"})
			return err
		},
		"currency": func(c *Client) error {
			_, err := c.BankConnect.ListAccounts(context.Background(), map[string]any{"currency": "TL"})
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestBankConnectRecordsAndSync(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.Method == "POST" {
			productJSON(w, 202, `{"success":true,"data":{"accountId":"acc-1","syncStatus":"SCHEDULED","minSyncIntervalSeconds":900}}`)
			return
		}
		productJSON(w, 200, `{"success":true,"data":{"accountId":"acc-1"}}`)
	})
	ctx := context.Background()
	if _, err := client.BankConnect.GetAccount(ctx, "acc-1"); err != nil {
		t.Fatal(err)
	}
	data, err := client.BankConnect.RequestSync(ctx, "acc-1")
	if err != nil || !strings.Contains(string(data), BankSyncStatusScheduled) {
		t.Fatalf("sync %s %v", data, err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/bank-connect/accounts/acc-1" || sent[1].Method != "POST" || sent[1].URI != "/api/v1/bank-connect/accounts/acc-1/sync" || len(sent[1].Body) != 0 || sent[1].Key != "" {
		t.Fatalf("requests %+v", sent)
	}
}

func TestBankTransferCreateGeneratesKeyAndRetriesWithIt(t *testing.T) {
	client, requests := newProductClient(t, 2, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			productJSON(w, 503, `{"success":false,"code":"bank_connect.unavailable"}`)
			return
		}
		productJSON(w, 201, `{"success":true,"data":{"transfer":{"id":"bt-1","status":"SUBMITTED","amountMinor":"150075"},"stepUp":null,"replayed":false}}`)
	})
	result, err := client.BankConnect.Transfers.Create(context.Background(), map[string]any{
		"accountId":      "acc-1",
		"receiverIban":   " TR330006100519786457841326 ",
		"receiverName":   "Ada Lovelace",
		"amountMinor":    "150075",
		"currency":       "try",
		"paymentPurpose": "SALARY_01",
		"description":    "",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !uuidV4.MatchString(result.IdempotencyKey) || result.StatusCode != 201 || result.Replayed || result.Transfer() == nil {
		t.Fatalf("result %+v", result)
	}
	sent := requests()
	if len(sent) != 2 || sent[0].Key != result.IdempotencyKey || sent[1].Key != result.IdempotencyKey || string(sent[0].Body) != string(sent[1].Body) {
		t.Fatalf("attempts %+v", sent)
	}
	want := `{"accountId":"acc-1","receiverIban":"TR330006100519786457841326","receiverName":"Ada Lovelace","amountMinor":"150075","currency":"TRY","paymentPurpose":"SALARY_01"}`
	if sent[0].URI != "/api/v1/bank-connect/transfers" || string(sent[0].Body) != want {
		t.Fatalf("request %s %s", sent[0].URI, sent[0].Body)
	}
}

func TestBankTransferKeepsTheCallersKeyAndReadsReplay(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 201, `{"success":true,"data":{"order":{"id":"fx-1","status":"SUBMITTED"},"replayed":true}}`)
	})
	result, err := client.BankConnect.FX.CreateOrder(context.Background(), map[string]any{"quoteId": "q-1"}, "payout-2026-0001")
	if err != nil {
		t.Fatal(err)
	}
	if result.IdempotencyKey != "payout-2026-0001" || !result.Replayed || result.Order() == nil {
		t.Fatalf("result %+v", result)
	}
	if sent := requests(); sent[0].Key != "payout-2026-0001" || sent[0].URI != "/api/v1/bank-connect/fx/orders" || string(sent[0].Body) != `{"quoteId":"q-1"}` {
		t.Fatalf("request %+v", sent[0])
	}
}

func TestBankTransferRefusesBeforeSending(t *testing.T) {
	transfer := func() map[string]any {
		return map[string]any{"accountId": "acc-1", "receiverIban": "TR330006100519786457841326", "receiverName": "Ada", "amountMinor": "100"}
	}
	cases := map[string]func(*Client) error{
		"unknown field": func(c *Client) error {
			request := transfer()
			request["environment"] = "LIVE"
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"missing field": func(c *Client) error {
			request := transfer()
			delete(request, "receiverName")
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"numeric amount": func(c *Client) error {
			request := transfer()
			request["amountMinor"] = 100
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"zero amount": func(c *Client) error {
			request := transfer()
			request["amountMinor"] = "0"
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"short name": func(c *Client) error {
			request := transfer()
			request["receiverName"] = "A"
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"long IBAN": func(c *Client) error {
			request := transfer()
			request["receiverIban"] = strings.Repeat("1", 43)
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"purpose": func(c *Client) error {
			request := transfer()
			request["paymentPurpose"] = "salary payment"
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"description": func(c *Client) error {
			request := transfer()
			request["description"] = strings.Repeat("ç", 141)
			_, err := c.BankConnect.Transfers.Create(context.Background(), request, "")
			return err
		},
		"short key": func(c *Client) error {
			_, err := c.BankConnect.Transfers.Create(context.Background(), transfer(), "short")
			return err
		},
		"quote with both amounts": func(c *Client) error {
			_, err := c.BankConnect.FX.CreateQuote(context.Background(), map[string]any{"sourceAccountId": "a", "targetAccountId": "b", "sourceAmountMinor": "1", "targetAmountMinor": "2"})
			return err
		},
		"quote without amount": func(c *Client) error {
			_, err := c.BankConnect.FX.CreateQuote(context.Background(), map[string]any{"sourceAccountId": "a", "targetAccountId": "b"})
			return err
		},
		"quote same account": func(c *Client) error {
			_, err := c.BankConnect.FX.CreateQuote(context.Background(), map[string]any{"sourceAccountId": "a", "targetAccountId": "a", "sourceAmountMinor": "1"})
			return err
		},
		"order description": func(c *Client) error {
			_, err := c.BankConnect.FX.CreateOrder(context.Background(), map[string]any{"quoteId": "q", "description": strings.Repeat("x", 141)}, "")
			return err
		},
		"transfer status filter": func(c *Client) error {
			_, err := c.BankConnect.Transfers.List(context.Background(), map[string]any{"status": BankFXOrderStatusCompleted + "X"})
			return err
		},
		"order filter": func(c *Client) error {
			_, err := c.BankConnect.FX.ListOrders(context.Background(), map[string]any{"accountId": "a"})
			return err
		},
		"purposes without connection": func(c *Client) error {
			_, err := c.BankConnect.Transfers.ListPurposes(context.Background(), " ")
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestBankTransferListFilters(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"items":[]},"pageInfo":{"hasMore":false,"nextCursor":null}}`)
	})
	_, err := client.BankConnect.Transfers.List(context.Background(), map[string]any{
		"status":      []string{BankTransferStatusCompleted, BankTransferStatusReturned},
		"createdFrom": time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		"accountId":   "acc-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent := requests(); sent[0].URI != "/api/v1/bank-connect/transfers?accountId=acc-1&createdFrom=2026-11-01T00%3A00%3A00.000Z&status=COMPLETED%2CRETURNED" {
		t.Fatalf("request %s", sent[0].URI)
	}
}

func TestBankConnectWebhookData(t *testing.T) {
	received := productWebhook(t, `{"id":"evt-1","type":"bank_account.transaction.received","data":{"transactionId":"tx-1","accountId":"acc-1","connectionId":"con-1","bankEftCode":"0062","bankName":"Bank","direction":"CREDIT","amountMinor":"150075","currency":"TRY","bookedAt":"2026-11-15T09:00:00Z","valueAt":null,"receivedAt":"2026-11-15T09:01:00Z","canonicalType":"TRANSFER_IN","description":"Invoice 42","counterpartyNameMasked":"A** L***","counterpartyIbanMasked":null,"receiptNo":null}}`)
	data, err := ParseBankAccountTransactionReceived(received)
	if err != nil {
		t.Fatal(err)
	}
	if data.TransactionID != "tx-1" || data.AmountMinor != "150075" || data.ValueAt != nil || data.Description == nil || *data.Description != "Invoice 42" {
		t.Fatalf("data %+v", data)
	}
	if _, err = ParseBankTransferWebhook(received); !errors.Is(err, ErrWebhook) {
		t.Fatalf("a bank-account event read as a transfer: %v", err)
	}
	transfer := productWebhook(t, `{"id":"evt-1","type":"bank_transfer.failed","data":{"transferId":"bt-1","connectionId":"con-1","accountId":"acc-1","bankEftCode":"0062","bankName":"Bank","status":"FAILED","rail":null,"amountMinor":"100","currency":"TRY","receiverIbanMasked":"TR** **26","receiverNameMasked":"A** L***","paymentPurpose":null,"description":null,"failureCode":"bank_connect.transfer.insufficient_balance","idempotencyKey":"payout-1","createdAt":"2026-11-15T09:00:00Z","completedAt":null,"failedAt":"2026-11-15T09:02:00Z","returnedAt":null}}`)
	event, err := ParseBankTransferWebhook(transfer)
	if err != nil {
		t.Fatal(err)
	}
	if event.Status != BankTransferStatusFailed || event.Rail != nil || event.FailureCode == nil || event.IdempotencyKey != "payout-1" {
		t.Fatalf("event %+v", event)
	}
	numeric := productWebhook(t, `{"id":"evt-1","type":"bank_transfer.created","data":{"transferId":"bt-1","amountMinor":100}}`)
	if _, err = ParseBankTransferWebhook(numeric); !errors.Is(err, ErrWebhook) {
		t.Fatalf("numeric amount: %v", err)
	}
	payment := productWebhook(t, `{"id":"evt-1","type":"payment.bank_transfer.underpaid","data":{"amountMinor":"100"}}`)
	if _, err = ParseBankTransferWebhook(payment); !errors.Is(err, ErrWebhook) {
		t.Fatalf("payment event read as a bank transfer: %v", err)
	}
}

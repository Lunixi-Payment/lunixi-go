package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// BankTransferClient — outgoing bank transfers, Client.BankConnect.Transfers
// (/api/v1/bank-connect/transfers, product BANK_CONNECT).
//
// Available on a connection whose capabilities include OUTGOING_TRANSFER.
// Every route, reads included, is signed and needs the bank-connect:transfer
// scope; the key must have an IP allowlist and the request must come from a
// listed IP (403 bank_connect.auth.ip_allowlist_required /
// bank_connect.auth.ip_not_allowed).
//
// With an API key there is no e-mail code: the key's scope, the signature
// and the source IP approve the transfer, so Create answers SUBMITTED and
// the transfer is queued for the bank. Follow it with Get or the
// BankTransferEvent* webhooks. The bank chooses the rail (own account, same
// bank, FAST or EFT); Lunixi does not check the balance — the bank decides.
// When the outcome at the bank is unknown the transfer stays SUBMITTED with
// outcomeUnknown true and Lunixi settles it; do not send it again.
type BankTransferClient struct{ client *Client }

// ListPurposes lists the payment purpose codes the bank accepts on this
// connection: {items: [{code, name, order}]}. Pass one as paymentPurpose to
// Create; leave it out for the bank's default.
func (t *BankTransferClient) ListPurposes(ctx context.Context, connectionID string) (json.RawMessage, error) {
	id, err := requiredText(connectionID, "connectionId")
	if err != nil {
		return nil, err
	}
	return bankTransferData(ctx, t.client, "listTransferPurposes", nil, url.Values{"connectionId": {id}}, nil, "")
}

// Create creates an outgoing transfer (HTTP 201); Data is {transfer,
// stepUp, replayed}.
//
// request: accountId (source account), receiverIban (TR IBAN), receiverName
// (2–140 characters), amountMinor (positive minor-unit text: "150075" is
// 1,500.75 TRY), optional currency (TRY), paymentPurpose (a ListPurposes
// code) and description (at most 140 characters).
//
// idempotencyKey (8–200 printable ASCII characters) is generated (UUID v4)
// when "" and returned as IdempotencyKey; a network error or a 5xx is
// retried (MaxRetries) with the same key and body. Keep your own key (e.g.
// your payout id) to retry safely across processes: the same key with the
// same body answers the existing transfer with Replayed true, the same key
// with another body is 409 bank_connect.transfer.idempotency_conflict.
func (t *BankTransferClient) Create(ctx context.Context, request map[string]any, idempotencyKey string) (*BankConnectWriteResult, error) {
	key, err := bankTransferKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	body, err := bankTransferBody("createTransfer", request)
	if err != nil {
		return nil, err
	}
	if err = checkTransferBody(body); err != nil {
		return nil, err
	}
	return bankTransferWrite(ctx, t.client, "createTransfer", body, key)
}

// List lists transfers, newest first. Filters: accountId, status (one
// BankTransferStatus* value or a list), createdFrom (inclusive) / createdTo
// (exclusive) as a time.Time or ISO-8601 text, limit 1–100, cursor.
func (t *BankTransferClient) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return bankTransferList(ctx, t.client, "listTransfers", filters)
}

// Iterate yields every transfer matching the filters, page by page.
func (t *BankTransferClient) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, t.List)
}

// Get reads one transfer; 404 bank_connect.transfer.not_found for an unknown
// id.
func (t *BankTransferClient) Get(ctx context.Context, transferID string) (json.RawMessage, error) {
	return bankTransferData(ctx, t.client, "getTransfer", map[string]string{"transferId": transferID}, nil, nil, "")
}

// BankFXClient — currency exchange between two of your accounts,
// Client.BankConnect.FX (/api/v1/bank-connect/fx, product BANK_CONNECT).
//
// Available on a connection whose capabilities include FX_EXCHANGE. The two
// accounts are active accounts of the same connection in different
// currencies. Same scope and IP allowlist rule as transfers.
//
// Flow: ListRates (optional) → CreateQuote → CreateOrder with the quote id
// before quote.expiresAt (the bank sets the validity, typically under two
// minutes; see validForSeconds). The order is SUBMITTED and queued; follow
// it with GetOrder. There is no webhook for exchange orders.
type BankFXClient struct{ client *Client }

// ListRates reads the current rates on the connection: {items,
// refreshSeconds, fetchedAt}; rates are decimal text.
func (f *BankFXClient) ListRates(ctx context.Context, connectionID string) (json.RawMessage, error) {
	id, err := requiredText(connectionID, "connectionId")
	if err != nil {
		return nil, err
	}
	return bankTransferData(ctx, f.client, "listFxRates", nil, url.Values{"connectionId": {id}}, nil, "")
}

// CreateQuote quotes an exchange (HTTP 201). request: sourceAccountId (you
// sell from it), targetAccountId (you buy into it) and exactly one of
// sourceAmountMinor / targetAmountMinor (positive minor-unit text of that
// account's currency). Nothing is booked; a quote is used by one order.
func (f *BankFXClient) CreateQuote(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	body, err := bankTransferBody("createFxQuote", request)
	if err != nil {
		return nil, err
	}
	if body.has("sourceAmountMinor") == body.has("targetAmountMinor") {
		return nil, invalid("createFxQuote takes exactly one of sourceAmountMinor and targetAmountMinor")
	}
	source, _ := body.get("sourceAccountId")
	target, _ := body.get("targetAccountId")
	if source == target {
		return nil, invalid("createFxQuote needs two different accounts")
	}
	return bankTransferData(ctx, f.client, "createFxQuote", nil, nil, body, "")
}

// CreateOrder executes a quote (HTTP 201); Data is {order, replayed}.
// request: quoteId, optional description (at most 140 characters). The key
// follows the same rule as Transfers.Create and is generated when "". 409
// bank_connect.fx.quote_expired / bank_connect.fx.quote_used: ask for a new
// quote.
func (f *BankFXClient) CreateOrder(ctx context.Context, request map[string]any, idempotencyKey string) (*BankConnectWriteResult, error) {
	key, err := bankTransferKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	body, err := bankTransferBody("createFxOrder", request)
	if err != nil {
		return nil, err
	}
	if description, ok := body.get("description"); ok {
		if err = checkTransferDescription(description); err != nil {
			return nil, err
		}
	}
	return bankTransferWrite(ctx, f.client, "createFxOrder", body, key)
}

// ListOrders lists exchange orders, newest first. Filters: status (one
// BankFXOrderStatus* value or a list), limit, cursor.
func (f *BankFXClient) ListOrders(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return bankTransferList(ctx, f.client, "listFxOrders", filters)
}

// IterateOrders yields every exchange order matching the filters.
func (f *BankFXClient) IterateOrders(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, f.ListOrders)
}

// GetOrder reads one exchange order; 404 bank_connect.fx.order_not_found
// for an unknown id.
func (f *BankFXClient) GetOrder(ctx context.Context, orderID string) (json.RawMessage, error) {
	return bankTransferData(ctx, f.client, "getFxOrder", map[string]string{"orderId": orderID}, nil, nil, "")
}

// BankConnectWriteResult is the answer of a keyed bank-connect write
// (Transfers.Create, FX.CreateOrder): HTTP 201, Replayed true when the same
// key and body were sent before. IdempotencyKey is the key that was sent
// (yours, or the generated one); keep it to retry the same operation.
type BankConnectWriteResult struct {
	// Data is the whole data object ({transfer, stepUp, replayed} or
	// {order, replayed}).
	Data           json.RawMessage
	Replayed       bool
	StatusCode     int
	IdempotencyKey string
	RequestID      string
}

// Record returns one field of Data, nil when absent or null.
func (r *BankConnectWriteResult) Record(name string) json.RawMessage {
	return recordField(r.Data, name)
}

// Transfer returns data.transfer of Transfers.Create.
func (r *BankConnectWriteResult) Transfer() json.RawMessage { return r.Record("transfer") }

// Order returns data.order of FX.CreateOrder.
func (r *BankConnectWriteResult) Order() json.RawMessage { return r.Record("order") }

// DecodeData decodes Data using json.Number for interface values.
func (r *BankConnectWriteResult) DecodeData(target any) error { return decodeNumbers(r.Data, target) }

// ── Route invoker ────────────────────────────────────────────────────────

func bankTransferSend(ctx context.Context, c *Client, name string, params map[string]string, query url.Values, body orderedObject, key string) (*Response, error) {
	route := bankTransferRoutes[name]
	path, err := fillRoutePath(route.Path, params)
	if err != nil {
		return nil, err
	}
	request := Request{Method: route.Method, Path: path, Query: query, IdempotencyKey: key}
	if body != nil {
		request.Body = body
	}
	return c.Do(ctx, request)
}

func bankTransferData(ctx context.Context, c *Client, name string, params map[string]string, query url.Values, body orderedObject, key string) (json.RawMessage, error) {
	r, err := bankTransferSend(ctx, c, name, params, query, body, key)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func bankTransferWrite(ctx context.Context, c *Client, name string, body orderedObject, key string) (*BankConnectWriteResult, error) {
	r, err := bankTransferSend(ctx, c, name, nil, nil, body, key)
	if err != nil {
		return nil, err
	}
	data := r.Data
	if objectFields(data) == nil {
		data = json.RawMessage(`{}`)
	}
	return &BankConnectWriteResult{Data: data, Replayed: boolField(objectFields(data), "replayed"), StatusCode: r.StatusCode, IdempotencyKey: key, RequestID: r.RequestID}, nil
}

func bankTransferList(ctx context.Context, c *Client, name string, filters map[string]any) (*CursorPage, error) {
	query, err := bankTransferQuery(filters, name)
	if err != nil {
		return nil, err
	}
	r, err := bankTransferSend(ctx, c, name, nil, query, nil, "")
	if err != nil {
		return nil, err
	}
	return envelopePage(r), nil
}

// bankTransferKey checks the caller's key, or generates a UUID v4 for "".
func bankTransferKey(key string) (string, error) {
	if key == "" {
		return randomUUID(), nil
	}
	if !validPrintableKey(key) {
		return "", invalid("idempotencyKey must be 8-200 printable ASCII characters (no spaces)")
	}
	return key, nil
}

// bankTransferBody whitelists a body against its route: nil and "" members
// are left out; unknown or missing fields fail before sending; amounts are
// positive minor-unit text; the other required fields are non-empty text,
// sent trimmed.
func bankTransferBody(name string, request map[string]any) (orderedObject, error) {
	object, err := plainObject(request, name+" body")
	if err != nil {
		return nil, err
	}
	route := bankTransferRoutes[name]
	plain := map[string]any{}
	for key, value := range object {
		if presentValue(value) {
			plain[key] = value
		}
	}
	if err = checkFieldNames(name, plain, route.Required, route.Optional, presentValue); err != nil {
		return nil, err
	}
	for key, value := range plain {
		if strings.HasSuffix(key, "AmountMinor") || key == "amountMinor" {
			text, ok := value.(string)
			if !ok || !validMinor(text, true, false) {
				return nil, invalid("'" + key + "' must be a canonical string in minor units")
			}
		}
	}
	for _, key := range route.Required {
		if key == "amountMinor" {
			continue
		}
		if plain[key], err = requiredText(plain[key], key); err != nil {
			return nil, invalid("'" + key + "' must be a non-empty string")
		}
	}
	return orderedFields(plain, route.Fields()), nil
}

func checkTransferBody(body orderedObject) error {
	iban, _ := body.get("receiverIban")
	if utf8.RuneCountInString(iban.(string)) > BankTransferIBANMax {
		return invalid("'receiverIban' must be at most " + strconv.Itoa(BankTransferIBANMax) + " characters")
	}
	name, _ := body.get("receiverName")
	if n := utf8.RuneCountInString(name.(string)); n < BankTransferReceiverNameMin || n > BankTransferReceiverNameMax {
		return invalid("'receiverName' must be " + strconv.Itoa(BankTransferReceiverNameMin) + "-" + strconv.Itoa(BankTransferReceiverNameMax) + " characters")
	}
	for i := range body {
		switch body[i].key {
		case "currency":
			currency, err := currencyCode(body[i].value, "currency")
			if err != nil {
				return err
			}
			body[i].value = currency
		case "paymentPurpose":
			purpose, ok := body[i].value.(string)
			if !ok || !bankTransferPaymentPurposePattern.MatchString(purpose) {
				return invalid("'paymentPurpose' must be a code from ListPurposes (letters, digits, underscore; at most 64)")
			}
		case "description":
			if err := checkTransferDescription(body[i].value); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkTransferDescription(value any) error {
	text, ok := value.(string)
	if !ok || utf8.RuneCountInString(text) > BankTransferDescriptionMax {
		return invalid("'description' must be a string of at most " + strconv.Itoa(BankTransferDescriptionMax) + " characters")
	}
	return nil
}

// bankTransferQuery checks the filters of listTransfers / listFxOrders
// against the route's query fields.
func bankTransferQuery(filters map[string]any, name string) (url.Values, error) {
	allowed := bankTransferRoutes[name].Query
	if err := unknownFilters(filters, allowed, "bank transfer"); err != nil {
		return nil, err
	}
	query := url.Values{}
	for _, key := range allowed {
		value := filters[key]
		if emptyFilter(value) {
			continue
		}
		var text string
		var err error
		switch key {
		case "limit":
			text, err = pageLimit(value, BankConnectMaxLimit)
		case "cursor":
			text, err = pageCursor(value, bankConnectCursorMax)
		case "status":
			if name == "listFxOrders" {
				text, err = valueList(key, value, BankFXOrderStatuses())
			} else {
				text, err = valueList(key, value, BankTransferStatuses())
			}
		case "accountId":
			text, err = requiredText(value, key)
		case "createdFrom", "createdTo":
			text, err = timeText(value, key)
		}
		if err != nil {
			return nil, err
		}
		query.Set(key, text)
	}
	return query, nil
}

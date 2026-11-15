package lunixi

import (
	"context"
	"encoding/json"
	"iter"
)

// PhysicalPosClient — payments on your card terminals, transactions,
// reports, end-of-day runs and results (/api/v1/physical-pos, product
// PHYSICAL_POS).
//
// Every call is signed with your API key; the key decides the environment (a
// TEST key can read but every payment write is 403
// physical_pos.environment.test_key_read_only). Payment writes need the scope
// physical_pos:payments; reads need physical_pos:view or
// physical_pos:payments.
//
// Requests and filters are maps with the gateway's field names
// (PhysicalPosRequestFields, PhysicalPosFilters). Card data is never sent:
// the card is read on the device. A field that looks like card data (pan,
// cardNumber, cvv, expiry …) or a card number in merchantReference, an item
// name or tax group code, or a free-text filter returns
// *PhysicalPosCardDataRejectedError before anything is sent; any other
// unknown field returns ErrConfiguration. Amounts are canonical minor-unit
// text ("4500"); counts, line numbers and quantities are integers.
//
// A payment write is asynchronous: the answer means "accepted", not "paid".
// Follow the outcome with the physical_pos.payment.* webhooks or GetPayment.
// Resolving a payment under manual review is done in the panel only.
//
// Idempotency — store the key first, then call:
//
//	key := lunixi.NewPhysicalPosIdempotencyKey()
//	orders.SavePosKey(orderID, key) // your storage
//	result, err := client.PhysicalPos.CreatePayment(ctx, request, key)
//	var unknown *lunixi.PhysicalPosOutcomeUnknownError
//	if errors.As(err, &unknown) {
//		// the payment may exist: call again later with the SAME key
//	}
//
// A new key for the same payment can charge the card twice. For a SALE, also
// send merchantReference with allowDuplicateReference false: the gateway
// then refuses a second sale while one with that reference is unresolved
// (physical_pos.payment.previous_outcome_unknown).
type PhysicalPosClient struct{ client *Client }

// CreatePayment starts a payment on a device: SALE, PREAUTH_OPEN or
// REFUND_UNMATCHED (POST /payments). target is exactly one of {deviceId},
// {vendorCode, serialNumber} or {externalRef}; items (SALE only) are
// {lineNo, name, quantityMilli, unitCode, unitPriceMinor, taxGroupCode,
// lineTotalMinor, barcode?}.
//
// instrument is CARD (default) or a provider-collected card such as
// MEAL_CARD; for those send interaction (INSTRUMENT_READ: the device reads
// the card, CODE_DISPLAY: the device shows the provider's QR / short code)
// and, when the terminal has several provider rows, terminalProviderId (a
// ListTerminalProviders row id). Leave both out for CARD.
//
// idempotencyKey is required and sent exactly as given. The result is 202
// with paymentStatus NOT_STARTED for a new payment, 200 with Replayed true
// when the same key and body were sent before (the payment's current view).
// When the outcome is unknown the error is *PhysicalPosOutcomeUnknownError.
func (p *PhysicalPosClient) CreatePayment(ctx context.Context, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	key, err := posRequireIdempotencyKey(idempotencyKey, "a physical POS payment")
	if err != nil {
		return nil, err
	}
	body, err := posBuildBody(request, "createPayment")
	if err != nil {
		return nil, err
	}
	return p.client.posPaymentWrite(ctx, "/payments", body, key)
}

// CreateReversal reverses an approved payment
// (POST /payments/{paymentId}/reversals): VOID, REFUND (by amountMinor or by
// items [{lineNo, quantityMilli, amountMinor?}]), PREAUTH_CLOSE or
// PREAUTH_CANCEL. The reversal is a new child payment (originalPaymentId =
// paymentID) followed like any payment. idempotencyKey is required; same
// result and errors as CreatePayment. A void after the device's end-of-day
// is *PhysicalPosVoidAfterSettlementError: refund it instead.
func (p *PhysicalPosClient) CreateReversal(ctx context.Context, paymentID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	id, err := posPathID(paymentID, "paymentId")
	if err != nil {
		return nil, err
	}
	key, err := posRequireIdempotencyKey(idempotencyKey, "a physical POS reversal")
	if err != nil {
		return nil, err
	}
	body, err := posBuildBody(request, "createReversal")
	if err != nil {
		return nil, err
	}
	return p.client.posPaymentWrite(ctx, "/payments/"+id+"/reversals", body, key)
}

// CancelPayment cancels a payment before its start command reaches the
// device (POST /payments/{paymentId}/cancel, 200). Read
// payment.paymentStatus: only EXPIRED with statusReason
// MERCHANT_CANCELLED_BEFORE_DISPATCH means it was cancelled; a payment
// already in a final state comes back unchanged with Replayed true. To undo
// an approved sale use a reversal. Cancelling is idempotent, so the key is
// optional ("" sends a new UUID, returned as IdempotencyKey).
func (p *PhysicalPosClient) CancelPayment(ctx context.Context, paymentID, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	id, err := posPathID(paymentID, "paymentId")
	if err != nil {
		return nil, err
	}
	key, err := posIdempotencyKeyOf(idempotencyKey)
	if err != nil {
		return nil, err
	}
	return p.client.posPaymentWrite(ctx, "/payments/"+id+"/cancel", map[string]any{}, key)
}

// GetPayment reads one payment with its items and refund items
// (GET /payments/{paymentId}).
func (p *PhysicalPosClient) GetPayment(ctx context.Context, paymentID string) (json.RawMessage, error) {
	return p.record(ctx, "/payments/", paymentID, "paymentId", "payment")
}

// ListPayments lists payments, newest first (GET /payments). Roots and
// reversals are in one list; items and refundItems are empty here (read one
// payment for them). Filters: limit, cursor, deviceId, paymentStatus,
// createdFrom, createdTo, merchantReference (finds a payment after an
// unknown outcome).
func (p *PhysicalPosClient) ListPayments(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return p.client.posList(ctx, "/payments", filters, "listPayments")
}

// IteratePayments yields every payment matching the filters, page by page.
func (p *PhysicalPosClient) IteratePayments(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, p.ListPayments)
}

// ListTransactions lists transaction trees (root payment + its reversals),
// newest first (GET /transactions). Filters: limit, cursor, deviceId,
// storeId, registerId, paymentStatus, createdFrom, createdTo,
// merchantReference, amountFromMinor, amountToMinor, origin, operation.
func (p *PhysicalPosClient) ListTransactions(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return p.client.posList(ctx, "/transactions", filters, "listTransactions")
}

// IterateTransactions yields every transaction tree matching the filters.
func (p *PhysicalPosClient) IterateTransactions(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, p.ListTransactions)
}

// GetTransaction reads the tree of a root or child payment with its
// timeline (GET /transactions/{paymentId}).
func (p *PhysicalPosClient) GetTransaction(ctx context.Context, paymentID string) (json.RawMessage, error) {
	return p.record(ctx, "/transactions/", paymentID, "paymentId", "transaction")
}

// ListAmbiguousPayments lists payments with an unknown outcome or under
// manual review (GET /transactions/ambiguous). Filters: limit, cursor,
// deviceId, paymentStatus (UNKNOWN or MANUAL_REVIEW).
func (p *PhysicalPosClient) ListAmbiguousPayments(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return p.client.posList(ctx, "/transactions/ambiguous", filters, "listAmbiguousPayments")
}

// IterateAmbiguousPayments yields every ambiguous payment, page by page.
func (p *PhysicalPosClient) IterateAmbiguousPayments(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, p.ListAmbiguousPayments)
}

// GetSalesSummary returns sale / refund / void counts and amounts with the
// net per currency (GET /reports/sales-summary). dateFrom / dateTo are
// business days (YYYY-MM-DD, Europe/Istanbul; a time.Time is sent as its own
// calendar day); groupBy DAY, STORE, DEVICE or INSTRUMENT; also deviceId,
// storeId.
func (p *PhysicalPosClient) GetSalesSummary(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	return p.client.posData(ctx, "/reports/sales-summary", filters, "getSalesSummary")
}

// GetTaxSummary returns the tax breakdown with refunds and voids deducted
// (GET /reports/tax-summary). Filters: dateFrom, dateTo, deviceId, storeId.
func (p *PhysicalPosClient) GetTaxSummary(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	return p.client.posData(ctx, "/reports/tax-summary", filters, "getTaxSummary")
}

// ListSettlements lists end-of-day results per bank line, with the reported
// / system / difference totals (GET /settlements). Filters: limit, cursor,
// deviceId, lineStatus (MATCHED, MISMATCH, UNMAPPED), createdFrom, createdTo.
func (p *PhysicalPosClient) ListSettlements(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return p.client.posList(ctx, "/settlements", filters, "listSettlements")
}

// IterateSettlements yields every settlement line, page by page.
func (p *PhysicalPosClient) IterateSettlements(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, p.ListSettlements)
}

// StartSettlement starts end-of-day (batch close) on a device
// (POST /devices/{deviceId}/settlement, physical_pos:payments). End-of-day
// cannot be undone. The answer means "accepted": the device reports the
// result later — follow it with GetSettlementRun or ListSettlements. The
// result holds data.run (Record("run")), Replayed, StatusCode (202 new / 200
// replay) and IdempotencyKey. Pass the key you stored, or "" for a new UUID;
// after an unknown outcome call again with the same key. A TEST key is
// refused with 403 physical_pos.environment.test_key_read_only.
func (p *PhysicalPosClient) StartSettlement(ctx context.Context, deviceID, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	id, err := posPathID(deviceID, "deviceId")
	if err != nil {
		return nil, err
	}
	key, err := posIdempotencyKeyOf(idempotencyKey)
	if err != nil {
		return nil, err
	}
	r, err := p.client.posSend(ctx, "POST", "/devices/"+id+"/settlement", map[string]any{}, nil, key)
	if err != nil {
		return nil, err
	}
	return posWriteResult(r, key), nil
}

// ListSettlementRuns lists the end-of-day runs started by Lunixi (manual
// and scheduled), newest first (GET /settlement-runs). Filters: limit,
// cursor, includeTotal, deviceId, source (MANUAL_API, MANUAL_PANEL,
// SCHEDULED), runStatus (PENDING, STARTED, SKIPPED, FAILED), createdFrom,
// createdTo.
func (p *PhysicalPosClient) ListSettlementRuns(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return p.client.posList(ctx, "/settlement-runs", filters, "listSettlementRuns")
}

// IterateSettlementRuns yields every end-of-day run matching the filters.
func (p *PhysicalPosClient) IterateSettlementRuns(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, p.ListSettlementRuns)
}

// GetSettlementRun reads one end-of-day run with its command status and
// result state (GET /settlement-runs/{runId}).
func (p *PhysicalPosClient) GetSettlementRun(ctx context.Context, runID string) (json.RawMessage, error) {
	return p.record(ctx, "/settlement-runs/", runID, "runId", "run")
}

func (p *PhysicalPosClient) record(ctx context.Context, prefix, id, field, name string) (json.RawMessage, error) {
	segment, err := posPathID(id, field)
	if err != nil {
		return nil, err
	}
	r, err := p.client.posSend(ctx, "GET", prefix+segment, nil, nil, "")
	if err != nil {
		return nil, err
	}
	return posRecord(r, name), nil
}

func (c *Client) posList(ctx context.Context, path string, filters map[string]any, route string) (*PhysicalPosPage, error) {
	query, err := posBuildQuery(filters, route)
	if err != nil {
		return nil, err
	}
	r, err := c.posSend(ctx, "GET", path, nil, query, "")
	if err != nil {
		return nil, err
	}
	return posPage(r), nil
}

func (c *Client) posData(ctx context.Context, path string, filters map[string]any, route string) (json.RawMessage, error) {
	query, err := posBuildQuery(filters, route)
	if err != nil {
		return nil, err
	}
	r, err := c.posSend(ctx, "GET", path, nil, query, "")
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

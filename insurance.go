package lunixi

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// InsuranceClient — offer, sell and service insurance for your basket items,
// and submit leads for the types sold by request only (/api/v1/insurance,
// product INSURANCE_SERVICE).
//
// Every call is signed, reads included; a key without the product is 403
// PRODUCT_NOT_ENABLED. The key decides the environment (TEST or LIVE).
//
// Request fields and query filters are checked before anything is sent (the
// gateway rejects an unknown field with 400): an unknown field or filter, a
// missing required value, a basket sent both as paymentIntentId and as
// context.items, or a malformed Idempotency-Key returns ErrConfiguration.
// The field rules of an insurance type's own object (insured, tickets …) are
// not repeated here — a violation comes back as 422
// insurance.validation.invalid with every details.errors[{field, reason}] at
// once. Amounts are canonical minor-unit text.
//
// Idempotency-Key (8–200 printable ASCII characters, no spaces) is required
// by CreateOrder, ConfirmPayment, IssuePolicy, CancelOrder, RefundPremium and
// CreateLead, and optional ("" sends none) for CreateQuote and BindPayment.
//
// Single records are the envelope's data; lists return CursorPage; policy
// documents and the reconciliation export are bytes.
type InsuranceClient struct{ client *Client }

// ListProducts lists the insurance types offered for a payment's basket,
// read from the items sent when the payment was created (GET /products).
// Filters: currency (required), paymentIntentId, insuranceType. Without
// paymentIntentId it lists the insurance types sold without a basket item.
// An empty list means insurance is not offered; it is not an error.
func (i *InsuranceClient) ListProducts(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	query, err := insuranceQuery(filters, "listProducts", "currency")
	if err != nil {
		return nil, err
	}
	return i.read(ctx, "/products", query)
}

// SearchProducts lists the insurance types offered for basket items before
// the payment exists (POST /products, read-only): {currency,
// insuranceType?, context: {items}} with the items you will send in the
// payment.
func (i *InsuranceClient) SearchProducts(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	body, err := insuranceObject(request, insuranceProductSearchFields, "searchProducts request")
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if payload["currency"], err = currencyCode(body["currency"], "currency"); err != nil {
		return nil, err
	}
	if payload["context"], err = insuranceContext(body["context"]); err != nil {
		return nil, err
	}
	if presentValue(body["insuranceType"]) {
		if payload["insuranceType"], err = insuranceText(body["insuranceType"], "insuranceType", 0); err != nil {
			return nil, err
		}
	}
	return i.write(ctx, "/products", payload, "")
}

// ListReferenceValues lists the Lunixi reference codes of one kind
// (InsuranceReferenceKind*), for the type fields that take a code. Filters:
// parentCode, limit, cursor.
func (i *InsuranceClient) ListReferenceValues(ctx context.Context, kind string, filters map[string]any) (*CursorPage, error) {
	if !slices.Contains(InsuranceReferenceKinds(), kind) {
		return nil, invalid("reference kind must be one of " + strings.Join(InsuranceReferenceKinds(), ", "))
	}
	return i.list(ctx, "/reference/"+kind, filters, "listReferenceValues")
}

// IterateReferenceValues yields every reference code of one kind.
func (i *InsuranceClient) IterateReferenceValues(ctx context.Context, kind string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, func(ctx context.Context, page map[string]any) (*CursorPage, error) {
		return i.ListReferenceValues(ctx, kind, page)
	})
}

// CreateQuote prices an insurance type (HTTP 201). The basket is either a
// payment's recorded basket (paymentIntentId) or the items you will send in
// the payment (context.items), not both; send neither for an insurance type
// sold without a basket item (the insured value is then in the type
// object). Every option of the answer has its own quoteId, terms and
// expiresAt; a company the request could not be priced with is in
// excludedOptions.
//
// The Idempotency-Key is optional ("" sends none) and only correlates
// traces: every call is a new, time-limited price.
func (i *InsuranceClient) CreateQuote(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceOptionalKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	body, err := insuranceObject(request, insuranceQuoteFields, "createQuote request")
	if err != nil {
		return nil, err
	}
	hasPayment, hasItems := presentValue(body["paymentIntentId"]), presentValue(body["context"])
	if hasPayment && hasItems {
		return nil, invalid("send the basket either as 'paymentIntentId' (a payment that exists) or as 'context.items' (before the payment), not both. Send neither for an insurance type sold without a basket item")
	}
	payload := compactNil(body)
	delete(payload, "paymentIntentId")
	delete(payload, "context")
	delete(payload, "merchantOrderId")
	if payload["insuranceType"], err = insuranceText(body["insuranceType"], "insuranceType", 0); err != nil {
		return nil, err
	}
	if payload["currency"], err = currencyCode(body["currency"], "currency"); err != nil {
		return nil, err
	}
	if hasPayment {
		if payload["paymentIntentId"], err = insuranceUUID(body["paymentIntentId"], "paymentIntentId"); err != nil {
			return nil, err
		}
	}
	if hasItems {
		if payload["context"], err = insuranceContext(body["context"]); err != nil {
			return nil, err
		}
	}
	if presentValue(body["merchantOrderId"]) {
		if payload["merchantOrderId"], err = insuranceText(body["merchantOrderId"], "merchantOrderId", insuranceTextMax); err != nil {
			return nil, err
		}
	}
	return i.write(ctx, "/quotes", payload, key)
}

// GetQuote reads one quote option; 404 insurance.quote.not_found for an
// unknown or another merchant's quote.
func (i *InsuranceClient) GetQuote(ctx context.Context, quoteID string) (json.RawMessage, error) {
	return i.record(ctx, "/quotes/:quoteId", "quoteId", quoteID)
}

// CreateOrder turns a quote option into an order (HTTP 201) and records the
// buyer's consent: quoteId, optional merchantOrderId, consent {termsVersion,
// termsSha256, acceptedAt} — the option's terms back unchanged plus the
// moment the buyer accepted them (ConsentFromQuote builds it). No provider
// call and no money movement. The key is required: the same key with the
// same body returns the recorded order, with another body 409
// insurance.idempotency.conflict.
func (i *InsuranceClient) CreateOrder(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceRequiredKey("insurance order create", idempotencyKey)
	if err != nil {
		return nil, err
	}
	body, err := insuranceObject(request, insuranceOrderFields, "createOrder request")
	if err != nil {
		return nil, err
	}
	if body["consent"] == nil {
		return nil, invalid("consent must be an object")
	}
	consent, err := insuranceObject(body["consent"], insuranceConsentFields, "consent")
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if payload["quoteId"], err = insuranceText(body["quoteId"], "quoteId", 0); err != nil {
		return nil, err
	}
	if presentValue(body["merchantOrderId"]) {
		if payload["merchantOrderId"], err = insuranceText(body["merchantOrderId"], "merchantOrderId", insuranceTextMax); err != nil {
			return nil, err
		}
	}
	terms := map[string]any{}
	if terms["termsVersion"], err = insuranceText(consent["termsVersion"], "consent.termsVersion", 0); err != nil {
		return nil, err
	}
	if terms["termsSha256"], err = insuranceText(consent["termsSha256"], "consent.termsSha256", 0); err != nil {
		return nil, err
	}
	if terms["acceptedAt"], err = timeText(consent["acceptedAt"], "consent.acceptedAt"); err != nil {
		return nil, err
	}
	payload["consent"] = terms
	return i.write(ctx, "/orders", payload, key)
}

// ListOrders lists your insurance orders, newest first. Filters: limit,
// cursor, paymentIntentId, merchantOrderId.
func (i *InsuranceClient) ListOrders(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return i.list(ctx, "/orders", filters, "listOrders")
}

// IterateOrders yields every order matching the filters, page by page.
func (i *InsuranceClient) IterateOrders(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, i.ListOrders)
}

// GetOrder reads one order; 404 insurance.order.not_found for an unknown or
// another merchant's order.
func (i *InsuranceClient) GetOrder(ctx context.Context, orderID string) (json.RawMessage, error) {
	return i.record(ctx, "/orders/:orderId", "orderId", orderID)
}

// BindPayment binds an order to the Lunixi payment that collects its
// premium. When you collect it (premiumCollectedBy MERCHANT) the payment
// basket must hold the premium item (PremiumBasketItem); when the provider
// collects it, it must not. Binding the same payment again returns the
// order. The key is optional ("" sends none).
func (i *InsuranceClient) BindPayment(ctx context.Context, orderID, paymentIntentID, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceOptionalKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	id, err := insuranceUUID(paymentIntentID, "paymentIntentId")
	if err != nil {
		return nil, err
	}
	return i.orderAction(ctx, orderID, "bind-payment", map[string]any{"paymentIntentId": id}, key)
}

// ConfirmPayment records a premium you collected outside Lunixi:
// {externalPaymentId, paidAmount, paidCurrency, paidAt, paymentProvider?}.
// paidAmount and paidCurrency must equal the order's premium. The key is
// required; repeating the same externalPaymentId returns the order.
func (i *InsuranceClient) ConfirmPayment(ctx context.Context, orderID string, payment map[string]any, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceRequiredKey("insurance payment confirmation", idempotencyKey)
	if err != nil {
		return nil, err
	}
	body, err := insuranceObject(payment, insuranceExternalPaymentFields, "payment")
	if err != nil {
		return nil, err
	}
	record := map[string]any{}
	if record["externalPaymentId"], err = insuranceText(body["externalPaymentId"], "payment.externalPaymentId", insuranceTextMax); err != nil {
		return nil, err
	}
	if record["paidAmount"], err = insurancePositiveMinor(body["paidAmount"], "payment.paidAmount"); err != nil {
		return nil, err
	}
	if record["paidCurrency"], err = currencyCode(body["paidCurrency"], "payment.paidCurrency"); err != nil {
		return nil, err
	}
	if record["paidAt"], err = timeText(body["paidAt"], "payment.paidAt"); err != nil {
		return nil, err
	}
	if presentValue(body["paymentProvider"]) {
		if record["paymentProvider"], err = insuranceText(body["paymentProvider"], "payment.paymentProvider", insuranceShortTextMax); err != nil {
			return nil, err
		}
	}
	return i.orderAction(ctx, orderID, "confirm-payment", map[string]any{"payment": record}, key)
}

// IssuePolicy requests the policy of an order whose premium you collected
// outside Lunixi (policyStatus PENDING; the result arrives by webhook). With
// a Lunixi payment the policy is requested on its own once the premium is
// collected. 412 insurance.premium.not_captured before the premium is
// collected. The key is required.
func (i *InsuranceClient) IssuePolicy(ctx context.Context, orderID, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceRequiredKey("insurance policy issue", idempotencyKey)
	if err != nil {
		return nil, err
	}
	return i.orderAction(ctx, orderID, "issue-policy", nil, key)
}

// CancelOrder opens a cancellation at the provider for every issued policy
// of the order (policyStatus CANCEL_PENDING; insurance.policy.cancelled
// confirms it). Cancelling does not refund the premium: call RefundPremium
// after it. The key is required.
func (i *InsuranceClient) CancelOrder(ctx context.Context, orderID, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceRequiredKey("insurance order cancel", idempotencyKey)
	if err != nil {
		return nil, err
	}
	return i.orderAction(ctx, orderID, "cancel", nil, key)
}

// RefundPremium refunds the premium. With a Lunixi payment pass nil: the
// premium is refunded from that payment (refundStatus PENDING, then
// insurance.refund.completed). For a premium collected outside Lunixi pass
// the refund you made: {externalRefundId, refundedAt}. An in-force policy
// must be cancelled first (409, policy_active). The key is required.
func (i *InsuranceClient) RefundPremium(ctx context.Context, orderID string, externalRefund map[string]any, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceRequiredKey("insurance premium refund", idempotencyKey)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if len(externalRefund) > 0 {
		refund, err := insuranceObject(externalRefund, insuranceExternalRefundFields, "externalRefund")
		if err != nil {
			return nil, err
		}
		record := map[string]any{}
		if record["externalRefundId"], err = insuranceText(refund["externalRefundId"], "externalRefund.externalRefundId", insuranceTextMax); err != nil {
			return nil, err
		}
		if record["refundedAt"], err = timeText(refund["refundedAt"], "externalRefund.refundedAt"); err != nil {
			return nil, err
		}
		payload["externalRefund"] = record
	}
	return i.orderAction(ctx, orderID, "refund-premium", payload, key)
}

// CreateLead submits a lead (HTTP 201) for an insurance type sold by request
// only: the provider gives no quote, so there is no premium, order, policy
// or document. policyholder, the type object and an optional note (sent as
// given, at most 1000 characters) follow the type's lead schema. The
// answer's status is SUBMITTED (followed by insurance.lead.submitted) or
// UNKNOWN (the lead is not sent again). A provider refusal is 422
// insurance.lead.rejected; a type or connection without leads is 412
// insurance.type.not_available. The key is required: the same key with the
// same body returns the same lead, with another body 409
// insurance.idempotency.conflict.
func (i *InsuranceClient) CreateLead(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	key, err := insuranceRequiredKey("insurance lead submit", idempotencyKey)
	if err != nil {
		return nil, err
	}
	body, err := insuranceObject(request, insuranceLeadFields, "createLead request")
	if err != nil {
		return nil, err
	}
	payload := compactNil(body)
	if payload["insuranceType"], err = insuranceText(body["insuranceType"], "insuranceType", insuranceShortTextMax); err != nil {
		return nil, err
	}
	delete(payload, "note")
	if presentValue(body["note"]) {
		note, ok := body["note"].(string)
		if !ok {
			return nil, invalid("'note' must be a string")
		}
		if utf8.RuneCountInString(note) > InsuranceLeadNoteMaxLength {
			return nil, invalid("'note' must be at most " + strconv.Itoa(InsuranceLeadNoteMaxLength) + " characters")
		}
		payload["note"] = note
	}
	return i.write(ctx, "/leads", payload, key)
}

// ListLeads lists your leads, newest first; personal data is masked in
// requestMasked. Filters: limit, cursor, insuranceType, status
// (InsuranceLeadStatus*).
func (i *InsuranceClient) ListLeads(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return i.list(ctx, "/leads", filters, "listLeads")
}

// IterateLeads yields every lead matching the filters, page by page.
func (i *InsuranceClient) IterateLeads(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, i.ListLeads)
}

// GetLead reads one lead; 404 insurance.lead.not_found for an unknown or
// another merchant's lead.
func (i *InsuranceClient) GetLead(ctx context.Context, leadID string) (json.RawMessage, error) {
	return i.record(ctx, "/leads/:leadId", "leadId", leadID)
}

// GetPolicyDocument returns the policy PDF in Response.Bytes. It is read
// from the provider on every call and is available while the policy is in
// force (ISSUED, CANCEL_PENDING); 503 insurance.document.unavailable is
// temporary — retry later. Error answers are read as the JSON envelope.
func (i *InsuranceClient) GetPolicyDocument(ctx context.Context, policyID string) (*Response, error) {
	return i.policyFile(ctx, policyID, "document")
}

// GetPolicyReceipt returns the provider's receipt for the policy premium (a
// PDF in Response.Bytes). Only for products that issue one; otherwise 412
// insurance.document.not_supported. Same availability as GetPolicyDocument.
func (i *InsuranceClient) GetPolicyReceipt(ctx context.Context, policyID string) (*Response, error) {
	return i.policyFile(ctx, policyID, "receipt")
}

// GetPolicyCertificate returns the provider's certificate of the policy (for
// example the Green Card of a motor policy; a PDF in Response.Bytes). Only
// for products that issue one; otherwise 412
// insurance.document.not_supported. Same availability as GetPolicyDocument.
func (i *InsuranceClient) GetPolicyCertificate(ctx context.Context, policyID string) (*Response, error) {
	return i.policyFile(ctx, policyID, "certificate")
}

// GetReconciliationSummary returns reconciliation totals grouped by
// provider connection, currency and premiumCollectedBy. Filters: from
// (inclusive) and to (exclusive), required, on the order creation time — a
// time.Time or ISO-8601 text with a time zone, at most 366 days apart; also
// connectionId, insuranceType, currency.
func (i *InsuranceClient) GetReconciliationSummary(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	query, err := insuranceQuery(filters, "getReconciliationSummary", "from", "to")
	if err != nil {
		return nil, err
	}
	return i.read(ctx, "/reconciliation/summary", query)
}

// ListReconciliationOrders lists the orders and policies behind the summary
// for the same filters, newest first (plus limit, cursor).
func (i *InsuranceClient) ListReconciliationOrders(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	query, err := insuranceQuery(filters, "listReconciliationOrders", "from", "to")
	if err != nil {
		return nil, err
	}
	r, err := i.client.Do(ctx, Request{Method: "GET", Path: insuranceBase + "/reconciliation/orders", Query: query})
	if err != nil {
		return nil, err
	}
	return envelopePage(r), nil
}

// IterateReconciliationOrders yields every reconciliation order, page by
// page.
func (i *InsuranceClient) IterateReconciliationOrders(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, i.ListReconciliationOrders)
}

// InsuranceReconciliationExport is the reconciliation report: CSV (UTF-8
// with BOM, one line per policy, amounts in minor units). A file holds at
// most InsuranceExportOrderCap orders; when it was cut, Truncated is true and
// NextCursor continues it (pass it as the cursor filter). Aborted is true
// when the file ends with the gateway's abort line: it is incomplete,
// request it again.
type InsuranceReconciliationExport struct {
	ContentType string
	Bytes       []byte
	Truncated   bool
	NextCursor  string
	Aborted     bool
	RequestID   string
}

// ExportReconciliation downloads the reconciliation report for the
// GetReconciliationSummary filters (plus cursor).
func (i *InsuranceClient) ExportReconciliation(ctx context.Context, filters map[string]any) (*InsuranceReconciliationExport, error) {
	query, err := insuranceQuery(filters, "exportReconciliation", "from", "to")
	if err != nil {
		return nil, err
	}
	r, err := i.client.do(ctx, Request{Method: "GET", Path: insuranceBase + "/reconciliation/export", Query: query}, insuranceCSVAccept)
	if err != nil {
		return nil, err
	}
	export := &InsuranceReconciliationExport{ContentType: r.ContentType, Bytes: r.Bytes, RequestID: r.RequestID}
	last := lastLine(r.Bytes)
	if match := insuranceTruncatedLine.FindStringSubmatch(last); match != nil {
		export.Truncated, export.NextCursor = true, match[2]
	}
	export.Aborted = insuranceAbortedLine.MatchString(last)
	return export, nil
}

// ConsentFromQuote builds CreateOrder's consent from a quote option (the
// data of GetQuote, or one of CreateQuote's options): the option's terms
// unchanged and the moment the buyer accepted them (not in the future).
func (i *InsuranceClient) ConsentFromQuote(option any, acceptedAt time.Time) (map[string]any, error) {
	source, err := plainObject(option, "quote option")
	if err != nil {
		return nil, err
	}
	terms, ok := source["terms"].(map[string]any)
	if !ok {
		return nil, invalid("the quote option has no 'terms': pass an option returned by CreateQuote or GetQuote")
	}
	consent := map[string]any{}
	if consent["termsVersion"], err = insuranceText(terms["version"], "terms.version", 0); err != nil {
		return nil, err
	}
	if consent["termsSha256"], err = insuranceText(terms["sha256"], "terms.sha256", 0); err != nil {
		return nil, err
	}
	if consent["acceptedAt"], err = timeText(acceptedAt, "acceptedAt"); err != nil {
		return nil, err
	}
	return consent, nil
}

// PremiumBasketItem is the premium basket item for a payment that collects
// the premium of an order (premiumCollectedBy MERCHANT; the data of
// GetOrder or CreateOrder): ID = orderId, ItemType VIRTUAL, Price =
// premiumAmount. Add it to the payment's basket next to the insured items
// and add the premium to the payment amount; for insurance sold without a
// basket item it can be the only item. It carries no insurance field. A
// PROVIDER order must not have one. category2 "" leaves it out.
func (i *InsuranceClient) PremiumBasketItem(order any, name, category1, category2 string) (BasketItem, error) {
	source, err := plainObject(order, "order")
	if err != nil {
		return BasketItem{}, err
	}
	if source["premiumCollectedBy"] != InsurancePremiumCollectedByMerchant {
		return BasketItem{}, invalid("only an order whose premium you collect (premiumCollectedBy MERCHANT) has a premium basket item")
	}
	var item BasketItem
	if item.ID, err = insuranceText(source["orderId"], "order.orderId", 0); err != nil {
		return BasketItem{}, err
	}
	if item.Price, err = insurancePositiveMinor(source["premiumAmount"], "order.premiumAmount"); err != nil {
		return BasketItem{}, err
	}
	if item.Name, err = insuranceText(name, "name", 0); err != nil {
		return BasketItem{}, err
	}
	if item.Category1, err = insuranceText(category1, "category1", 0); err != nil {
		return BasketItem{}, err
	}
	if category2 != "" {
		if item.Category2, err = insuranceText(category2, "category2", 0); err != nil {
			return BasketItem{}, err
		}
	}
	item.ItemType = "VIRTUAL"
	return item, nil
}

// ── Sending ──────────────────────────────────────────────────────────────

var (
	insuranceTruncatedLine = regexp.MustCompile(`^# truncated=true orderCap=(\d+) nextCursor=(\S+)\s*$`)
	insuranceAbortedLine   = regexp.MustCompile(`^# export-aborted=true\s*$`)
)

func (i *InsuranceClient) read(ctx context.Context, path string, query url.Values) (json.RawMessage, error) {
	r, err := i.client.Do(ctx, Request{Method: "GET", Path: insuranceBase + path, Query: query})
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (i *InsuranceClient) write(ctx context.Context, path string, body any, key string) (json.RawMessage, error) {
	request := Request{Method: "POST", Path: insuranceBase + path, IdempotencyKey: key}
	if body != nil {
		request.Body = body
	}
	r, err := i.client.Do(ctx, request)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (i *InsuranceClient) record(ctx context.Context, template, field, id string) (json.RawMessage, error) {
	path, err := fillRoutePath(template, map[string]string{field: id})
	if err != nil {
		return nil, err
	}
	return i.read(ctx, path, nil)
}

func (i *InsuranceClient) list(ctx context.Context, path string, filters map[string]any, route string) (*CursorPage, error) {
	query, err := insuranceQuery(filters, route)
	if err != nil {
		return nil, err
	}
	r, err := i.client.Do(ctx, Request{Method: "GET", Path: insuranceBase + path, Query: query})
	if err != nil {
		return nil, err
	}
	return envelopePage(r), nil
}

// orderAction posts to /orders/{orderId}/{action}; a nil body sends none.
func (i *InsuranceClient) orderAction(ctx context.Context, orderID, action string, body map[string]any, key string) (json.RawMessage, error) {
	path, err := fillRoutePath("/orders/:orderId/"+action, map[string]string{"orderId": orderID})
	if err != nil {
		return nil, err
	}
	if body == nil {
		return i.write(ctx, path, nil, key)
	}
	return i.write(ctx, path, body, key)
}

func (i *InsuranceClient) policyFile(ctx context.Context, policyID, file string) (*Response, error) {
	path, err := fillRoutePath(insuranceBase+"/policies/:policyId/"+file, map[string]string{"policyId": policyID})
	if err != nil {
		return nil, err
	}
	return i.client.do(ctx, Request{Method: "GET", Path: path}, insurancePDFAccept)
}

// ── Checks ───────────────────────────────────────────────────────────────

// insuranceQuery checks the filters of a read; required names the filters
// the read cannot do without.
func insuranceQuery(filters map[string]any, route string, required ...string) (url.Values, error) {
	allowed := insuranceFilters[route]
	if err := unknownFilters(filters, allowed, "insurance"); err != nil {
		return nil, err
	}
	query := url.Values{}
	for _, key := range allowed {
		value := filters[key]
		if emptyFilter(value) {
			continue
		}
		text, err := insuranceFilterValue(key, value)
		if err != nil {
			return nil, err
		}
		query.Set(key, text)
	}
	var missing []string
	for _, key := range required {
		if !query.Has(key) {
			missing = append(missing, "'"+key+"'")
		}
	}
	if len(missing) > 0 {
		return nil, invalid(route + " requires " + strings.Join(missing, " and "))
	}
	return query, nil
}

func insuranceFilterValue(key string, value any) (string, error) {
	switch key {
	case "limit":
		return pageLimit(value, InsuranceMaxLimit)
	case "cursor":
		return pageCursor(value, insuranceCursorMax)
	case "currency":
		return currencyCode(value, key)
	case "paymentIntentId", "connectionId":
		return insuranceUUID(value, key)
	case "from", "to":
		return timeText(value, key)
	case "insuranceType", "parentCode":
		return insuranceText(value, key, insuranceShortTextMax)
	case "merchantOrderId":
		return insuranceText(value, key, insuranceTextMax)
	case "status":
		return oneValue(key, value, InsuranceLeadStatuses())
	}
	return "", invalid("unknown insurance filter '" + key + "'")
}

// insuranceObject reads an object and refuses members outside allowed.
func insuranceObject(value any, allowed []string, label string) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	object, err := plainObject(value, label)
	if err != nil {
		return nil, err
	}
	var unknown []string
	for key := range object {
		if !slices.Contains(allowed, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return nil, invalid("unknown " + label + " field(s): " + strings.Join(unknown, ", ") + ". Allowed: " + strings.Join(allowed, ", ") + ". The gateway rejects unknown fields with 400")
	}
	return object, nil
}

// insuranceContext is context {items}: 1–100 basket items, the same items the
// payment carries.
func insuranceContext(value any) (map[string]any, error) {
	if !presentValue(value) {
		return nil, invalid("'context.items' is required")
	}
	object, err := insuranceObject(value, []string{"items"}, "context")
	if err != nil {
		return nil, err
	}
	items, ok := object["items"].([]any)
	if !ok || len(items) < 1 || len(items) > InsuranceMaxContextItems {
		return nil, invalid("'context.items' must hold 1 to " + strconv.Itoa(InsuranceMaxContextItems) + " basket items")
	}
	return map[string]any{"items": items}, nil
}

// insuranceText is a non-empty string, sent trimmed; max 0 means no limit.
func insuranceText(value any, field string, max int) (string, error) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return "", invalid(field + " is required")
	}
	if max > 0 && utf8.RuneCountInString(text) > max {
		return "", invalid(field + " must be at most " + strconv.Itoa(max) + " characters")
	}
	return text, nil
}

func insuranceUUID(value any, field string) (string, error) {
	text, _ := value.(string)
	text = strings.TrimSpace(text)
	if !uuidPattern.MatchString(text) {
		return "", invalid("'" + field + "' must be a Lunixi id (UUID)")
	}
	return text, nil
}

func insurancePositiveMinor(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok || !validMinor(text, true, false) {
		return "", invalid(field + " must be a canonical string in minor units")
	}
	return text, nil
}

func insuranceOptionalKey(key string) (string, error) {
	if key == "" {
		return "", nil
	}
	if !validPrintableKey(key) {
		return "", invalid("the insurance Idempotency-Key must be 8-200 printable ASCII characters without spaces")
	}
	return key, nil
}

func insuranceRequiredKey(operation, key string) (string, error) {
	if key == "" {
		return "", invalid("a stable Idempotency-Key is required for " + operation + ". Reuse the same key when retrying the same operation")
	}
	return insuranceOptionalKey(key)
}

// lastLine is the last non-empty line of a text file.
func lastLine(data []byte) string {
	lines := bytes.Split(data, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSuffix(string(lines[i]), "\r")
		if line != "" {
			return line
		}
	}
	return ""
}

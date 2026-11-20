package lunixi

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Status of a split payment (status).
const (
	SplitPaymentStatusComposing              = "COMPOSING"
	SplitPaymentStatusAuthorizing            = "AUTHORIZING"
	SplitPaymentStatusAwaiting3DS            = "AWAITING_3DS"
	SplitPaymentStatusAwaitingAction         = "AWAITING_ACTION"
	SplitPaymentStatusAwaitingCreditApproval = "AWAITING_CREDIT_APPROVAL"
	SplitPaymentStatusAwaitingReplacement    = "AWAITING_REPLACEMENT_CARD"
	SplitPaymentStatusAuthorized             = "AUTHORIZED"
	SplitPaymentStatusCapturing              = "CAPTURING"
	// SplitPaymentStatusCompleted: every leg was charged.
	SplitPaymentStatusCompleted    = "COMPLETED"
	SplitPaymentStatusCompensating = "COMPENSATING"
	SplitPaymentStatusVoided       = "VOIDED"
	SplitPaymentStatusFailed       = "FAILED"
	// SplitPaymentStatusReconciliationRequired: the non-reversible leg was
	// charged but the rest could not be collected; you decide on delivery.
	SplitPaymentStatusReconciliationRequired = "RECONCILIATION_REQUIRED"
)

// Webhook event types of a split payment. The result of the main payment
// arrives as payment.captured / payment.failed with settlementMode SPLIT and
// legs[]; deliver the order on that event.
const (
	SplitPaymentEventCompleted    = "payment.split.completed"
	SplitPaymentEventVoided       = "payment.split.voided"
	SplitPaymentEventCompensating = "payment.split.compensating"
	// SplitPaymentEventOutcomePending: the non-reversible leg's result is being
	// confirmed; card holds are kept until then.
	SplitPaymentEventOutcomePending = "payment.split.outcome_pending"
	// SplitPaymentEventReconciliationRequired: the order could not be fully
	// collected (shortfallAmountMinor, reason).
	SplitPaymentEventReconciliationRequired = "payment.split.reconciliation_required"
	// SplitPaymentEventLegRefundPending: a charged leg is being refunded and
	// retried until it succeeds.
	SplitPaymentEventLegRefundPending = "payment.split.leg_refund_pending"
)

// SplitLegEventsSubscription is put into a webhook endpoint's event types to
// also receive the events of each leg. It is not an event type and payment.*
// does not include it.
const SplitLegEventsSubscription = "payment.split.leg_events"

var splitLegBlockFields = []string{"mobileBilling", "wallet", "promissoryNote"}

var splitPaymentFields = []string{"totalAmount", "currency", "ownerRef", "legs", "orderId", "ownerScope", "callbackUrl", "buyer", "billingAddress", "shippingAddress", "basketItems"}

var splitPaymentLegFields = []string{"paymentMethod", "cardToken", "amount", "force3D", "installment", "installmentQuoteToken", "mobileBilling", "wallet", "promissoryNote"}

var splitCardLegOnlyFields = []string{"cardToken", "force3D", "installmentQuoteToken"}

var listSplitPaymentsFilters = []string{"status", "currency", "createdFrom", "createdTo", "orderId", "ownerRef", "limit", "after"}

var splitPaymentStatuses = []string{
	SplitPaymentStatusComposing, SplitPaymentStatusAuthorizing, SplitPaymentStatusAwaiting3DS, SplitPaymentStatusAwaitingAction,
	SplitPaymentStatusAwaitingCreditApproval, SplitPaymentStatusAwaitingReplacement, SplitPaymentStatusAuthorized, SplitPaymentStatusCapturing,
	SplitPaymentStatusCompleted, SplitPaymentStatusCompensating, SplitPaymentStatusVoided, SplitPaymentStatusFailed, SplitPaymentStatusReconciliationRequired,
}

// SplitLegBlockFields returns the method blocks a non-card split leg can
// carry. Meal card, open banking, shopping loan and bank transfer cannot be
// split legs. The returned slice is a fresh copy.
func SplitLegBlockFields() []string { return slices.Clone(splitLegBlockFields) }

// SplitPaymentFields returns the fields of a split payment body: the amounts
// and legs plus the order details (buyer, billingAddress, shippingAddress,
// basketItems) every leg is sent to its provider with. The returned slice is
// a fresh copy.
func SplitPaymentFields() []string { return slices.Clone(splitPaymentFields) }

// SplitPaymentLegFields returns the fields of one split leg. The returned
// slice is a fresh copy.
func SplitPaymentLegFields() []string { return slices.Clone(splitPaymentLegFields) }

// ListSplitPaymentsFilters returns the filter names of GET /payments/split.
// The returned slice is a fresh copy.
func ListSplitPaymentsFilters() []string { return slices.Clone(listSplitPaymentsFilters) }

// validateSplitLegs applies the leg rules to a raw split body: known body
// fields (SplitPaymentFields) and known leg
// fields only; a card leg (paymentMethod empty or CARD) carries no method
// block; a non-card leg is a mobileBilling, wallet or promissoryNote method
// with exactly its own block, a providerCode and no card fields. A
// promissory-note leg gives noteCount, not a schedule.
func validateSplitLegs(raw []byte) error {
	value, err := decodeGeneric(raw)
	if err != nil {
		return err
	}
	body, _ := value.(map[string]any)
	for _, key := range sortedKeys(body) {
		if !slices.Contains(splitPaymentFields, key) {
			return invalid("unknown split payment field: " + key)
		}
	}
	legs, _ := body["legs"].([]any)
	for index, entry := range legs {
		path := "legs[" + strconv.Itoa(index) + "]"
		leg, ok := entry.(map[string]any)
		if !ok {
			return invalid(path + " must be an object")
		}
		for _, key := range sortedKeys(leg) {
			if slices.Contains(methodBlockFields, key) && !slices.Contains(splitLegBlockFields, key) {
				return invalid(path + " cannot carry the " + key + " block; a split leg carries only mobileBilling, wallet or promissoryNote")
			}
			if !slices.Contains(splitPaymentLegFields, key) {
				return invalid("unknown " + path + " field: " + key)
			}
		}
		method, _ := leg["paymentMethod"].(string)
		if present(leg, "paymentMethod") {
			if _, isText := leg["paymentMethod"].(string); !isText {
				return invalid(path + ".paymentMethod must be a payment method code")
			}
		}
		if method == "" || method == PaymentMethodCard {
			for _, block := range splitLegBlockFields {
				if present(leg, block) {
					return invalid(path + " is a card leg and cannot carry the " + block + " block")
				}
			}
			continue
		}
		if err = validateMethodLeg(leg, method, path); err != nil {
			return err
		}
	}
	return nil
}

func validateMethodLeg(leg map[string]any, method, path string) error {
	code, err := NormalizePaymentMethod(method)
	if err != nil {
		return err
	}
	blockName, _ := BlockNameOf(code)
	if !slices.Contains(splitLegBlockFields, blockName) {
		return invalid(path + ": paymentMethod " + code + " cannot be a split payment leg; a non-card leg is mobileBilling, wallet or promissoryNote")
	}
	for _, field := range splitCardLegOnlyFields {
		if present(leg, field) {
			return invalid(path + ": " + field + " can only be sent with a card leg")
		}
	}
	if present(leg, "installment") {
		if number, ok := leg["installment"].(json.Number); !ok || number.String() != "1" {
			return invalid(path + ": installment can only be sent with a card leg")
		}
	}
	for _, other := range splitLegBlockFields {
		if other != blockName && present(leg, other) {
			return invalid(path + ": paymentMethod " + code + " takes only the " + blockName + " block; " + other + " must not be sent")
		}
	}
	block, ok := leg[blockName].(map[string]any)
	if !ok {
		return invalid(path + ": paymentMethod " + code + " requires the " + blockName + " block")
	}
	if !present(block, "providerCode") {
		return invalid(path + "." + blockName + ".providerCode is required in a split leg (a provider with splitEligible true from ListPaymentMethods)")
	}
	if err = validateRawBlock(block, path+"."+blockName); err != nil {
		return err
	}
	if code == PaymentMethodPromissoryNote {
		if present(block, "schedule") {
			return invalid(path + "." + blockName + ".schedule cannot be sent in a split leg; give the number of notes with noteCount")
		}
		if _, err = noteCountText(block["noteCount"], path+"."+blockName+".noteCount"); err != nil {
			return err
		}
	}
	return nil
}

// SplitListFilters narrows ListSplits. Empty fields are not sent. Status
// (SplitPaymentStatus* values) and Currency take 1–20 values each and are sent
// as repeated parameters; CreatedFrom/CreatedTo are ISO 8601 date-times; Limit,
// when set, is 1–100; After is the pageInfo.endCursor of the previous page.
type SplitListFilters struct {
	Status      []string
	Currency    []string
	CreatedFrom string
	CreatedTo   string
	OrderID     string
	OwnerRef    string
	Limit       *int
	After       string
}

var iso8601Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d{1,9})?)?(Z|[+-]\d{2}:?\d{2})?)?$`)

func boundedText(value, name string, max int) error {
	if strings.TrimSpace(value) == "" || utf16Length(value) > max {
		return invalid(name + " must be a non-empty string of at most " + strconv.Itoa(max) + " characters")
	}
	return nil
}

func (f SplitListFilters) query() (url.Values, error) {
	query := url.Values{}
	if f.Status != nil {
		if len(f.Status) == 0 || len(f.Status) > 20 {
			return nil, invalid("status takes 1 to 20 values")
		}
		for _, status := range f.Status {
			if !slices.Contains(splitPaymentStatuses, status) {
				return nil, invalid("status must be a SplitPaymentStatus value")
			}
		}
		query["status"] = slices.Clone(f.Status)
	}
	if f.Currency != nil {
		if len(f.Currency) == 0 || len(f.Currency) > 20 {
			return nil, invalid("currency takes 1 to 20 values")
		}
		for _, currency := range f.Currency {
			if !currencyPattern.MatchString(currency) {
				return nil, invalid("currency must be an ISO 4217 code")
			}
		}
		query["currency"] = slices.Clone(f.Currency)
	}
	for name, value := range map[string]string{"createdFrom": f.CreatedFrom, "createdTo": f.CreatedTo} {
		if value == "" {
			continue
		}
		if !iso8601Pattern.MatchString(value) {
			return nil, invalid(name + " must be an ISO 8601 date-time")
		}
		query.Set(name, value)
	}
	for _, text := range []struct {
		name, value string
		max         int
	}{{"orderId", f.OrderID, 200}, {"ownerRef", f.OwnerRef, 200}, {"after", f.After, 2048}} {
		if text.value == "" {
			continue
		}
		if err := boundedText(text.value, text.name, text.max); err != nil {
			return nil, err
		}
		query.Set(text.name, text.value)
	}
	if f.Limit != nil {
		if *f.Limit < 1 || *f.Limit > 100 {
			return nil, invalid("limit must be an integer from 1 to 100")
		}
		query.Set("limit", strconv.Itoa(*f.Limit))
	}
	if len(query) == 0 {
		return nil, nil
	}
	return query, nil
}

// ListSplits lists split payments, newest first (GET /payments/split), with a
// forward cursor in PageInfo.
func (p *PaymentClient) ListSplits(ctx context.Context, filters SplitListFilters) (*Response, error) {
	query, err := filters.query()
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/split", Query: query})
}

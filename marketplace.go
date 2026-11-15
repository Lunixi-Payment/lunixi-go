package lunixi

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// MarketplaceClient — split charges across sub-dealers, the sub-dealer
// lifecycle, commissions, payouts and the audit log, reached as
// Client.Marketplace.
//
// Split charges (/api/v1/payments/marketplace/*) are signed per request and
// take an optional Idempotency-Key (16–128 printable ASCII characters, as
// every payment write). A split is aggregate per seller: group the basket
// by vendor and pass one leg per sub-dealer (MarketplaceSeller); the legs
// must add up to amount. Dealer, commission, payout and audit routes
// (/api/v1/marketplace/*) are bearer-only and take no key.
//
// Card data (card number, expiry, security code) is sent as given and never
// logged or put in an error message; a direct card charge is PCI DSS SAQ-D
// scope, so most marketplaces use CreateCheckoutIntent (hosted form).
//
// Amounts are canonical minor-unit strings. Entity calls return the
// envelope's data; the charges return the whole Response; lists return
// CursorPage.
type MarketplaceClient struct{ client *Client }

// ── Split charges ────────────────────────────────────────────────────────

// CreateCheckoutIntent creates a hosted-form split checkout (POST
// /payments/marketplace/intents, the route of
// Payments.CreateMarketplaceIntent): the intent plus the seller legs.
// multiSellerPolicy "" is MarketplaceMultiSellerPolicyAllOrNothing;
// idempotencyKey "" sends none. An answer without a token or form (a seller
// not eligible under ALL_OR_NOTHING) is an *APIError instead of leaving the
// buyer on an empty page.
func (m *MarketplaceClient) CreateCheckoutIntent(ctx context.Context, request IntentRequest, sellers []MarketplaceSeller, multiSellerPolicy, idempotencyKey string) (*Response, error) {
	policy, err := multiSellerPolicyOf(multiSellerPolicy)
	if err != nil {
		return nil, err
	}
	intent := MarketplaceIntentRequest{IntentRequest: request, Sellers: sellers, MultiSellerPolicy: policy}
	if err = intent.validate(); err != nil {
		return nil, err
	}
	if intent.Sellers, err = sellerLegs(sellers, request.Amount); err != nil {
		return nil, err
	}
	if intent.EnabledInstallments != nil {
		intent.EnabledInstallments = slices.Sorted(slices.Values(intent.EnabledInstallments))
	}
	body, err := plainObject(intent, "createCheckoutIntent request")
	if err != nil {
		return nil, err
	}
	r, err := m.call(ctx, "createCheckoutIntent", nil, nil, body, idempotencyKey)
	if err != nil {
		return nil, err
	}
	answer := objectFields(mergedRoot(r))
	if stringField(answer, "token") == "" && stringField(answer, "checkoutFormContent") == "" {
		code := stringField(answer, "code")
		if !safeCode.MatchString(code) {
			code = "MARKETPLACE_INTENT_WITHOUT_TOKEN"
		}
		return nil, &APIError{StatusCode: r.StatusCode, Code: code, RequestID: r.RequestID}
	}
	return r, nil
}

// ChargeMarketplaceCard sends a direct split card charge from a raw body
// (amount, currency, orderId, card, buyer, billingAddress, basketItems,
// sellers, multiSellerPolicy, preauth? …): POST /payments/marketplace/3d
// when threeDS is true, /2d otherwise. amount is positive minor-unit text,
// price and paidPrice are refused, preauth is a boolean. Prefer
// ChargeMarketplaceCard2d / ChargeMarketplaceCard3d, which also check the
// commercial context and the legs.
func (m *MarketplaceClient) ChargeMarketplaceCard(ctx context.Context, payload map[string]any, threeDS bool, idempotencyKey string) (*Response, error) {
	body, err := requestObject("chargeMarketplaceCard payload", payload)
	if err != nil {
		return nil, err
	}
	if err = chargeAmount(body); err != nil {
		return nil, err
	}
	name := "chargeMarketplaceCard2d"
	if threeDS {
		name = "chargeMarketplaceCard3d"
	}
	return m.call(ctx, name, nil, nil, body, idempotencyKey)
}

// ChargeMarketplaceCard2d charges a card without 3-D Secure: the card
// payment body (MarketplaceChargeRequiredFields at least: amount, currency,
// orderId, card, buyer, billingAddress and a non-empty basketItems) plus
// the seller legs. multiSellerPolicy "" is ALL_OR_NOTHING.
func (m *MarketplaceClient) ChargeMarketplaceCard2d(ctx context.Context, request map[string]any, sellers []MarketplaceSeller, multiSellerPolicy, idempotencyKey string) (*Response, error) {
	body, err := chargeBody(request, sellers, multiSellerPolicy)
	if err != nil {
		return nil, err
	}
	return m.ChargeMarketplaceCard(ctx, body, false, idempotencyKey)
}

// ChargeMarketplaceCard3d charges a card with 3-D Secure; the same body as
// ChargeMarketplaceCard2d (with your callbackUrl).
func (m *MarketplaceClient) ChargeMarketplaceCard3d(ctx context.Context, request map[string]any, sellers []MarketplaceSeller, multiSellerPolicy, idempotencyKey string) (*Response, error) {
	body, err := chargeBody(request, sellers, multiSellerPolicy)
	if err != nil {
		return nil, err
	}
	return m.ChargeMarketplaceCard(ctx, body, true, idempotencyKey)
}

// ── Sub-dealers ──────────────────────────────────────────────────────────

// CreateDealer creates a sub-dealer: legalName, optional taxId, dealerType
// (MarketplaceDealerType*), categoryCode, email, externalRef. The dealer
// then onboards itself (KYC, IBAN, contract) before it is ACTIVE.
func (m *MarketplaceClient) CreateDealer(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	body, err := requestObject("createDealer request", request)
	if err != nil {
		return nil, err
	}
	if err = choice(body, "dealerType", MarketplaceDealerTypes()); err != nil {
		return nil, err
	}
	return m.data(ctx, "createDealer", nil, body)
}

// BulkCreateDealers creates several dealers at once (an import); each row
// as in CreateDealer.
func (m *MarketplaceClient) BulkCreateDealers(ctx context.Context, dealers []map[string]any) (json.RawMessage, error) {
	if len(dealers) == 0 {
		return nil, invalid("bulkCreateDealers needs a non-empty array of dealers")
	}
	rows := make([]any, len(dealers))
	for i, dealer := range dealers {
		row, err := requestObject("dealers["+strconv.Itoa(i)+"]", dealer)
		if err != nil {
			return nil, err
		}
		rows[i] = row
	}
	return m.data(ctx, "bulkCreateDealers", nil, map[string]any{"dealers": rows})
}

// ListDealers lists sub-dealers. Filters: status, search, limit, offset.
func (m *MarketplaceClient) ListDealers(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return m.page(ctx, "listDealers", filters)
}

// GetDealer reads one sub-dealer.
func (m *MarketplaceClient) GetDealer(ctx context.Context, dealerID string) (json.RawMessage, error) {
	return m.data(ctx, "getDealer", dealer(dealerID), nil)
}

// UpdateDealerStatus moves a sub-dealer to a MarketplaceDealerStatus*.
func (m *MarketplaceClient) UpdateDealerStatus(ctx context.Context, dealerID, status string) (json.RawMessage, error) {
	if err := oneOf(status, MarketplaceDealerStatuses(), "status"); err != nil {
		return nil, err
	}
	return m.data(ctx, "updateDealerStatus", dealer(dealerID), map[string]any{"status": status})
}

// OnboardingToken creates a self-onboarding link: {token, url, …} (token
// also starts the embedded onboarding SDK), the envelope root and data
// merged. MarketplaceOnboardingURL gives the URL.
func (m *MarketplaceClient) OnboardingToken(ctx context.Context, dealerID string) (json.RawMessage, error) {
	r, err := m.call(ctx, "onboardingToken", dealer(dealerID), nil, nil, "")
	if err != nil {
		return nil, err
	}
	return mergedRoot(r), nil
}

// DealerApplication reads the onboarding / application status.
func (m *MarketplaceClient) DealerApplication(ctx context.Context, dealerID string) (json.RawMessage, error) {
	return m.data(ctx, "dealerApplication", dealer(dealerID), nil)
}

// SubmitDealerApplication submits the dealer's application; payload nil
// sends {}.
func (m *MarketplaceClient) SubmitDealerApplication(ctx context.Context, dealerID string, payload map[string]any) (json.RawMessage, error) {
	return m.data(ctx, "submitDealerApplication", dealer(dealerID), orEmpty(payload))
}

// SetDealerTerms sets the commission / terms definition of a dealer.
func (m *MarketplaceClient) SetDealerTerms(ctx context.Context, dealerID string, payload map[string]any) (json.RawMessage, error) {
	return m.object(ctx, "setDealerTerms", dealer(dealerID), payload)
}

// SignDealerContract records the signature / contract acceptance; payload
// nil sends {}.
func (m *MarketplaceClient) SignDealerContract(ctx context.Context, dealerID string, payload map[string]any) (json.RawMessage, error) {
	return m.data(ctx, "signDealerContract", dealer(dealerID), orEmpty(payload))
}

// ActivateDealer activates a sub-dealer.
func (m *MarketplaceClient) ActivateDealer(ctx context.Context, dealerID string) (json.RawMessage, error) {
	return m.data(ctx, "activateDealer", dealer(dealerID), nil)
}

// RecordDealerKycResult records a KYC outcome, when you run the dealer's
// KYC yourself.
func (m *MarketplaceClient) RecordDealerKycResult(ctx context.Context, dealerID string, payload map[string]any) (json.RawMessage, error) {
	return m.object(ctx, "recordDealerKycResult", dealer(dealerID), payload)
}

// ── Reporting and commissions ────────────────────────────────────────────

// PayoutLedger lists the payout ledger. Filters: status, subDealerId, days,
// limit.
func (m *MarketplaceClient) PayoutLedger(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return m.page(ctx, "payoutLedger", filters)
}

// Transactions lists split transactions. Filters: status, subDealerId,
// limit.
func (m *MarketplaceClient) Transactions(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return m.page(ctx, "transactions", filters)
}

// CommissionRules lists the commission rules. Filter: limit.
func (m *MarketplaceClient) CommissionRules(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return m.page(ctx, "commissionRules", filters)
}

// UpsertCommissionRule creates or updates a commission rule.
func (m *MarketplaceClient) UpsertCommissionRule(ctx context.Context, rule map[string]any) (json.RawMessage, error) {
	return m.object(ctx, "upsertCommissionRule", nil, rule)
}

// PreviewCommissionRule dry-runs a rule's effect.
func (m *MarketplaceClient) PreviewCommissionRule(ctx context.Context, rule map[string]any) (json.RawMessage, error) {
	return m.object(ctx, "previewCommissionRule", nil, rule)
}

// ResolveCommission reads the effective commission of a dealer (and
// category, "" for none) at a month-to-date volume (mtdVolumeMinor, a
// minor-unit string, "" for none).
func (m *MarketplaceClient) ResolveCommission(ctx context.Context, subDealerID, categoryCode, mtdVolumeMinor string) (json.RawMessage, error) {
	id, err := requiredText(subDealerID, "subDealerId")
	if err != nil {
		return nil, err
	}
	query := optionalText(optionalText(map[string]any{"subDealerId": id}, "categoryCode", categoryCode), "mtdVolumeMinor", mtdVolumeMinor)
	r, err := m.call(ctx, "resolveCommission", nil, query, nil, "")
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// ── Payouts ──────────────────────────────────────────────────────────────

// CreatePayout creates a manual payout: subDealerId, amountMinor (positive
// minor-unit string), currency, reference? …
func (m *MarketplaceClient) CreatePayout(ctx context.Context, payload map[string]any) (json.RawMessage, error) {
	body, err := requestObject("createPayout payload", payload)
	if err != nil {
		return nil, err
	}
	if amount, ok := body["amountMinor"]; ok && amount != nil {
		if text, isText := amount.(string); !isText || !positiveMoney(text) {
			return nil, invalid("amountMinor must be positive minor-unit text")
		}
	}
	return m.data(ctx, "createPayout", nil, body)
}

// MarkPayoutPaid marks a payout paid: paidAt?, reference? …; payload nil
// sends {}.
func (m *MarketplaceClient) MarkPayoutPaid(ctx context.Context, payoutID string, payload map[string]any) (json.RawMessage, error) {
	return m.data(ctx, "markPayoutPaid", map[string]string{"payoutId": payoutID}, orEmpty(payload))
}

// BulkMarkPayoutsPaid marks several payouts paid; extra (paidAt,
// reference …) goes into the same body, nil for none.
func (m *MarketplaceClient) BulkMarkPayoutsPaid(ctx context.Context, payoutIDs []string, extra map[string]any) (json.RawMessage, error) {
	if len(payoutIDs) == 0 {
		return nil, invalid("bulkMarkPayoutsPaid needs a non-empty array of payout ids")
	}
	ids := make([]any, len(payoutIDs))
	for i, id := range payoutIDs {
		text, err := requiredText(id, "payoutIds["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		ids[i] = text
	}
	body := maps.Clone(orEmpty(extra))
	body["payoutIds"] = ids
	return m.data(ctx, "bulkMarkPayoutsPaid", nil, body)
}

// ── Audit log (append-only, hash-chained) ────────────────────────────────

// AuditLog lists the audit log. Filters: limit, type.
func (m *MarketplaceClient) AuditLog(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return m.page(ctx, "auditLog", filters)
}

// VerifyAuditLog verifies the integrity of the hash chain.
func (m *MarketplaceClient) VerifyAuditLog(ctx context.Context) (json.RawMessage, error) {
	return m.data(ctx, "verifyAuditLog", nil, nil)
}

// ── Route invoker ────────────────────────────────────────────────────────

func dealer(dealerID string) map[string]string { return map[string]string{"dealerId": dealerID} }

func (m *MarketplaceClient) call(ctx context.Context, name string, params map[string]string, query, body map[string]any, key string) (*Response, error) {
	return sendTableRoute(ctx, m.client, marketplaceRoutes, name, tableCall{params: params, query: query, body: body, key: key})
}

func (m *MarketplaceClient) data(ctx context.Context, name string, params map[string]string, body map[string]any) (json.RawMessage, error) {
	r, err := m.call(ctx, name, params, nil, body, "")
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// object sends a Free body that must be given (nil is refused).
func (m *MarketplaceClient) object(ctx context.Context, name string, params map[string]string, body map[string]any) (json.RawMessage, error) {
	if body == nil {
		return nil, invalid(name + " body must be an object")
	}
	return m.data(ctx, name, params, body)
}

func (m *MarketplaceClient) page(ctx context.Context, name string, filters map[string]any) (*CursorPage, error) {
	r, err := m.call(ctx, name, nil, orEmpty(filters), nil, "")
	if err != nil {
		return nil, err
	}
	return tablePage(r), nil
}

func multiSellerPolicyOf(policy string) (string, error) {
	if policy == "" {
		return MarketplaceMultiSellerPolicyAllOrNothing, nil
	}
	if err := oneOf(policy, MarketplaceMultiSellerPolicies(), "multiSellerPolicy"); err != nil {
		return "", err
	}
	return policy, nil
}

// chargeBody is the body of ChargeMarketplaceCard2d / 3d: the commercial
// context is required, then the seller legs and the policy are added.
func chargeBody(request map[string]any, sellers []MarketplaceSeller, multiSellerPolicy string) (map[string]any, error) {
	body, err := requestObject("marketplace charge request", request)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, field := range marketplaceChargeRequiredFields {
		if !presentValue(body[field]) {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return nil, invalid("a marketplace card charge requires " + strings.Join(missing, ", "))
	}
	if items, ok := body["basketItems"].([]any); !ok || len(items) == 0 {
		return nil, invalid("a marketplace card charge requires at least one basket item")
	}
	if _, ok := body["sellers"]; ok {
		return nil, invalid("pass the seller legs and multiSellerPolicy as arguments, not in the request body")
	}
	if _, ok := body["multiSellerPolicy"]; ok {
		return nil, invalid("pass the seller legs and multiSellerPolicy as arguments, not in the request body")
	}
	if err = chargeAmount(body); err != nil {
		return nil, err
	}
	policy, err := multiSellerPolicyOf(multiSellerPolicy)
	if err != nil {
		return nil, err
	}
	amount, _ := body["amount"].(string)
	legs, err := sellerLegs(sellers, amount)
	if err != nil {
		return nil, err
	}
	rows := make([]any, len(legs))
	for i, leg := range legs {
		row := map[string]any{"subDealerId": leg.SubDealerID, "amountMinor": leg.AmountMinor}
		if leg.Scenario != nil {
			row["scenario"] = leg.Scenario
		}
		rows[i] = row
	}
	body["sellers"] = rows
	body["multiSellerPolicy"] = policy
	return body, nil
}

// chargeAmount: amount (the basket total) is positive minor-unit text;
// price and paidPrice are removed fields; preauth is a boolean. The error
// names the field only, never a card value.
func chargeAmount(body map[string]any) error {
	var removed []string
	for _, field := range []string{"price", "paidPrice"} {
		if _, ok := body[field]; ok {
			removed = append(removed, field)
		}
	}
	if len(removed) > 0 {
		return invalid(strings.Join(removed, ", ") + ": removed from the payment body; send the amount only in 'amount' (minor units, the basket total without the installment fee)")
	}
	if amount, ok := body["amount"].(string); !ok || !positiveMoney(amount) {
		return invalid("amount must be positive minor-unit text")
	}
	if value, ok := body["preauth"]; ok {
		if _, isBool := value.(bool); !isBool {
			return invalid("preauth must be a boolean (true = pre-authorization, false = immediate capture)")
		}
	}
	return nil
}

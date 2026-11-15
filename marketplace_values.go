package lunixi

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
)

// Marketplace (sub-dealer split) — /api/v1/marketplace/* for dealers,
// commissions, payouts and the audit log (bearer-only), and the split
// charges under /api/v1/payments/marketplace/* (signed per request). The
// values are the same codes as the other Lunixi SDKs.

// Sub-dealer lifecycle. Only ACTIVE dealers can receive a split.
const (
	MarketplaceDealerStatusDraft      = "DRAFT"
	MarketplaceDealerStatusPendingKyc = "PENDING_KYC"
	MarketplaceDealerStatusActive     = "ACTIVE"
	MarketplaceDealerStatusSuspended  = "SUSPENDED"
	MarketplaceDealerStatusHold       = "HOLD"
	MarketplaceDealerStatusClosed     = "CLOSED"
)

// dealerType of a sub-dealer.
const (
	MarketplaceDealerTypeLegalEntity = "LEGAL_ENTITY"
	MarketplaceDealerTypeIndividual  = "INDIVIDUAL"
)

// multiSellerPolicy of a split charge: every seller leg must be eligible, or
// the eligible legs may go through without the others.
const (
	MarketplaceMultiSellerPolicyAllOrNothing   = "ALL_OR_NOTHING"
	MarketplaceMultiSellerPolicyPartialAllowed = "PARTIAL_ALLOWED"
)

// MarketplaceDealerOnboardingHostedBase is the hosted dealer onboarding
// wizard (MarketplaceOnboardingURL when the answer carries no url).
const MarketplaceDealerOnboardingHostedBase = "https://altbayi.lunixi.com"

// MarketplaceDealerStatuses returns every sub-dealer status.
func MarketplaceDealerStatuses() []string {
	return []string{MarketplaceDealerStatusDraft, MarketplaceDealerStatusPendingKyc, MarketplaceDealerStatusActive, MarketplaceDealerStatusSuspended, MarketplaceDealerStatusHold, MarketplaceDealerStatusClosed}
}

// MarketplaceDealerTypes returns every sub-dealer type.
func MarketplaceDealerTypes() []string {
	return []string{MarketplaceDealerTypeLegalEntity, MarketplaceDealerTypeIndividual}
}

// MarketplaceMultiSellerPolicies returns every multi-seller policy.
func MarketplaceMultiSellerPolicies() []string {
	return []string{MarketplaceMultiSellerPolicyAllOrNothing, MarketplaceMultiSellerPolicyPartialAllowed}
}

// marketplaceSellerFields are the fields of a seller leg: the aggregate
// amount of one sub-dealer.
var marketplaceSellerFields = []string{"subDealerId", "amountMinor", "scenario"}

// MarketplaceSellerFields returns the fields of a seller leg.
func MarketplaceSellerFields() []string { return slices.Clone(marketplaceSellerFields) }

// marketplaceChargeRequiredFields are what a split card charge
// (ChargeMarketplaceCard2d / 3d) needs besides the seller legs: the
// commercial context of a traceable card payment.
var marketplaceChargeRequiredFields = []string{"amount", "currency", "orderId", "card", "buyer", "billingAddress", "basketItems"}

// MarketplaceChargeRequiredFields returns the fields a split card charge
// requires.
func MarketplaceChargeRequiredFields() []string { return slices.Clone(marketplaceChargeRequiredFields) }

const (
	marketplaceBase       = "/api/v1/marketplace"
	marketplaceChargeBase = "/api/v1/payments/marketplace"
)

func marketplaceCharge(path string) Route {
	return Route{Method: "POST", Path: marketplaceChargeBase + path, StepUp: true, OptionalIdempotencyKey: true, Free: true}
}

// marketplaceRoutes is the single description of every marketplace route
// the SDK calls (the same table as the other Lunixi SDKs). Only the three
// split charges are signed per request.
var marketplaceRoutes = map[string]Route{
	"createCheckoutIntent":    marketplaceCharge("/intents"),
	"chargeMarketplaceCard2d": marketplaceCharge("/2d"),
	"chargeMarketplaceCard3d": marketplaceCharge("/3d"),

	"createDealer":            {Method: "POST", Path: marketplaceBase + "/dealers", Required: []string{"legalName"}, Optional: []string{"taxId", "dealerType", "categoryCode", "email", "externalRef"}},
	"bulkCreateDealers":       {Method: "POST", Path: marketplaceBase + "/dealers/bulk", Required: []string{"dealers"}},
	"listDealers":             {Method: "GET", Path: marketplaceBase + "/dealers", Query: []string{"status", "search", "limit", "offset"}},
	"getDealer":               {Method: "GET", Path: marketplaceBase + "/dealers/:dealerId"},
	"updateDealerStatus":      {Method: "PUT", Path: marketplaceBase + "/dealers/:dealerId/status", Required: []string{"status"}},
	"onboardingToken":         {Method: "POST", Path: marketplaceBase + "/dealers/:dealerId/onboarding-token"},
	"dealerApplication":       {Method: "GET", Path: marketplaceBase + "/dealers/:dealerId/application"},
	"submitDealerApplication": {Method: "POST", Path: marketplaceBase + "/dealers/:dealerId/submit", Free: true},
	"setDealerTerms":          {Method: "POST", Path: marketplaceBase + "/dealers/:dealerId/terms", Free: true},
	"signDealerContract":      {Method: "POST", Path: marketplaceBase + "/dealers/:dealerId/sign", Free: true},
	"activateDealer":          {Method: "POST", Path: marketplaceBase + "/dealers/:dealerId/activate"},
	"recordDealerKycResult":   {Method: "POST", Path: marketplaceBase + "/dealers/:dealerId/kyc-result", Free: true},

	"payoutLedger":          {Method: "GET", Path: marketplaceBase + "/payout-ledger", Query: []string{"status", "subDealerId", "days", "limit"}},
	"transactions":          {Method: "GET", Path: marketplaceBase + "/transactions", Query: []string{"status", "subDealerId", "limit"}},
	"commissionRules":       {Method: "GET", Path: marketplaceBase + "/commission-rules", Query: []string{"limit"}},
	"upsertCommissionRule":  {Method: "POST", Path: marketplaceBase + "/commission-rules", Free: true},
	"previewCommissionRule": {Method: "POST", Path: marketplaceBase + "/commission-rules/preview", Free: true},
	"resolveCommission":     {Method: "GET", Path: marketplaceBase + "/commission/resolve", Query: []string{"subDealerId", "categoryCode", "mtdVolumeMinor"}},

	"createPayout":        {Method: "POST", Path: marketplaceBase + "/payouts", Free: true},
	"markPayoutPaid":      {Method: "POST", Path: marketplaceBase + "/payouts/:payoutId/mark-paid", Free: true},
	"bulkMarkPayoutsPaid": {Method: "POST", Path: marketplaceBase + "/payouts/bulk-mark-paid", Free: true},

	"auditLog":       {Method: "GET", Path: marketplaceBase + "/audit-log", Query: []string{"limit", "type"}},
	"verifyAuditLog": {Method: "GET", Path: marketplaceBase + "/audit-log/verify"},
}

// MarketplaceRoutes returns a copy of the marketplace route table, keyed by
// operation name.
func MarketplaceRoutes() map[string]Route { return cloneRoutes(marketplaceRoutes) }

// MarketplaceOnboardingURL is the onboarding URL of an OnboardingToken
// answer: its url, else the hosted wizard with its token. base "" is
// MarketplaceDealerOnboardingHostedBase.
func MarketplaceOnboardingURL(link json.RawMessage, base string) string {
	fields := objectFields(link)
	if address := stringField(fields, "url"); address != "" {
		return address
	}
	if base == "" {
		base = MarketplaceDealerOnboardingHostedBase
	}
	return strings.TrimRight(base, "/") + "/m/" + url.PathEscape(stringField(fields, "token"))
}

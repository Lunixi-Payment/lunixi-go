package lunixi

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
)

// Subscriptions — /api/v1/subscriptions/* and the customer-portal link
// routes under /api/panel/v1/subscriptions/portal. The values are the same
// codes as the other Lunixi SDKs.

// Errors of Customers.Upsert: a missing or invalid customer field
// (details.field, details.reason, details.fields).
const (
	SubscriptionCustomerErrorCodeFieldRequired = "subscription.customer.field_required"
	SubscriptionCustomerErrorCodeFieldInvalid  = "subscription.customer.field_invalid"
)

// Status of a subscription.
const (
	// SubscriptionStatusPending: startsAt is in the future.
	SubscriptionStatusPending  = "PENDING"
	SubscriptionStatusTrialing = "TRIALING"
	SubscriptionStatusActive   = "ACTIVE"
	// SubscriptionStatusPastDue: a charge failed; dunning runs.
	SubscriptionStatusPastDue = "PAST_DUE"
	// SubscriptionStatusUnpaid: dunning exhausted; grace period.
	SubscriptionStatusUnpaid = "UNPAID"
	// SubscriptionStatusPaused: paused by the merchant; no charges.
	SubscriptionStatusPaused     = "PAUSED"
	SubscriptionStatusUpgraded   = "UPGRADED"
	SubscriptionStatusDowngraded = "DOWNGRADED"
	SubscriptionStatusCanceled   = "CANCELED"
	// SubscriptionStatusExpired: the recurrence ran out.
	SubscriptionStatusExpired = "EXPIRED"
)

// Status of a subscription invoice: DRAFT → OPEN → PAID | UNCOLLECTIBLE |
// VOID.
const (
	SubscriptionInvoiceStatusDraft         = "DRAFT"
	SubscriptionInvoiceStatusOpen          = "OPEN"
	SubscriptionInvoiceStatusPaid          = "PAID"
	SubscriptionInvoiceStatusUncollectible = "UNCOLLECTIBLE"
	SubscriptionInvoiceStatusVoid          = "VOID"
)

// How invoices are collected (collectionMethod).
const (
	// SubscriptionCollectionMethodAutoCharge charges the saved card each
	// cycle (the default).
	SubscriptionCollectionMethodAutoCharge = "AUTO_CHARGE"
	// SubscriptionCollectionMethodSendInvoice finalises the invoice without
	// charging it.
	SubscriptionCollectionMethodSendInvoice = "SEND_INVOICE"
	// SubscriptionCollectionMethodManual leaves collection to the merchant.
	SubscriptionCollectionMethodManual = "MANUAL"
)

// billing.frequency of a plan.
const (
	SubscriptionBillingFrequencyDaily   = "DAILY"
	SubscriptionBillingFrequencyWeekly  = "WEEKLY"
	SubscriptionBillingFrequencyMonthly = "MONTHLY"
	SubscriptionBillingFrequencyYearly  = "YEARLY"
)

// verificationMode of a saved card.
const (
	SubscriptionCardVerificationModeNone        = "NONE"
	SubscriptionCardVerificationModeOnAdd       = "ON_ADD"
	SubscriptionCardVerificationModeFirstCharge = "FIRST_CHARGE"
)

// behavior of a plan change: now (prorated) or at the next period.
const (
	SubscriptionPlanChangeBehaviorImmediate  = "IMMEDIATE"
	SubscriptionPlanChangeBehaviorNextPeriod = "NEXT_PERIOD"
)

// sdkMode of a portal SDK session.
const (
	SubscriptionPortalSdkModeFull     = "FULL"
	SubscriptionPortalSdkModeHeadless = "HEADLESS"
	SubscriptionPortalSdkModeHybrid   = "HYBRID"
)

// SubscriptionPortalHostedBase is the hosted customer portal; a portal
// link's token opens it (SubscriptionPortalURL).
const SubscriptionPortalHostedBase = "https://portal.lunixi.com"

// Webhook event types of subscriptions (the same signed envelope as payment
// events; data amounts are minor-unit strings).
const (
	SubscriptionEventCreated              = "subscription.created"
	SubscriptionEventActivated            = "subscription.activated"
	SubscriptionEventTrialWillEnd         = "subscription.trial_will_end"
	SubscriptionEventPaused               = "subscription.paused"
	SubscriptionEventResumed              = "subscription.resumed"
	SubscriptionEventCancelScheduled      = "subscription.cancel_scheduled"
	SubscriptionEventCanceled             = "subscription.canceled"
	SubscriptionEventUpgraded             = "subscription.upgraded"
	SubscriptionEventDowngraded           = "subscription.downgraded"
	SubscriptionEventPlanChangeScheduled  = "subscription.plan_change_scheduled"
	SubscriptionEventPendingChangeApplied = "subscription.pending_change_applied"
	SubscriptionEventPaymentCardSwapped   = "subscription.payment_card_swapped"
	SubscriptionEventPaymentMethodLost    = "subscription.payment_method_lost"
	SubscriptionEventInvoiceCreated       = "invoice.created"
	SubscriptionEventInvoicePaid          = "invoice.paid"
	SubscriptionEventInvoicePaymentFailed = "invoice.payment_failed"
	SubscriptionEventInvoiceVoided        = "invoice.voided"
	SubscriptionEventCardAdded            = "card.added"
	SubscriptionEventCardSetDefault       = "card.set_default"
	SubscriptionEventCardUpdated          = "card.updated"
	SubscriptionEventCardDeactivated      = "card.deactivated"
	SubscriptionEventCardExpiringSoon     = "card.expiring_soon"
	SubscriptionEventCardExpired          = "card.expired"
)

// SubscriptionStatuses returns every subscription status.
func SubscriptionStatuses() []string {
	return []string{SubscriptionStatusPending, SubscriptionStatusTrialing, SubscriptionStatusActive, SubscriptionStatusPastDue, SubscriptionStatusUnpaid, SubscriptionStatusPaused, SubscriptionStatusUpgraded, SubscriptionStatusDowngraded, SubscriptionStatusCanceled, SubscriptionStatusExpired}
}

// SubscriptionActiveStates returns the live states: the subscription may be
// billed.
func SubscriptionActiveStates() []string {
	return []string{SubscriptionStatusTrialing, SubscriptionStatusActive, SubscriptionStatusPastDue}
}

// SubscriptionTerminalStates returns the states without a further
// transition.
func SubscriptionTerminalStates() []string {
	return []string{SubscriptionStatusUpgraded, SubscriptionStatusDowngraded, SubscriptionStatusCanceled, SubscriptionStatusExpired}
}

// SubscriptionInvoiceStatuses returns every invoice status.
func SubscriptionInvoiceStatuses() []string {
	return []string{SubscriptionInvoiceStatusDraft, SubscriptionInvoiceStatusOpen, SubscriptionInvoiceStatusPaid, SubscriptionInvoiceStatusUncollectible, SubscriptionInvoiceStatusVoid}
}

// SubscriptionCollectionMethods returns every collection method.
func SubscriptionCollectionMethods() []string {
	return []string{SubscriptionCollectionMethodAutoCharge, SubscriptionCollectionMethodSendInvoice, SubscriptionCollectionMethodManual}
}

// SubscriptionBillingFrequencies returns every billing frequency.
func SubscriptionBillingFrequencies() []string {
	return []string{SubscriptionBillingFrequencyDaily, SubscriptionBillingFrequencyWeekly, SubscriptionBillingFrequencyMonthly, SubscriptionBillingFrequencyYearly}
}

// SubscriptionCardVerificationModes returns every card verification mode.
func SubscriptionCardVerificationModes() []string {
	return []string{SubscriptionCardVerificationModeNone, SubscriptionCardVerificationModeOnAdd, SubscriptionCardVerificationModeFirstCharge}
}

// SubscriptionPlanChangeBehaviors returns every plan-change behavior.
func SubscriptionPlanChangeBehaviors() []string {
	return []string{SubscriptionPlanChangeBehaviorImmediate, SubscriptionPlanChangeBehaviorNextPeriod}
}

// SubscriptionPortalSdkModes returns every portal SDK mode.
func SubscriptionPortalSdkModes() []string {
	return []string{SubscriptionPortalSdkModeFull, SubscriptionPortalSdkModeHeadless, SubscriptionPortalSdkModeHybrid}
}

// SubscriptionEventTypes returns every subscription webhook event type.
func SubscriptionEventTypes() []string {
	return []string{
		SubscriptionEventCreated, SubscriptionEventActivated, SubscriptionEventTrialWillEnd, SubscriptionEventPaused,
		SubscriptionEventResumed, SubscriptionEventCancelScheduled, SubscriptionEventCanceled, SubscriptionEventUpgraded,
		SubscriptionEventDowngraded, SubscriptionEventPlanChangeScheduled, SubscriptionEventPendingChangeApplied,
		SubscriptionEventPaymentCardSwapped, SubscriptionEventPaymentMethodLost, SubscriptionEventInvoiceCreated,
		SubscriptionEventInvoicePaid, SubscriptionEventInvoicePaymentFailed, SubscriptionEventInvoiceVoided,
		SubscriptionEventCardAdded, SubscriptionEventCardSetDefault, SubscriptionEventCardUpdated,
		SubscriptionEventCardDeactivated, SubscriptionEventCardExpiringSoon, SubscriptionEventCardExpired,
	}
}

// subscriptionBillingRuleFields are the fields of a plan's billing rule.
var subscriptionBillingRuleFields = []string{"frequency", "interval", "byMonthDay", "byWeekDay", "timezone", "lastDayOfMonthFix", "count"}

// SubscriptionBillingRuleFields returns the fields of a plan's billing rule.
func SubscriptionBillingRuleFields() []string { return slices.Clone(subscriptionBillingRuleFields) }

const (
	subscriptionsBase = "/api/v1/subscriptions"
	portalPanelBase   = subscriptionPortalPanelBase
)

// subscriptionRoutes is the single description of every subscription route
// the SDK calls (the same table as the other Lunixi SDKs). The surface is
// bearer-only: no route is signed per request.
var subscriptionRoutes = map[string]Route{
	"create": {Method: "POST", Path: subscriptionsBase + "/subscriptions", IdempotencyKey: true,
		Required: []string{"customerId", "planId", "idempotencyKey"},
		Optional: []string{"paymentCardId", "paymentMethodId", "collectionMethod", "priceUpdatePolicy", "entitlementPolicy", "billingTimezone", "startsAt", "chargeOnCreate", "billingCycleAnchorOverride", "skipTrial", "metadata"}},
	"get":        {Method: "GET", Path: subscriptionsBase + "/subscriptions/:subscriptionId"},
	"list":       {Method: "GET", Path: subscriptionsBase + "/subscriptions", Query: []string{"customerId", "status", "pageSize", "pageToken"}},
	"cancel":     {Method: "POST", Path: subscriptionsBase + "/subscriptions/:subscriptionId/cancel", Required: []string{"cancelAtPeriodEnd"}, Optional: []string{"reason"}},
	"pause":      {Method: "POST", Path: subscriptionsBase + "/subscriptions/:subscriptionId/pause", Optional: []string{"resumeAt"}, OptionalBody: true},
	"resume":     {Method: "POST", Path: subscriptionsBase + "/subscriptions/:subscriptionId/resume"},
	"changePlan": {Method: "POST", Path: subscriptionsBase + "/subscriptions/:subscriptionId/change-plan", OptionalIdempotencyKey: true, Required: []string{"newPlanId"}, Optional: []string{"behavior", "reason", "idempotencyKey"}},
	"auditLogs":  {Method: "GET", Path: subscriptionsBase + "/subscriptions/:subscriptionId/audit-logs", Query: []string{"limit", "pageToken"}},

	"plans.create": {Method: "POST", Path: subscriptionsBase + "/plans", OptionalIdempotencyKey: true,
		Required: []string{"productId", "code", "name", "price", "currency", "billing"},
		Optional: []string{"description", "trialPeriodDays", "maxDunningAttempts", "dunningScheduleDays", "gracePeriodDays", "metadata"}},
	"plans.get":               {Method: "GET", Path: subscriptionsBase + "/plans/:planId"},
	"plans.list":              {Method: "GET", Path: subscriptionsBase + "/plans", Query: []string{"productId", "includeArchived", "pageSize", "pageToken"}},
	"plans.update":            {Method: "PATCH", Path: subscriptionsBase + "/plans/:planId", Free: true},
	"plans.archive":           {Method: "DELETE", Path: subscriptionsBase + "/plans/:planId"},
	"plans.translations":      {Method: "GET", Path: subscriptionsBase + "/plans/:planId/translations"},
	"plans.upsertTranslation": {Method: "PATCH", Path: subscriptionsBase + "/plans/:planId/translations/:locale", Free: true},

	"customers.upsert":          {Method: "PUT", Path: subscriptionsBase + "/customers", OptionalIdempotencyKey: true, Required: []string{"externalId", "name", "surname", "email", "gsmNumber", "identityNumber", "billingAddress", "billingCity", "billingCountry", "billingZipCode"}, Optional: []string{"billingContactName", "shippingContactName", "shippingAddress", "shippingCity", "shippingCountry", "shippingZipCode"}},
	"customers.get":             {Method: "GET", Path: subscriptionsBase + "/customers/:customerId"},
	"customers.getByExternalId": {Method: "GET", Path: subscriptionsBase + "/customers/by-external/:externalId"},
	"customers.list":            {Method: "GET", Path: subscriptionsBase + "/customers", Query: []string{"search", "includeArchived", "pageSize", "pageToken"}},
	"customers.creditBalance":   {Method: "GET", Path: subscriptionsBase + "/customers/:customerId/credit-balance"},
	"customers.adjustCredit":    {Method: "POST", Path: subscriptionsBase + "/customers/:customerId/credit-balance/adjust", Free: true},
	"customers.archive":         {Method: "DELETE", Path: subscriptionsBase + "/customers/:customerId"},

	"cards.add": {Method: "POST", Path: subscriptionsBase + "/cards", OptionalIdempotencyKey: true,
		Required: []string{"customerId", "paymentMethodToken"},
		Optional: []string{"brand", "last4", "bin", "expireMonth", "expireYear", "cardholderName", "isDefault", "nickname", "verificationMode", "supportedCurrencies"}},
	"cards.get":        {Method: "GET", Path: subscriptionsBase + "/cards/:cardId"},
	"cards.list":       {Method: "GET", Path: subscriptionsBase + "/cards", Query: []string{"customerId", "includeInactive", "pageSize", "pageToken"}},
	"cards.setDefault": {Method: "POST", Path: subscriptionsBase + "/cards/:cardId/set-default"},
	"cards.deactivate": {Method: "DELETE", Path: subscriptionsBase + "/cards/:cardId", Optional: []string{"reason"}, OptionalBody: true},
	"cards.verify":     {Method: "POST", Path: subscriptionsBase + "/cards/:cardId/verify", Free: true, OptionalBody: true},
	"cards.update":     {Method: "PATCH", Path: subscriptionsBase + "/cards/:cardId", Free: true},

	"invoices.list":        {Method: "GET", Path: subscriptionsBase + "/invoices", Query: []string{"subscriptionId", "status", "pageSize", "pageToken"}},
	"invoices.get":         {Method: "GET", Path: subscriptionsBase + "/invoices/:invoiceId"},
	"invoices.void":        {Method: "POST", Path: subscriptionsBase + "/invoices/:invoiceId/void", Optional: []string{"reason"}, OptionalBody: true},
	"invoices.retryCharge": {Method: "POST", Path: subscriptionsBase + "/invoices/:invoiceId/retry-charge", OptionalIdempotencyKey: true},

	"portal.createSdkSession": {Method: "POST", Path: subscriptionsBase + "/portal/sdk-session", Required: []string{"customerId", "origin"}, Optional: []string{"sdkMode", "portalLinkId"}},
	"portal.createPortalLink": {Method: "POST", Path: portalPanelBase + "/links", Free: true},
	"portal.listLinks":        {Method: "GET", Path: portalPanelBase + "/links", Query: []string{"customerId", "status", "limit"}},
	"portal.getLink":          {Method: "GET", Path: portalPanelBase + "/links/:linkId"},
	"portal.updateLink":       {Method: "PATCH", Path: portalPanelBase + "/links/:linkId", Free: true},
	"portal.revokeLink":       {Method: "DELETE", Path: portalPanelBase + "/links/:linkId", Optional: []string{"reason"}, OptionalBody: true},
	"portal.linkActivity":     {Method: "GET", Path: portalPanelBase + "/links/:linkId/activity"},
	"portal.sendInvite":       {Method: "POST", Path: portalPanelBase + "/invite", Free: true},
	"portal.getSettings":      {Method: "GET", Path: portalPanelBase + "/settings"},
	"portal.updateSettings":   {Method: "PUT", Path: portalPanelBase + "/settings", Free: true},

	"products.create":            {Method: "POST", Path: subscriptionsBase + "/products", Free: true},
	"products.list":              {Method: "GET", Path: subscriptionsBase + "/products", Query: []string{"limit", "cursor", "active"}},
	"products.get":               {Method: "GET", Path: subscriptionsBase + "/products/:productId"},
	"products.update":            {Method: "PATCH", Path: subscriptionsBase + "/products/:productId", Free: true},
	"products.archive":           {Method: "DELETE", Path: subscriptionsBase + "/products/:productId"},
	"products.translations":      {Method: "GET", Path: subscriptionsBase + "/products/:productId/translations"},
	"products.upsertTranslation": {Method: "PATCH", Path: subscriptionsBase + "/products/:productId/translations/:locale", Free: true},

	"features.create":            {Method: "POST", Path: subscriptionsBase + "/features", Free: true},
	"features.list":              {Method: "GET", Path: subscriptionsBase + "/features", AnyQuery: true},
	"features.get":               {Method: "GET", Path: subscriptionsBase + "/features/:featureId"},
	"features.update":            {Method: "PATCH", Path: subscriptionsBase + "/features/:featureId", Free: true},
	"features.delete":            {Method: "DELETE", Path: subscriptionsBase + "/features/:featureId"},
	"features.translations":      {Method: "GET", Path: subscriptionsBase + "/features/:featureId/translations"},
	"features.upsertTranslation": {Method: "PATCH", Path: subscriptionsBase + "/features/:featureId/translations/:locale", Free: true},

	"addons.create":     {Method: "POST", Path: subscriptionsBase + "/addons", Free: true},
	"addons.list":       {Method: "GET", Path: subscriptionsBase + "/addons", AnyQuery: true},
	"addons.update":     {Method: "PATCH", Path: subscriptionsBase + "/addons/:addonId", Free: true},
	"addons.deactivate": {Method: "DELETE", Path: subscriptionsBase + "/addons/:addonId"},

	"usage.record":           {Method: "POST", Path: subscriptionsBase + "/usage/record", Free: true},
	"usage.checkEntitlement": {Method: "POST", Path: subscriptionsBase + "/usage/check-entitlement", Free: true},
}

// SubscriptionRoutes returns a copy of the subscription route table, keyed
// by operation name ("plans.create").
func SubscriptionRoutes() map[string]Route { return cloneRoutes(subscriptionRoutes) }

// IsSubscriptionActive reports whether a subscription in this status may be
// billed (TRIALING, ACTIVE, PAST_DUE).
func IsSubscriptionActive(status string) bool {
	return slices.Contains(SubscriptionActiveStates(), status)
}

// IsSubscriptionTerminal reports UPGRADED, DOWNGRADED, CANCELED and EXPIRED.
func IsSubscriptionTerminal(status string) bool {
	return slices.Contains(SubscriptionTerminalStates(), status)
}

// SubscriptionPortalURL is the hosted portal URL of a portal link: link is
// the Portal.CreatePortalLink answer (its token is read) or the token as a
// JSON string. base "" is SubscriptionPortalHostedBase.
func SubscriptionPortalURL(link json.RawMessage, base string) string {
	var token string
	if json.Unmarshal(link, &token) != nil {
		token = stringField(objectFields(link), "token")
	}
	if base == "" {
		base = SubscriptionPortalHostedBase
	}
	return strings.TrimRight(base, "/") + "/?token=" + url.QueryEscape(token)
}

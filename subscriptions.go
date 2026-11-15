package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"maps"
	"strings"
)

// SubscriptionsClient — subscriptions (/api/v1/subscriptions/*), reached as
// Client.Subscriptions. The related resources hang off Plans, Customers,
// Cards, Invoices, Portal, Products, Features, Addons and Usage.
//
// The calls carry the access token minted from your key, without the
// per-request signature. Creating a subscription requires an
// Idempotency-Key (reuse it when retrying the same subscription; it is sent
// in the body too); plan, customer, card, plan-change and charge-retry
// writes accept one. Keys are 1–200 printable ASCII characters.
//
// Requests and filters are maps with the gateway's field names, checked
// against SubscriptionRoutes before anything is sent: an unknown field or
// filter, a missing required field, a value outside its set or a missing
// key returns ErrConfiguration, which names the field and never its value.
// Amounts are canonical minor-unit strings ("9900" is 99.00 TRY; "0" is a
// free plan), checked in bodies, filters and answers.
//
// Single records are returned as the envelope's data. The subscription,
// plan, customer, card, invoice and audit-log lists return CursorPage with
// the nextPageToken in NextCursor (send it back as pageToken) and PageSize;
// the other lists return CursorPage with the cursor of the answer.
type SubscriptionsClient struct {
	client    *Client
	Plans     *SubscriptionPlans
	Customers *SubscriptionCustomers
	Cards     *SubscriptionCards
	Invoices  *SubscriptionInvoices
	Portal    *SubscriptionPortal
	Products  *SubscriptionProducts
	Features  *SubscriptionFeatures
	Addons    *SubscriptionAddons
	Usage     *SubscriptionUsage
}

func newSubscriptionsClient(c *Client) *SubscriptionsClient {
	s := &SubscriptionsClient{client: c}
	s.Plans = &SubscriptionPlans{subs: s}
	s.Customers = &SubscriptionCustomers{subs: s}
	s.Cards = &SubscriptionCards{subs: s}
	s.Invoices = &SubscriptionInvoices{subs: s}
	s.Portal = &SubscriptionPortal{subs: s}
	s.Products = &SubscriptionProducts{subs: s}
	s.Features = &SubscriptionFeatures{subs: s}
	s.Addons = &SubscriptionAddons{subs: s}
	s.Usage = &SubscriptionUsage{subs: s}
	return s
}

// Create creates a subscription: customerId, planId, optional
// paymentCardId, paymentMethodId, collectionMethod, priceUpdatePolicy,
// entitlementPolicy, billingTimezone, startsAt, chargeOnCreate,
// billingCycleAnchorOverride, skipTrial, metadata. The Idempotency-Key is
// required (the argument or the request's idempotencyKey; both given, they
// must agree) and is sent in the header and the body.
func (s *SubscriptionsClient) Create(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, err := requestObject("create request", request)
	if err != nil {
		return nil, err
	}
	key, err := bodyKey("create", body, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if key != "" {
		body["idempotencyKey"] = key
	}
	if err = choice(body, "collectionMethod", SubscriptionCollectionMethods()); err != nil {
		return nil, err
	}
	for _, field := range []string{"chargeOnCreate", "skipTrial"} {
		if err = booleanMember(body, field, field); err != nil {
			return nil, err
		}
	}
	if err = objectMember(body, "metadata"); err != nil {
		return nil, err
	}
	return s.data(ctx, "create", nil, body, key)
}

// Get reads one subscription.
func (s *SubscriptionsClient) Get(ctx context.Context, subscriptionID string) (json.RawMessage, error) {
	return s.data(ctx, "get", map[string]string{"subscriptionId": subscriptionID}, nil, "")
}

// List lists subscriptions. Filters: customerId, status, pageSize,
// pageToken.
func (s *SubscriptionsClient) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return s.subscriptionList(ctx, "list", nil, filters)
}

// Iterate yields every subscription matching the filters, page by page.
func (s *SubscriptionsClient) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateToken(ctx, filters, "pageToken", s.List)
}

// Cancel cancels a subscription now, or at the period end with atPeriodEnd;
// reason "" sends none.
func (s *SubscriptionsClient) Cancel(ctx context.Context, subscriptionID string, atPeriodEnd bool, reason string) (json.RawMessage, error) {
	body := optionalText(map[string]any{"cancelAtPeriodEnd": atPeriodEnd}, "reason", reason)
	return s.data(ctx, "cancel", map[string]string{"subscriptionId": subscriptionID}, body, "")
}

// Pause pauses billing; resumeAt (ISO-8601) resumes it automatically, ""
// sends no body.
func (s *SubscriptionsClient) Pause(ctx context.Context, subscriptionID, resumeAt string) (json.RawMessage, error) {
	return s.data(ctx, "pause", map[string]string{"subscriptionId": subscriptionID}, optionalText(map[string]any{}, "resumeAt", resumeAt), "")
}

// Resume resumes a paused subscription.
func (s *SubscriptionsClient) Resume(ctx context.Context, subscriptionID string) (json.RawMessage, error) {
	return s.data(ctx, "resume", map[string]string{"subscriptionId": subscriptionID}, nil, "")
}

// ChangePlan moves a subscription to another plan: newPlanId, optional
// behavior (SubscriptionPlanChangeBehavior*, case-insensitive), reason. An
// Idempotency-Key (the argument or the request's idempotencyKey) is sent in
// the header and the body.
func (s *SubscriptionsClient) ChangePlan(ctx context.Context, subscriptionID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, err := requestObject("changePlan request", request)
	if err != nil {
		return nil, err
	}
	key, err := bodyKey("changePlan", body, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if key != "" {
		body["idempotencyKey"] = key
	}
	if err = upperChoice(body, "behavior", SubscriptionPlanChangeBehaviors()); err != nil {
		return nil, err
	}
	return s.data(ctx, "changePlan", map[string]string{"subscriptionId": subscriptionID}, body, key)
}

// AuditLogs lists the audit trail of a subscription. Filters: limit,
// pageToken.
func (s *SubscriptionsClient) AuditLogs(ctx context.Context, subscriptionID string, filters map[string]any) (*CursorPage, error) {
	return s.subscriptionList(ctx, "auditLogs", map[string]string{"subscriptionId": subscriptionID}, filters)
}

// SubscriptionPlans — plans, the templates a subscription is created
// against.
type SubscriptionPlans struct{ subs *SubscriptionsClient }

// Create creates a plan: productId, code, name, price (minor-unit string,
// "0" for a free plan), currency, billing {frequency, interval?,
// byMonthDay?, byWeekDay?, timezone?, lastDayOfMonthFix?, count?}
// (interval defaults to 1), optional description, trialPeriodDays,
// maxDunningAttempts, dunningScheduleDays, gracePeriodDays, metadata.
// idempotencyKey "" sends none.
func (p *SubscriptionPlans) Create(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, err := requestObject("plans.create request", request)
	if err != nil {
		return nil, err
	}
	for _, field := range []string{"productId", "code", "name"} {
		if err = trimmedText(body, field); err != nil {
			return nil, err
		}
	}
	if body["price"] == nil {
		return nil, invalid("plans.create requires price")
	}
	if currency, ok := body["currency"]; ok && currency != nil {
		if body["currency"], err = supportedCurrency(currency, "currency"); err != nil {
			return nil, err
		}
	}
	if rule, ok := body["billing"]; ok && rule != nil {
		if body["billing"], err = billingRule(rule); err != nil {
			return nil, err
		}
	}
	for _, field := range []string{"trialPeriodDays", "maxDunningAttempts", "gracePeriodDays"} {
		if err = countMember(body, field, 0, field); err != nil {
			return nil, err
		}
	}
	if days, ok := body["dunningScheduleDays"]; ok && days != nil {
		if body["dunningScheduleDays"], err = integerList(days, "dunningScheduleDays", 0, MaxSafeInteger); err != nil {
			return nil, err
		}
	}
	return p.subs.data(ctx, "plans.create", nil, body, idempotencyKey)
}

// Get reads one plan.
func (p *SubscriptionPlans) Get(ctx context.Context, planID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "plans.get", map[string]string{"planId": planID}, nil, "")
}

// List lists plans. Filters: productId, includeArchived, pageSize,
// pageToken.
func (p *SubscriptionPlans) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return p.subs.subscriptionList(ctx, "plans.list", nil, filters)
}

// Iterate yields every plan matching the filters.
func (p *SubscriptionPlans) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateToken(ctx, filters, "pageToken", p.List)
}

// Update changes a plan (price, billing, metadata …); a price is a
// minor-unit string.
func (p *SubscriptionPlans) Update(ctx context.Context, planID string, changes map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "plans.update", map[string]string{"planId": planID}, changes)
}

// Archive archives a plan.
func (p *SubscriptionPlans) Archive(ctx context.Context, planID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "plans.archive", map[string]string{"planId": planID}, nil, "")
}

// Translations reads the translations of a plan.
func (p *SubscriptionPlans) Translations(ctx context.Context, planID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "plans.translations", map[string]string{"planId": planID}, nil, "")
}

// UpsertTranslation writes the translation of a plan for one locale.
func (p *SubscriptionPlans) UpsertTranslation(ctx context.Context, planID, locale string, translation map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "plans.upsertTranslation", map[string]string{"planId": planID, "locale": locale}, translation)
}

// SubscriptionCustomers — customers, upserted by your own externalId (one
// stable customer per user).
type SubscriptionCustomers struct{ subs *SubscriptionsClient }

// Upsert creates or updates a customer: externalId, optional email, name,
// phone, locale, metadata. idempotencyKey "" sends none.
func (c *SubscriptionCustomers) Upsert(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, err := requestObject("customers.upsert request", request)
	if err != nil {
		return nil, err
	}
	return c.subs.data(ctx, "customers.upsert", nil, body, idempotencyKey)
}

// Get reads one customer.
func (c *SubscriptionCustomers) Get(ctx context.Context, customerID string) (json.RawMessage, error) {
	return c.subs.data(ctx, "customers.get", map[string]string{"customerId": customerID}, nil, "")
}

// GetByExternalID finds a customer by your own externalId (letters, digits
// and _ . : - only).
func (c *SubscriptionCustomers) GetByExternalID(ctx context.Context, externalID string) (json.RawMessage, error) {
	return c.subs.data(ctx, "customers.getByExternalId", map[string]string{"externalId": externalID}, nil, "")
}

// List lists customers. Filters: search, includeArchived, pageSize,
// pageToken.
func (c *SubscriptionCustomers) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return c.subs.subscriptionList(ctx, "customers.list", nil, filters)
}

// Iterate yields every customer matching the filters.
func (c *SubscriptionCustomers) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateToken(ctx, filters, "pageToken", c.List)
}

// CreditBalance reads the credit balance of a customer.
func (c *SubscriptionCustomers) CreditBalance(ctx context.Context, customerID string) (json.RawMessage, error) {
	return c.subs.data(ctx, "customers.creditBalance", map[string]string{"customerId": customerID}, nil, "")
}

// AdjustCredit grants or deducts credit: amountMinor (minor-unit string),
// currency, reason, type …
func (c *SubscriptionCustomers) AdjustCredit(ctx context.Context, customerID string, adjustment map[string]any) (json.RawMessage, error) {
	return c.subs.object(ctx, "customers.adjustCredit", map[string]string{"customerId": customerID}, adjustment)
}

// Archive archives a customer.
func (c *SubscriptionCustomers) Archive(ctx context.Context, customerID string) (json.RawMessage, error) {
	return c.subs.data(ctx, "customers.archive", map[string]string{"customerId": customerID}, nil, "")
}

// SubscriptionCards — saved cards for automatic charges. A card is added
// from the checkout SDK's paymentMethodToken; a card number or security
// code is never sent (those fields are refused, and the refusal names only
// the field).
type SubscriptionCards struct{ subs *SubscriptionsClient }

// Add saves a card: customerId, paymentMethodToken, optional brand, last4,
// bin, expireMonth, expireYear, cardholderName, isDefault, nickname,
// verificationMode (SubscriptionCardVerificationMode*),
// supportedCurrencies. idempotencyKey "" sends none.
func (c *SubscriptionCards) Add(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, err := requestObject("cards.add request", request)
	if err != nil {
		return nil, err
	}
	if err = choice(body, "verificationMode", SubscriptionCardVerificationModes()); err != nil {
		return nil, err
	}
	for _, field := range []string{"expireMonth", "expireYear"} {
		if err = countMember(body, field, 1, field); err != nil {
			return nil, err
		}
	}
	if err = booleanMember(body, "isDefault", "isDefault"); err != nil {
		return nil, err
	}
	if currencies, ok := body["supportedCurrencies"]; ok && currencies != nil {
		list, isList := currencies.([]any)
		if !isList {
			return nil, invalid("supportedCurrencies must be an array")
		}
		codes := make([]any, len(list))
		for i, entry := range list {
			if codes[i], err = supportedCurrency(entry, "supportedCurrencies"); err != nil {
				return nil, err
			}
		}
		body["supportedCurrencies"] = codes
	}
	return c.subs.data(ctx, "cards.add", nil, body, idempotencyKey)
}

// Get reads one saved card.
func (c *SubscriptionCards) Get(ctx context.Context, cardID string) (json.RawMessage, error) {
	return c.subs.data(ctx, "cards.get", map[string]string{"cardId": cardID}, nil, "")
}

// List lists a customer's cards. Filters: includeInactive (false when left
// out), pageSize, pageToken.
func (c *SubscriptionCards) List(ctx context.Context, customerID string, filters map[string]any) (*CursorPage, error) {
	id, err := requiredText(customerID, "customerId")
	if err != nil {
		return nil, err
	}
	query := maps.Clone(filters)
	if query == nil {
		query = map[string]any{}
	}
	if given, ok := query["customerId"]; ok && given != nil && given != id {
		return nil, invalid("cards.list takes the customer as its argument, not as the customerId filter")
	}
	query["customerId"] = id
	if query["includeInactive"] == nil {
		query["includeInactive"] = false
	}
	return c.subs.subscriptionList(ctx, "cards.list", nil, query)
}

// Iterate yields every card of a customer.
func (c *SubscriptionCards) Iterate(ctx context.Context, customerID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateToken(ctx, filters, "pageToken", func(ctx context.Context, current map[string]any) (*CursorPage, error) {
		return c.List(ctx, customerID, current)
	})
}

// SetDefault makes a card the customer's default card.
func (c *SubscriptionCards) SetDefault(ctx context.Context, cardID string) (json.RawMessage, error) {
	return c.subs.data(ctx, "cards.setDefault", map[string]string{"cardId": cardID}, nil, "")
}

// Deactivate deactivates a card; reason "" sends no body.
func (c *SubscriptionCards) Deactivate(ctx context.Context, cardID, reason string) (json.RawMessage, error) {
	return c.subs.data(ctx, "cards.deactivate", map[string]string{"cardId": cardID}, optionalText(map[string]any{}, "reason", reason), "")
}

// Verify verifies a saved card (a small authorisation or a 3-D check);
// options as the gateway documents them, nil for none.
func (c *SubscriptionCards) Verify(ctx context.Context, cardID string, options map[string]any) (json.RawMessage, error) {
	return c.subs.data(ctx, "cards.verify", map[string]string{"cardId": cardID}, orEmpty(options), "")
}

// Update changes card metadata (label, expiry, billing …); never a card
// number.
func (c *SubscriptionCards) Update(ctx context.Context, cardID string, changes map[string]any) (json.RawMessage, error) {
	return c.subs.object(ctx, "cards.update", map[string]string{"cardId": cardID}, changes)
}

// SubscriptionInvoices — invoices: billing history, void and charge retry.
type SubscriptionInvoices struct{ subs *SubscriptionsClient }

// List lists invoices. Filters: subscriptionId, status, pageSize,
// pageToken.
func (i *SubscriptionInvoices) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return i.subs.subscriptionList(ctx, "invoices.list", nil, filters)
}

// Iterate yields every invoice matching the filters.
func (i *SubscriptionInvoices) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateToken(ctx, filters, "pageToken", i.List)
}

// Get reads one invoice.
func (i *SubscriptionInvoices) Get(ctx context.Context, invoiceID string) (json.RawMessage, error) {
	return i.subs.data(ctx, "invoices.get", map[string]string{"invoiceId": invoiceID}, nil, "")
}

// Void voids an invoice; reason "" sends no body.
func (i *SubscriptionInvoices) Void(ctx context.Context, invoiceID, reason string) (json.RawMessage, error) {
	return i.subs.data(ctx, "invoices.void", map[string]string{"invoiceId": invoiceID}, optionalText(map[string]any{}, "reason", reason), "")
}

// RetryCharge retries the charge of an open invoice; idempotencyKey ""
// sends none.
func (i *SubscriptionInvoices) RetryCharge(ctx context.Context, invoiceID, idempotencyKey string) (json.RawMessage, error) {
	return i.subs.data(ctx, "invoices.retryCharge", map[string]string{"invoiceId": invoiceID}, nil, idempotencyKey)
}

// SubscriptionPortal — the customer portal. Embedded: CreateSdkSession
// mints an origin-bound session for the browser portal SDK (origin must be
// allow-listed). Hosted: CreatePortalLink returns a link whose token opens
// the hosted portal (SubscriptionPortalURL); the customer verifies with a
// one-time code.
type SubscriptionPortal struct{ subs *SubscriptionsClient }

// SubscriptionPortalSessionOptions are the optional fields of a portal SDK
// session.
type SubscriptionPortalSessionOptions struct {
	// SdkMode is a SubscriptionPortalSdkMode* value; "" sends none.
	SdkMode string
	// PortalLinkID binds the session to a portal link; "" sends none.
	PortalLinkID string
}

// CreateSdkSession mints a portal SDK session. The gateway answers at the
// envelope root; the result merges the root and data ({portalToken,
// sessionProof, proofNonce, proofExpiresAt, …}, proofNonce and
// proofExpiresAt read from nonce and expiresAt when only those are sent).
// Hand those four to the browser.
func (p *SubscriptionPortal) CreateSdkSession(ctx context.Context, customerID, origin string, options SubscriptionPortalSessionOptions) (json.RawMessage, error) {
	customer, err := requiredText(customerID, "customerId")
	if err != nil {
		return nil, err
	}
	allowed, err := requiredText(origin, "origin")
	if err != nil {
		return nil, err
	}
	body := map[string]any{"customerId": customer, "origin": allowed}
	if options.SdkMode != "" {
		if err = oneOf(options.SdkMode, SubscriptionPortalSdkModes(), "sdkMode"); err != nil {
			return nil, err
		}
		body["sdkMode"] = options.SdkMode
	}
	optionalText(body, "portalLinkId", options.PortalLinkID)
	r, err := p.subs.call(ctx, "portal.createSdkSession", nil, nil, body, "")
	if err != nil {
		return nil, err
	}
	session := objectFields(mergedRoot(r))
	if _, ok := session["proofNonce"]; !ok && session["nonce"] != nil {
		session["proofNonce"] = session["nonce"]
	}
	if _, ok := session["proofExpiresAt"]; !ok && session["expiresAt"] != nil {
		session["proofExpiresAt"] = session["expiresAt"]
	}
	return json.Marshal(session)
}

// CreatePortalLink creates a hosted portal link for a customer. options:
// allowedPlanIds, defaultPlanId, permissions, sessionDurationMinutes,
// expiresAt, requireEmailVerification …
func (p *SubscriptionPortal) CreatePortalLink(ctx context.Context, customerID string, options map[string]any) (json.RawMessage, error) {
	customer, err := requiredText(customerID, "customerId")
	if err != nil {
		return nil, err
	}
	body := maps.Clone(orEmpty(options))
	body["customerId"] = customer
	return p.subs.data(ctx, "portal.createPortalLink", nil, body, "")
}

// ListLinks lists portal links. Filters: customerId, status, limit.
func (p *SubscriptionPortal) ListLinks(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return p.subs.page(ctx, "portal.listLinks", nil, filters)
}

// GetLink reads one portal link.
func (p *SubscriptionPortal) GetLink(ctx context.Context, linkID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "portal.getLink", map[string]string{"linkId": linkID}, nil, "")
}

// UpdateLink changes a portal link.
func (p *SubscriptionPortal) UpdateLink(ctx context.Context, linkID string, changes map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "portal.updateLink", map[string]string{"linkId": linkID}, changes)
}

// RevokeLink revokes a portal link; reason "" sends no body.
func (p *SubscriptionPortal) RevokeLink(ctx context.Context, linkID, reason string) (json.RawMessage, error) {
	return p.subs.data(ctx, "portal.revokeLink", map[string]string{"linkId": linkID}, optionalText(map[string]any{}, "reason", reason), "")
}

// LinkActivity reads the activity of a portal link.
func (p *SubscriptionPortal) LinkActivity(ctx context.Context, linkID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "portal.linkActivity", map[string]string{"linkId": linkID}, nil, "")
}

// SendInvite emails a portal invite: customerId, optional email,
// allowedPlanIds …
func (p *SubscriptionPortal) SendInvite(ctx context.Context, invite map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "portal.sendInvite", nil, invite)
}

// GetSettings reads the portal settings (branding, allowed origins).
func (p *SubscriptionPortal) GetSettings(ctx context.Context) (json.RawMessage, error) {
	return p.subs.data(ctx, "portal.getSettings", nil, nil, "")
}

// UpdateSettings writes the portal settings.
func (p *SubscriptionPortal) UpdateSettings(ctx context.Context, settings map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "portal.updateSettings", nil, settings)
}

// SubscriptionProducts — catalog products (groups of plans).
type SubscriptionProducts struct{ subs *SubscriptionsClient }

// Create creates a catalog product.
func (p *SubscriptionProducts) Create(ctx context.Context, product map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "products.create", nil, product)
}

// List lists catalog products. Filters: limit, cursor, active.
func (p *SubscriptionProducts) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return p.subs.page(ctx, "products.list", nil, filters)
}

// Iterate yields every catalog product matching the filters.
func (p *SubscriptionProducts) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, p.List)
}

// Get reads one catalog product.
func (p *SubscriptionProducts) Get(ctx context.Context, productID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "products.get", map[string]string{"productId": productID}, nil, "")
}

// Update changes a catalog product.
func (p *SubscriptionProducts) Update(ctx context.Context, productID string, changes map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "products.update", map[string]string{"productId": productID}, changes)
}

// Archive archives a catalog product.
func (p *SubscriptionProducts) Archive(ctx context.Context, productID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "products.archive", map[string]string{"productId": productID}, nil, "")
}

// Translations reads the translations of a catalog product.
func (p *SubscriptionProducts) Translations(ctx context.Context, productID string) (json.RawMessage, error) {
	return p.subs.data(ctx, "products.translations", map[string]string{"productId": productID}, nil, "")
}

// UpsertTranslation writes the translation of a catalog product for one
// locale.
func (p *SubscriptionProducts) UpsertTranslation(ctx context.Context, productID, locale string, translation map[string]any) (json.RawMessage, error) {
	return p.subs.object(ctx, "products.upsertTranslation", map[string]string{"productId": productID, "locale": locale}, translation)
}

// SubscriptionFeatures — entitlement features.
type SubscriptionFeatures struct{ subs *SubscriptionsClient }

// Create creates a feature.
func (f *SubscriptionFeatures) Create(ctx context.Context, feature map[string]any) (json.RawMessage, error) {
	return f.subs.object(ctx, "features.create", nil, feature)
}

// List lists features; any text, integer or boolean filter is passed on.
func (f *SubscriptionFeatures) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return f.subs.page(ctx, "features.list", nil, filters)
}

// Get reads one feature.
func (f *SubscriptionFeatures) Get(ctx context.Context, featureID string) (json.RawMessage, error) {
	return f.subs.data(ctx, "features.get", map[string]string{"featureId": featureID}, nil, "")
}

// Update changes a feature.
func (f *SubscriptionFeatures) Update(ctx context.Context, featureID string, changes map[string]any) (json.RawMessage, error) {
	return f.subs.object(ctx, "features.update", map[string]string{"featureId": featureID}, changes)
}

// Delete deletes a feature.
func (f *SubscriptionFeatures) Delete(ctx context.Context, featureID string) (json.RawMessage, error) {
	return f.subs.data(ctx, "features.delete", map[string]string{"featureId": featureID}, nil, "")
}

// Translations reads the translations of a feature.
func (f *SubscriptionFeatures) Translations(ctx context.Context, featureID string) (json.RawMessage, error) {
	return f.subs.data(ctx, "features.translations", map[string]string{"featureId": featureID}, nil, "")
}

// UpsertTranslation writes the translation of a feature for one locale.
func (f *SubscriptionFeatures) UpsertTranslation(ctx context.Context, featureID, locale string, translation map[string]any) (json.RawMessage, error) {
	return f.subs.object(ctx, "features.upsertTranslation", map[string]string{"featureId": featureID, "locale": locale}, translation)
}

// SubscriptionAddons — add-ons.
type SubscriptionAddons struct{ subs *SubscriptionsClient }

// Create creates an add-on.
func (a *SubscriptionAddons) Create(ctx context.Context, addon map[string]any) (json.RawMessage, error) {
	return a.subs.object(ctx, "addons.create", nil, addon)
}

// List lists add-ons; any text, integer or boolean filter is passed on.
func (a *SubscriptionAddons) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return a.subs.page(ctx, "addons.list", nil, filters)
}

// Update changes an add-on.
func (a *SubscriptionAddons) Update(ctx context.Context, addonID string, changes map[string]any) (json.RawMessage, error) {
	return a.subs.object(ctx, "addons.update", map[string]string{"addonId": addonID}, changes)
}

// Deactivate deactivates an add-on.
func (a *SubscriptionAddons) Deactivate(ctx context.Context, addonID string) (json.RawMessage, error) {
	return a.subs.data(ctx, "addons.deactivate", map[string]string{"addonId": addonID}, nil, "")
}

// SubscriptionUsage — metered usage and entitlement checks.
type SubscriptionUsage struct{ subs *SubscriptionsClient }

// Record records a metered-usage event: customerId or subscriptionId,
// featureKey, quantity, optional timestamp, idempotencyKey …
func (u *SubscriptionUsage) Record(ctx context.Context, usage map[string]any) (json.RawMessage, error) {
	return u.subs.object(ctx, "usage.record", nil, usage)
}

// CheckEntitlement reports whether the customer is entitled or has quota
// left: customerId or subscriptionId, featureKey, optional requested …
func (u *SubscriptionUsage) CheckEntitlement(ctx context.Context, check map[string]any) (json.RawMessage, error) {
	return u.subs.object(ctx, "usage.checkEntitlement", nil, check)
}

// ── Route invoker ────────────────────────────────────────────────────────

func (s *SubscriptionsClient) call(ctx context.Context, name string, params map[string]string, query, body map[string]any, key string) (*Response, error) {
	return sendTableRoute(ctx, s.client, subscriptionRoutes, name, tableCall{params: params, query: query, body: body, key: key})
}

func (s *SubscriptionsClient) data(ctx context.Context, name string, params map[string]string, body map[string]any, key string) (json.RawMessage, error) {
	r, err := s.call(ctx, name, params, nil, body, key)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// object sends a Free body that must be given (nil is refused).
func (s *SubscriptionsClient) object(ctx context.Context, name string, params map[string]string, body map[string]any) (json.RawMessage, error) {
	if body == nil {
		return nil, invalid(name + " body must be an object")
	}
	return s.data(ctx, name, params, body, "")
}

func (s *SubscriptionsClient) page(ctx context.Context, name string, params map[string]string, filters map[string]any) (*CursorPage, error) {
	r, err := s.call(ctx, name, params, orEmpty(filters), nil, "")
	if err != nil {
		return nil, err
	}
	return tablePage(r), nil
}

func (s *SubscriptionsClient) subscriptionList(ctx context.Context, name string, params map[string]string, filters map[string]any) (*CursorPage, error) {
	r, err := s.call(ctx, name, params, orEmpty(filters), nil, "")
	if err != nil {
		return nil, err
	}
	return subscriptionPage(r), nil
}

// billingRule checks a plan's billing rule: known fields only, frequency
// upper-cased from SubscriptionBillingFrequencies, interval (default 1) and
// count integers, byMonthDay 1–31, byWeekDay week-day codes,
// lastDayOfMonthFix a boolean.
func billingRule(value any) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid("billing must be an object")
	}
	rule := compactNil(object)
	if err := checkFieldNames("billing", rule, nil, subscriptionBillingRuleFields, presentValue); err != nil {
		return nil, err
	}
	frequency, _ := rule["frequency"].(string)
	rule["frequency"] = strings.ToUpper(strings.TrimSpace(frequency))
	if err := oneOf(rule["frequency"].(string), SubscriptionBillingFrequencies(), "billing.frequency"); err != nil {
		return nil, err
	}
	if _, ok := rule["interval"]; !ok {
		rule["interval"] = 1
	}
	if err := countMember(rule, "interval", 1, "billing.interval"); err != nil {
		return nil, err
	}
	if err := countMember(rule, "count", 0, "billing.count"); err != nil {
		return nil, err
	}
	if days, ok := rule["byMonthDay"]; ok {
		list, err := integerList(days, "billing.byMonthDay", 1, 31)
		if err != nil {
			return nil, err
		}
		rule["byMonthDay"] = list
	}
	if days, ok := rule["byWeekDay"]; ok {
		list, isList := days.([]any)
		for _, day := range list {
			if _, isText := day.(string); !isText {
				isList = false
			}
		}
		if !isList {
			return nil, invalid("billing.byWeekDay must be an array of week days (MO, TU, WE, TH, FR, SA, SU)")
		}
	}
	if err := booleanMember(rule, "lastDayOfMonthFix", "billing.lastDayOfMonthFix"); err != nil {
		return nil, err
	}
	return rule, nil
}

package lunixi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The caller of the subscription, KYC, identity and marketplace route
// tables (SubscriptionRoutes, KycRoutes, IdentityRoutes, MarketplaceRoutes).
// Every check runs before anything leaves the process and fails with
// ErrConfiguration naming the field, never its value (card data and
// document images included):
//
//   - the ":name" path segments follow the resource id rule;
//   - a body is checked against Required / Optional (unknown or missing
//     fields), or taken as an object for a Free route; a route without
//     either takes no body;
//   - filters are checked against Query (any scalar filter with AnyQuery);
//   - the Idempotency-Key follows KeyRule;
//   - on the subscription and marketplace contracts, every money member of
//     the body, the filters and the answer is a canonical minor-unit string.
//
// StepUp routes are signed per request; the others carry the access token
// only.

// subscriptionPortalPanelBase is the one panel family an API key reaches:
// the hosted customer-portal links of subscriptions.
const subscriptionPortalPanelBase = "/api/panel/v1/subscriptions/portal"

// routeTableKeyMax is the longest Idempotency-Key of the route-table
// families (1–200 printable ASCII characters without spaces).
const routeTableKeyMax = 200

// routeTableKeyRoute: the paths whose keys follow validRouteTableKey.
func routeTableKeyRoute(path string) bool {
	for _, base := range []string{"/api/v1/subscriptions", "/api/v1/kyc", "/api/v1/identity", "/api/v1/marketplace", subscriptionPortalPanelBase} {
		if path == base || strings.HasPrefix(path, base+"/") {
			return true
		}
	}
	return false
}

func validRouteTableKey(key string) bool { return safeHeader(key, routeTableKeyMax) }

// tableCall is one call of a route table. A nil query sends no filters; a
// nil body sends none.
type tableCall struct {
	params map[string]string
	query  map[string]any
	body   map[string]any
	key    string
}

// sendTableRoute sends one route of a table after the checks above.
func sendTableRoute(ctx context.Context, c *Client, table map[string]Route, name string, call tableCall) (*Response, error) {
	route, ok := table[name]
	if !ok {
		return nil, invalid("unknown route " + name)
	}
	path, err := fillRoutePath(route.Path, call.params)
	if err != nil {
		return nil, err
	}
	key, err := tableKey(name, route, call.key)
	if err != nil {
		return nil, err
	}
	var filters map[string]any
	if call.query != nil {
		if filters, err = tableFilters(name, route, call.query); err != nil {
			return nil, err
		}
	}
	payload, plain, err := tableBody(name, route, call.body)
	if err != nil {
		return nil, err
	}
	money := wireMoneyRoute(route.Path)
	if money {
		if err = validateWireMoney(plain); err != nil {
			return nil, err
		}
		if err = validateWireMoney(filters); err != nil {
			return nil, err
		}
	}
	request := Request{Method: route.Method, Path: path, Body: payload, IdempotencyKey: key}
	if len(filters) > 0 {
		query := url.Values{}
		for field, value := range filters {
			text, err := queryText(field, value)
			if err != nil {
				return nil, err
			}
			query.Set(field, text)
		}
		request.Query = query
	}
	r, err := c.send(ctx, request, sendOptions{unsigned: !route.StepUp, deleteBody: route.Method == "DELETE"})
	if err != nil {
		return nil, err
	}
	if money && len(r.Envelope) > 0 {
		var answer any
		if decodeNumbers(r.Envelope, &answer) == nil {
			if err = validateWireMoney(answer); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// tableKey applies the route's Idempotency-Key rule; the key is trimmed.
// Its format is checked by the transport (validIdempotencyFor).
func tableKey(name string, route Route, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		if route.IdempotencyKey {
			return "", invalid("a stable Idempotency-Key is required for " + name + ". Reuse the same key when retrying the same operation")
		}
		return "", nil
	}
	if route.KeyRule() == "none" {
		return "", invalid(name + " does not take an Idempotency-Key")
	}
	return key, nil
}

// bodyKey is the request's own idempotencyKey or the argument; both given,
// they must agree.
func bodyKey(name string, body map[string]any, key string) (string, error) {
	own := ""
	if text, ok := body["idempotencyKey"].(string); ok {
		own = strings.TrimSpace(text)
	}
	given := strings.TrimSpace(key)
	if own != "" && given != "" && own != given {
		return "", invalid(name + ": the request's idempotencyKey and the idempotencyKey argument differ")
	}
	if given != "" {
		return given, nil
	}
	return own, nil
}

// tableBody returns what is sent (nil for no body) and the plain object the
// money check reads.
func tableBody(name string, route Route, body map[string]any) (any, map[string]any, error) {
	if len(route.Required) == 0 && len(route.Optional) == 0 && !route.Free {
		if len(body) > 0 {
			return nil, nil, invalid(name + " takes no body")
		}
		return nil, nil, nil
	}
	object, err := plainObject(body, name+" body")
	if err != nil {
		return nil, nil, err
	}
	plain := compactNil(object)
	if !route.Free {
		if err = checkFieldNames(name, plain, route.Required, route.Optional, presentNonEmpty); err != nil {
			return nil, nil, err
		}
	}
	if route.OptionalBody && len(plain) == 0 {
		return nil, plain, nil
	}
	if route.Free {
		return plain, plain, nil
	}
	return orderedFields(plain, route.Fields()), plain, nil
}

// tableFilters checks the filters against the route: nil and "" are left
// out; values are text, booleans or integers.
func tableFilters(name string, route Route, filters map[string]any) (map[string]any, error) {
	given := compactNil(filters)
	if !route.AnyQuery {
		var unknown []string
		for field := range given {
			if !slices.Contains(route.Query, field) {
				unknown = append(unknown, field)
			}
		}
		if len(unknown) > 0 {
			slices.Sort(unknown)
			message := name + " does not accept the filter(s) " + strings.Join(unknown, ", ")
			if len(route.Query) > 0 {
				message += "; it takes " + strings.Join(route.Query, ", ")
			}
			return nil, invalid(message)
		}
	}
	out := make(map[string]any, len(given))
	for field, value := range given {
		if text, ok := value.(string); ok && text == "" {
			continue
		}
		switch value.(type) {
		case string, bool, json.Number:
		default:
			if _, ok := integerOf(value); !ok {
				return nil, invalid(name + " filter " + field + " must be text, an integer or a boolean")
			}
		}
		out[field] = value
	}
	return out, nil
}

// ── Money of the subscription and marketplace contracts ──────────────────

// wireMoneyRoute: the subscription and marketplace contracts carry money
// members as canonical minor-unit strings.
var wireMoneyRoutePattern = regexp.MustCompile(`^/api/v1/(?:subscriptions|marketplace)(?:/|$)`)

func wireMoneyRoute(path string) bool { return wireMoneyRoutePattern.MatchString(path) }

// wireMoneyFields are the money members of those contracts.
var wireMoneyFields = []string{
	"amount", "amountMinor", "price", "priceAmount", "paidAmount", "totalAmount", "subtotalAmount", "taxAmount", "amountPaid",
	"legAmount", "chargeAmount", "capturedAmount", "refundedAmount", "authorizedAmount", "compensatedAmount",
	"requestedAmount", "refundableAmount", "nonRefundableAmount", "disputedAmount", "legsAmount",
	"minAmount", "maxAmount", "minAmountMinor", "maxAmountMinor", "unitPriceMinor", "collectedAmountMinor",
	"premiumAmount", "collectedPremiumAmount", "inForcePremiumAmount", "cancelledPremiumAmount",
	"refundedPremiumAmount", "refundOpenPremiumAmount", "insuranceValueAmount", "priceSnapshotAmount",
	"sourceAmountMinor", "targetAmountMinor", "taxAmountMinor", "balanceMinor", "balanceAfterMinor",
	"availableMinor", "blockedMinor", "availableWithoutOverdraftMinor", "overdraftAvailableMinor",
	"sourceBalanceAfterMinor", "targetBalanceAfterMinor", "balance", "availableBalance", "blockedBalance",
	"feeAmount", "feeAmountMinor", "fixedAmountMinor", "minTransactionMinor", "commissionAmountMinor", "payoutAmountMinor",
	"settlementAmountMinor", "netAmountMinor", "grossAmountMinor", "fixedAmount", "fixedFee", "fixedFeeMinor", "mtdVolumeMinor",
	"orderAmountMinor", "capturedAmountMinor", "cumulativeCapturedMinor", "remainingCapturableMinor",
	"refundAmountMinor", "cumulativeRefundedMinor", "remainingRefundableMinor", "netAmount", "grossAmount",
}

// wireMoneyContainers are the closed schema objects walked for money
// members; metadata, custom fields and provider bodies are opaque.
var wireMoneyContainers = []string{
	"data", "items", "legs", "steps", "pendingSteps", "schedule", "promissoryNote", "notes",
	"aggregate", "refund", "dispute", "details", "payment", "basketItems", "sellers", "scenario",
	"counters", "balances", "totals", "orders", "policies", "options", "quote", "order", "transfer",
	"commission", "commissionRule", "fee", "settlement", "payout", "context",
}

// validateWireMoney checks a JSON-form value (bodies, filters, answers) the
// same way in both directions, without coercion: every money member is a
// canonical minor-unit string ("0" allowed), signed only for balances.
func validateWireMoney(value any) error {
	switch v := value.(type) {
	case []any:
		for _, entry := range v {
			if err := validateWireMoney(entry); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, entry := range v {
			if entry == nil {
				continue
			}
			switch {
			case slices.Contains(wireMoneyFields, key):
				signed := strings.Contains(strings.ToLower(key), "balance") || key == "availableMinor" || key == "availableWithoutOverdraftMinor"
				if text, ok := entry.(string); !ok || !validMinor(text, false, signed) {
					return invalid(key + " must be a canonical minor-unit string")
				}
			case key == "suggestedAmountsMinor":
				amounts, ok := entry.([]any)
				if !ok {
					return invalid("suggestedAmountsMinor must be an array")
				}
				for _, amount := range amounts {
					if text, ok := amount.(string); !ok || !validMinor(text, true, false) {
						return invalid("suggestedAmountsMinor must hold canonical minor-unit strings")
					}
				}
			case slices.Contains(wireMoneyContainers, key):
				if err := validateWireMoney(entry); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ── Answers ──────────────────────────────────────────────────────────────

// tablePage reads every list shape of the route-table products: data as the
// row array, or data.items / operations / rows; the cursor in pageInfo (in
// data or at the root; a pageInfo that names no cursor is the last page),
// else nextCursor or nextPageToken (in data or at the root). The cursor is
// opaque: send it back unchanged.
func tablePage(r *Response) *CursorPage {
	page := &CursorPage{Items: []json.RawMessage{}, Raw: r.Data, RequestID: r.RequestID}
	root := objectFields(r.Envelope)
	container := objectFields(r.Data)
	var rows []json.RawMessage
	if json.Unmarshal(r.Data, &rows) == nil && rows != nil {
		page.Items = jsonObjects(rows)
	} else {
		for _, field := range []string{"items", "operations", "rows"} {
			if json.Unmarshal(container[field], &rows) == nil && rows != nil {
				page.Items = jsonObjects(rows)
				break
			}
		}
	}
	info := objectFields(container["pageInfo"])
	if info == nil {
		info = objectFields(root["pageInfo"])
	}
	_, namesNext := info["nextCursor"]
	_, namesEnd := info["endCursor"]
	switch {
	case stringField(info, "nextCursor") != "":
		page.NextCursor = stringField(info, "nextCursor")
	case stringField(info, "endCursor") != "":
		page.NextCursor = stringField(info, "endCursor")
	case info == nil || (!namesNext && !namesEnd):
		for _, candidate := range []string{stringField(container, "nextCursor"), stringField(container, "nextPageToken"), stringField(root, "nextPageToken"), stringField(root, "nextCursor")} {
			if candidate != "" {
				page.NextCursor = candidate
				break
			}
		}
	}
	var hasMore bool
	if raw, ok := info["hasMore"]; ok && json.Unmarshal(raw, &hasMore) == nil {
		page.HasMore = hasMore && page.NextCursor != ""
	} else {
		page.HasMore = page.NextCursor != ""
	}
	for _, raw := range []json.RawMessage{info["totalCount"], container["totalCount"], container["total"]} {
		if total := posSafeInteger(raw); total != nil {
			page.TotalCount = total
			break
		}
	}
	return page
}

// subscriptionPage is a subscription, plan, customer, card, invoice or
// audit-log list: tablePage plus the page size at the envelope root (the
// row count when it is missing). NextCursor is the nextPageToken.
func subscriptionPage(r *Response) *CursorPage {
	page := tablePage(r)
	page.PageSize = posSafeInteger(objectFields(r.Envelope)["pageSize"])
	if page.PageSize == nil {
		count := int64(len(page.Items))
		page.PageSize = &count
	}
	return page
}

func jsonObjects(rows []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		if trimmed := bytes.TrimSpace(row); len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			out = append(out, row)
		}
	}
	return out
}

// mergedRoot is an answer that puts its fields at the envelope root and/or
// under data: both, data winning.
func mergedRoot(r *Response) json.RawMessage {
	merged := map[string]json.RawMessage{}
	for key, value := range objectFields(r.Envelope) {
		if key != "data" {
			merged[key] = value
		}
	}
	for key, value := range objectFields(r.Data) {
		merged[key] = value
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

// ── Field checks ─────────────────────────────────────────────────────────

// requestObject is a request map as its JSON form; nil is refused.
func requestObject(label string, request map[string]any) (map[string]any, error) {
	if request == nil {
		return nil, invalid(label + " must be an object")
	}
	return plainObject(request, label)
}

// choice: value (when not nil) is one of allowed. The error never shows the
// value.
func choice(body map[string]any, field string, allowed []string) error {
	value, ok := body[field]
	if !ok || value == nil {
		return nil
	}
	text, isText := value.(string)
	if !isText {
		return invalid(field + " must be one of " + strings.Join(allowed, ", "))
	}
	return oneOf(text, allowed, field)
}

// upperChoice is choice after upper-casing the text (sent upper-cased).
func upperChoice(body map[string]any, field string, allowed []string) error {
	if text, ok := body[field].(string); ok {
		body[field] = strings.ToUpper(strings.TrimSpace(text))
	}
	return choice(body, field, allowed)
}

func booleanMember(body map[string]any, field, label string) error {
	if value, ok := body[field]; ok && value != nil {
		if _, isBool := value.(bool); !isBool {
			return invalid(label + " must be a boolean")
		}
	}
	return nil
}

func objectMember(body map[string]any, field string) error {
	if value, ok := body[field]; ok && value != nil {
		if _, isObject := value.(map[string]any); !isObject {
			return invalid(field + " must be an object")
		}
	}
	return nil
}

// countMember: an integer of at least min, when present.
func countMember(body map[string]any, field string, min int64, label string) error {
	value, ok := body[field]
	if !ok || value == nil {
		return nil
	}
	if n, isInt := integerOf(value); !isInt || n < min {
		return invalid(label + " must be an integer of at least " + formatInt(min))
	}
	return nil
}

// integerList: an array of integers from min to max.
func integerList(value any, label string, min, max int64) ([]any, error) {
	list, ok := value.([]any)
	if ok {
		for _, entry := range list {
			if n, isInt := integerOf(entry); !isInt || n < min || n > max {
				ok = false
				break
			}
		}
	}
	if !ok {
		message := label + " must be an array of integers from " + formatInt(min)
		if max != MaxSafeInteger {
			message += " to " + formatInt(max)
		}
		return nil, invalid(message)
	}
	return list, nil
}

// gatewayCurrencies are the currencies the gateway accepts.
var gatewayCurrencies = []string{"JPY", "KWD", "TRY", "USD", "EUR", "GBP", "RUB", "AED", "SAR", "QAR", "CHF", "CAD", "AUD", "CNY", "AZN"}

// supportedCurrency trims and upper-cases a gateway currency.
func supportedCurrency(value any, field string) (string, error) {
	text, _ := value.(string)
	currency := strings.ToUpper(strings.TrimSpace(text))
	if !slices.Contains(gatewayCurrencies, currency) {
		return "", invalid(field + " must be a supported currency (" + strings.Join(gatewayCurrencies, ", ") + ")")
	}
	return currency, nil
}

// trimmedText sets body[field] to its trimmed text when present; a value
// that is not non-blank text is refused.
func trimmedText(body map[string]any, field string) error {
	value, ok := body[field]
	if !ok || value == nil {
		return nil
	}
	text, err := requiredText(value, field)
	if err != nil {
		return err
	}
	body[field] = text
	return nil
}

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }

package lunixi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"iter"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Shared helpers of the table-driven clients (WalletClient, SanctionClient,
// BankTransferClient, BankFXClient, SubscriptionsClient, KycClient,
// IdentityClient, MarketplaceClient) and of the cursor-paged product clients
// (BankConnectClient, InsuranceClient).

// Route is one gateway route of a table-driven client (WalletRoutes,
// SanctionRoutes, BankTransferRoutes, SubscriptionRoutes, KycRoutes,
// IdentityRoutes, MarketplaceRoutes). Path is the template; ":name"
// segments are filled from the method arguments. IdempotencyKey reports that
// the gateway refuses the call without an Idempotency-Key. Required and
// Optional are the body fields the gateway accepts (any other is refused
// before sending); AnyOf names fields of which at least one must be present;
// Query lists the accepted query parameters. Binary: a 2xx answer is a file
// (an image), not the JSON envelope. Screening: the answer is read only when
// the envelope's status is "success" (sanction screening).
//
// The remaining fields are read by the subscription, KYC, identity and
// marketplace clients. StepUp: the call carries the per-request Ed25519
// signature; their other routes are sent with the access token only (every
// other client signs every call). OptionalIdempotencyKey: an Idempotency-Key
// may be sent (IdempotencyKey: it must be). Free: the body is an object the
// gateway validates, without a field list. OptionalBody: an empty body is
// sent as no body. AnyQuery: any scalar filter is accepted.
type Route struct {
	Method                 string
	Path                   string
	IdempotencyKey         bool
	Required               []string
	Optional               []string
	AnyOf                  []string
	Query                  []string
	Binary                 bool
	Screening              bool
	StepUp                 bool
	OptionalIdempotencyKey bool
	Free                   bool
	OptionalBody           bool
	AnyQuery               bool
}

// KeyRule is the route's Idempotency-Key rule: "required", "optional" or
// "none".
func (r Route) KeyRule() string {
	switch {
	case r.IdempotencyKey:
		return "required"
	case r.OptionalIdempotencyKey:
		return "optional"
	}
	return "none"
}

// Fields returns Required followed by Optional.
func (r Route) Fields() []string { return append(slices.Clone(r.Required), r.Optional...) }

func (r Route) clone() Route {
	r.Required = slices.Clone(r.Required)
	r.Optional = slices.Clone(r.Optional)
	r.AnyOf = slices.Clone(r.AnyOf)
	r.Query = slices.Clone(r.Query)
	return r
}

func cloneRoutes(table map[string]Route) map[string]Route {
	out := make(map[string]Route, len(table))
	for name, route := range table {
		out[name] = route.clone()
	}
	return out
}

// NestedFields are the fields of the rows of a list field (wallet rows and
// payout items, sanction identifiers, parties and accounts).
type NestedFields struct {
	Required []string
	Optional []string
}

func cloneNested(table map[string]NestedFields) map[string]NestedFields {
	out := make(map[string]NestedFields, len(table))
	for name, spec := range table {
		out[name] = NestedFields{Required: slices.Clone(spec.Required), Optional: slices.Clone(spec.Optional)}
	}
	return out
}

func cloneLists(table map[string][]string) map[string][]string {
	out := make(map[string][]string, len(table))
	for name, list := range table {
		out[name] = slices.Clone(list)
	}
	return out
}

// CursorPage is one page of a cursor-paged list (wallet, bank accounts and
// transfers, sanction, insurance, subscriptions, KYC, identity,
// marketplace). Items are the rows in wire order. Send NextCursor back
// verbatim as the cursor filter (pageToken on the subscription, plan,
// customer, card, invoice and audit-log lists); HasMore is true only with a
// non-empty cursor. TotalCount is set when the list reports it. PageSize is
// the page size the subscription lists report (the row count when they do
// not), nil on the other lists. Raw is the whole data object of the answer
// (wallet and sanction lists carry other fields next to the rows, such as
// runsToday on the screening history).
type CursorPage struct {
	Items      []json.RawMessage
	HasMore    bool
	NextCursor string
	TotalCount *int64
	PageSize   *int64
	Raw        json.RawMessage
	RequestID  string
}

// iterateCursor follows the cursor of a list lazily. A failed page is
// yielded once as the error and ends the sequence.
func iterateCursor(ctx context.Context, filters map[string]any, list func(context.Context, map[string]any) (*CursorPage, error)) iter.Seq2[json.RawMessage, error] {
	return iterateToken(ctx, filters, "cursor", list)
}

// iterateToken is iterateCursor for a list whose cursor filter is named
// tokenFilter (pageToken on the subscription lists).
func iterateToken(ctx context.Context, filters map[string]any, tokenFilter string, list func(context.Context, map[string]any) (*CursorPage, error)) iter.Seq2[json.RawMessage, error] {
	return func(yield func(json.RawMessage, error) bool) {
		current := maps.Clone(filters)
		for {
			page, err := list(ctx, current)
			if err != nil {
				yield(nil, err)
				return
			}
			for _, item := range page.Items {
				if !yield(item, nil) {
					return
				}
			}
			if !page.HasMore || page.NextCursor == "" {
				return
			}
			current = maps.Clone(current)
			if current == nil {
				current = map[string]any{}
			}
			current[tokenFilter] = page.NextCursor
		}
	}
}

// envelopePage reads a list whose rows are data.items and whose paging is
// the envelope's top-level pageInfo (a copy inside data is the fallback).
func envelopePage(r *Response) *CursorPage {
	page := &CursorPage{Items: []json.RawMessage{}, Raw: r.Data, RequestID: r.RequestID}
	data := objectFields(r.Data)
	var items []json.RawMessage
	if json.Unmarshal(data["items"], &items) == nil && items != nil {
		page.Items = items
	}
	info := pageInfoOf(r, data)
	page.NextCursor = stringField(info, "nextCursor")
	page.HasMore = boolField(info, "hasMore") && page.NextCursor != ""
	page.TotalCount = posSafeInteger(info["totalCount"])
	return page
}

// dataCursorPage reads a list that carries its cursor inside data
// ({items, nextCursor}) or in the envelope's pageInfo. With firstArray, a
// data object without items yields its first array field (wallet operations
// answer {operations, nextCursor}). HasMore follows pageInfo.hasMore when it
// is a boolean, otherwise the presence of a cursor.
func dataCursorPage(r *Response, firstArray bool) *CursorPage {
	page := &CursorPage{Items: []json.RawMessage{}, Raw: r.Data, RequestID: r.RequestID}
	var rows []json.RawMessage
	if json.Unmarshal(r.Data, &rows) == nil && rows != nil {
		page.Items = rows
	}
	data := objectFields(r.Data)
	if json.Unmarshal(data["items"], &rows) == nil && rows != nil {
		page.Items = rows
	} else if firstArray && data != nil {
		if first := firstArrayField(r.Data); first != nil {
			page.Items = first
		}
	}
	info := pageInfoOf(r, data)
	page.NextCursor = stringField(data, "nextCursor")
	if page.NextCursor == "" {
		page.NextCursor = stringField(info, "nextCursor")
	}
	var hasMore bool
	if raw, ok := info["hasMore"]; ok && json.Unmarshal(raw, &hasMore) == nil {
		page.HasMore = hasMore && page.NextCursor != ""
	} else {
		page.HasMore = page.NextCursor != ""
	}
	page.TotalCount = posSafeInteger(data["totalCount"])
	if page.TotalCount == nil {
		page.TotalCount = posSafeInteger(info["totalCount"])
	}
	return page
}

func pageInfoOf(r *Response, data map[string]json.RawMessage) map[string]json.RawMessage {
	raw := r.PageInfo
	if len(raw) == 0 || string(raw) == "null" {
		raw = data["pageInfo"]
	}
	return objectFields(raw)
}

func objectFields(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	return fields
}

func stringField(fields map[string]json.RawMessage, name string) string {
	var text string
	if json.Unmarshal(fields[name], &text) != nil {
		return ""
	}
	return text
}

func boolField(fields map[string]json.RawMessage, name string) bool {
	var flag bool
	return json.Unmarshal(fields[name], &flag) == nil && flag
}

// firstArrayField is the first array member of a JSON object, in wire order.
func firstArrayField(raw json.RawMessage) []json.RawMessage {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil
	}
	for decoder.More() {
		if _, err := decoder.Token(); err != nil {
			return nil
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil
		}
		var rows []json.RawMessage
		if json.Unmarshal(value, &rows) == nil && rows != nil {
			return rows
		}
	}
	return nil
}

// recordField returns one member of a data object, nil when it is absent or
// null.
func recordField(data json.RawMessage, name string) json.RawMessage {
	value, ok := objectFields(data)[name]
	if !ok || string(value) == "null" {
		return nil
	}
	return value
}

// ── Paths, bodies and queries ────────────────────────────────────────────

// fillRoutePath fills the ":name" segments of a route template. Every value
// is trimmed and must follow the resource id rule of this SDK (letters,
// digits and _ . : - only), so no segment needs escaping.
func fillRoutePath(template string, values map[string]string) (string, error) {
	segments := strings.Split(template, "/")
	for i, segment := range segments {
		if !strings.HasPrefix(segment, ":") {
			continue
		}
		name := segment[1:]
		value := strings.TrimSpace(values[name])
		if value == "" {
			return "", invalid("'" + name + "' is required")
		}
		if _, err := idPath(value); err != nil {
			return "", invalid("'" + name + "' is not a valid identifier")
		}
		segments[i] = value
	}
	return strings.Join(segments, "/"), nil
}

// plainObject turns a request map (or any JSON-encodable object) into its
// JSON form (map[string]any, []any, string, json.Number, bool, nil), so the
// checks see what is sent. nil is an empty object.
func plainObject(value any, label string) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, invalid(label + " must be JSON-encodable")
	}
	if string(raw) == "null" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if decodeNumbers(raw, &out) != nil || out == nil {
		return nil, invalid(label + " must be an object")
	}
	return out, nil
}

// compactNil drops nil members.
func compactNil(object map[string]any) map[string]any {
	out := make(map[string]any, len(object))
	for key, value := range object {
		if value != nil {
			out[key] = value
		}
	}
	return out
}

// checkFieldNames refuses members outside allowed and reports the required
// members that present says are missing.
func checkFieldNames(label string, object map[string]any, required, optional []string, present func(any) bool) error {
	var unknown []string
	for key := range object {
		if !slices.Contains(required, key) && !slices.Contains(optional, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return invalid(label + " does not accept " + strings.Join(unknown, ", ") + " (the gateway rejects unknown fields)")
	}
	var missing []string
	for _, key := range required {
		if !present(object[key]) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return invalid(label + " requires " + strings.Join(missing, ", "))
	}
	return nil
}

// orderedFields emits the members of object in the order of fields.
func orderedFields(object map[string]any, fields []string) orderedObject {
	out := orderedObject{}
	for _, key := range fields {
		if value, ok := object[key]; ok {
			out = append(out, member{key: key, value: value})
		}
	}
	return out
}

// presentValue: not nil and not "".
func presentValue(value any) bool {
	if value == nil {
		return false
	}
	text, ok := value.(string)
	return !ok || text != ""
}

// presentNonEmpty: presentValue and, for a list, at least one element.
func presentNonEmpty(value any) bool {
	if !presentValue(value) {
		return false
	}
	if list, ok := value.([]any); ok {
		return len(list) > 0
	}
	return true
}

// emptyFilter: a filter that is left out (nil, "", an empty list).
func emptyFilter(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case *time.Time:
		return v == nil
	}
	return false
}

// routeQuery checks filters against a route's query parameters and encodes
// them: text, booleans, integers, a list of strings (sent comma-separated)
// and time.Time (UTC ISO-8601 with milliseconds). Empty filters are left
// out.
func routeQuery(label string, allowed []string, filters map[string]any) (url.Values, error) {
	var unknown []string
	for key, value := range filters {
		if !slices.Contains(allowed, key) && value != nil {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return nil, invalid(label + " does not accept the filter(s) " + strings.Join(unknown, ", "))
	}
	query := url.Values{}
	for _, key := range allowed {
		value := filters[key]
		if emptyFilter(value) {
			continue
		}
		text, err := queryText(key, value)
		if err != nil {
			return nil, err
		}
		query.Set(key, text)
	}
	return query, nil
}

func queryText(key string, value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case json.Number:
		return v.String(), nil
	case time.Time, *time.Time:
		return timeText(v, key)
	}
	if n, ok := integerOf(value); ok {
		return strconv.FormatInt(n, 10), nil
	}
	if list, ok := stringsOf(value); ok {
		return strings.Join(list, ","), nil
	}
	return "", invalid("'" + key + "' must be text, a boolean, an integer, a list of strings or a time.Time")
}

// integerOf reads a Go integer, an integral json.Number or an integral
// float64 within the safe integer range.
func integerOf(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint:
		if uint64(v) <= uint64(MaxSafeInteger) {
			return int64(v), true
		}
	case uint64:
		if v <= uint64(MaxSafeInteger) {
			return int64(v), true
		}
	case json.Number:
		if n, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return n, true
		}
	case float64:
		if v == float64(int64(v)) && v <= float64(MaxSafeInteger) && v >= -float64(MaxSafeInteger) {
			return int64(v), true
		}
	}
	return 0, false
}

// stringsOf reads one string or a list of strings.
func stringsOf(value any) ([]string, bool) {
	switch v := value.(type) {
	case string:
		return []string{v}, true
	case []string:
		return slices.Clone(v), true
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			text, ok := entry.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	}
	return nil, false
}

// timeText is a time.Time as UTC ISO-8601 with milliseconds, or non-empty
// ISO-8601 text as given (trimmed).
func timeText(value any, field string) (string, error) {
	if pointer, ok := value.(*time.Time); ok {
		if pointer == nil {
			return "", invalid("'" + field + "' is not a valid time")
		}
		value = *pointer
	}
	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return "", invalid("'" + field + "' is not a valid time")
		}
		return v.UTC().Format("2006-01-02T15:04:05.000Z"), nil
	case string:
		if text := strings.TrimSpace(v); text != "" {
			return text, nil
		}
	}
	return "", invalid("'" + field + "' must be a time.Time or an ISO-8601 string with a time zone")
}

// instantOf reads a time.Time or RFC 3339 text; false when it cannot be read
// (the gateway then decides).
func instantOf(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, !v.IsZero()
	case *time.Time:
		if v != nil {
			return *v, !v.IsZero()
		}
	case string:
		if at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(v)); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

// ── Keys ─────────────────────────────────────────────────────────────────

// randomUUID returns a random UUID v4.
func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("lunixi: random source unavailable")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// validPrintableKey: 8–200 printable ASCII characters without spaces (bank
// transfers, insurance, payment links and physical POS).
func validPrintableKey(key string) bool { return len(key) >= 8 && safeHeader(key, 200) }

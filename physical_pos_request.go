package lunixi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"maps"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Request checks shared by PhysicalPosClient and PhysicalPosManagementClient:
// field lists, card data, filters, Idempotency-Key, paging, the payment-write
// attempt loop and error mapping.

// PhysicalPosPage is one page of a physical POS list. Items are the rows of
// data.items in wire order; the paging comes from the envelope's pageInfo (a
// copy inside data is read as a fallback). Send NextCursor back verbatim as
// the cursor filter; HasMore is true only with a non-empty cursor.
// TotalCount is set only when includeTotal was asked for on an entity list.
type PhysicalPosPage struct {
	Items      []json.RawMessage
	HasMore    bool
	NextCursor string
	TotalCount *int64
	RequestID  string
}

// PhysicalPosWriteResult is the answer of a physical POS write that carries
// an Idempotency-Key.
//
// Payment writes (CreatePayment, CreateReversal) answer 202 for a new
// payment — accepted, not paid: follow the outcome by webhook or GetPayment —
// and 200 with Replayed true when the same key and body were sent again (the
// payment's current view). CancelPayment answers 200. StartSettlement answers
// 202 new / 200 replay, management creates 201 new / 200 replay.
// IdempotencyKey is the key that was sent (yours, or the generated one);
// keep it while the outcome is not known.
type PhysicalPosWriteResult struct {
	// Data is the whole data object (payment, run, store, terminal, device
	// … and sibling fields such as link or closedAssignment).
	Data           json.RawMessage
	Replayed       bool
	StatusCode     int
	IdempotencyKey string
	RequestID      string
}

// Record returns one field of Data (payment, run, store, register,
// terminal, acquirer, provider, device …), nil when absent.
func (r *PhysicalPosWriteResult) Record(name string) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(r.Data, &fields) != nil {
		return nil
	}
	value, ok := fields[name]
	if !ok || string(value) == "null" {
		return nil
	}
	return value
}

// Payment returns data.payment of a payment write.
func (r *PhysicalPosWriteResult) Payment() json.RawMessage { return r.Record("payment") }

// Accepted reports a 202 answer: a new payment, reversal or end-of-day run
// was queued to the device.
func (r *PhysicalPosWriteResult) Accepted() bool { return r.StatusCode == 202 }

// DecodeData decodes Data using json.Number for interface values.
func (r *PhysicalPosWriteResult) DecodeData(target any) error {
	return decodeNumbers(r.Data, target)
}

func decodeNumbers(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

// NewPhysicalPosIdempotencyKey returns a new Idempotency-Key (a random UUID
// v4). Store it with your order before you call, and use the same key to
// retry that operation.
func NewPhysicalPosIdempotencyKey() string { return randomUUID() }

func posCheckIdempotencyKey(key string) (string, error) {
	if !validPhysicalPosIdempotency(key) {
		return "", invalid("the physical POS Idempotency-Key must be 8–200 printable ASCII characters without spaces; it is sent exactly as given. Use a new UUID for every operation and the same key to retry it")
	}
	return key, nil
}

// posIdempotencyKeyOf is the key to send where the SDK may create one: the
// caller's key (checked, never trimmed) or a new UUID for "".
func posIdempotencyKeyOf(key string) (string, error) {
	if key == "" {
		return NewPhysicalPosIdempotencyKey(), nil
	}
	return posCheckIdempotencyKey(key)
}

// posRequireIdempotencyKey: payment start and reversal take the caller's
// key; nothing is generated.
func posRequireIdempotencyKey(key, operation string) (string, error) {
	if key == "" {
		return "", invalid("a stable Idempotency-Key is required for " + operation + ". Create one with NewPhysicalPosIdempotencyKey, store it with your order, then call; when the outcome is unknown call again with the SAME key")
	}
	return posCheckIdempotencyKey(key)
}

// posPathID checks one path segment (the resource id rule of this SDK:
// letters, digits and _ . : - only, so no segment needs escaping).
func posPathID(value, field string) (string, error) {
	if value == "" {
		return "", invalid("'" + field + "' must be a non-empty string")
	}
	if _, err := idPath(value); err != nil {
		return "", invalid("'" + field + "' is not a valid identifier")
	}
	return value, nil
}

// ── Bodies ───────────────────────────────────────────────────────────────

// posNormalize turns a request map into its JSON form (map[string]any,
// []any, string, json.Number, bool, nil), so every check sees what is sent.
func posNormalize(value any, label string) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, invalid(label + " must be JSON-encodable")
	}
	var out any
	if err = decodeNumbers(raw, &out); err != nil {
		return nil, invalid(label + " must be JSON-encodable")
	}
	return out, nil
}

func posCardDataRejected(field string) error {
	return &PhysicalPosCardDataRejectedError{Field: field, cause: ErrConfiguration}
}

func posRejectCardData(value any, root string) error {
	if field, found := posFindCardDataField(value, root); found {
		return posCardDataRejected(field)
	}
	return nil
}

func posAssertKeys(object map[string]any, allowed []string, label, what string) error {
	var unknown []string
	for key := range object {
		if !slices.Contains(allowed, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return invalid("unknown " + label + " " + what + "(s): " + strings.Join(unknown, ", ") + ". Allowed: " + strings.Join(allowed, ", ") + ". The gateway rejects unknown " + what + "s with 400")
}

func posRequireString(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok || text == "" {
		return "", invalid("'" + field + "' must be a non-empty string")
	}
	return text, nil
}

// posInteger reads a JSON integer (a number, not a string) within the safe
// integer range.
func posInteger(value any, field string) (int64, error) {
	fail := invalid("'" + field + "' must be an integer (a JSON number, not a string)")
	var n int64
	switch v := value.(type) {
	case json.Number:
		if parsed, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			n = parsed
		} else if f, err := strconv.ParseFloat(string(v), 64); err == nil && f == math.Trunc(f) && math.Abs(f) <= float64(MaxSafeInteger) {
			n = int64(f)
		} else {
			return 0, fail
		}
	case int:
		n = int64(v)
	case int8:
		n = int64(v)
	case int16:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case uint:
		if uint64(v) > uint64(MaxSafeInteger) {
			return 0, fail
		}
		n = int64(v)
	case uint8:
		n = int64(v)
	case uint16:
		n = int64(v)
	case uint32:
		n = int64(v)
	case uint64:
		if v > uint64(MaxSafeInteger) {
			return 0, fail
		}
		n = int64(v)
	case float32:
		return posInteger(json.Number(strconv.FormatFloat(float64(v), 'f', -1, 32)), field)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fail
		}
		return posInteger(json.Number(strconv.FormatFloat(v, 'f', -1, 64)), field)
	default:
		return 0, fail
	}
	if n > MaxSafeInteger || n < -MaxSafeInteger {
		return 0, fail
	}
	return n, nil
}

func posRowVersion(value any, field string) (int64, error) {
	n, err := posInteger(value, field)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > PhysicalPosRowVersionMax {
		return 0, invalid("'" + field + "' must be between 1 and " + strconv.FormatInt(PhysicalPosRowVersionMax, 10) + ": send the rowVersion you last read")
	}
	return n, nil
}

// posMinor checks canonical minor-unit text (positive, or zero allowed).
func posMinor(value any, field string, positive bool) (string, error) {
	text, ok := value.(string)
	if !ok || !validMinor(text, positive, false) {
		return "", invalid(field + " must be a canonical string in minor units")
	}
	return text, nil
}

func posObjectOf(value any, allowed []string, field string, check func(key string, entry any, path string) (any, bool, error)) (orderedObject, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid("'" + field + "' must be an object")
	}
	if err := posAssertKeys(object, allowed, field, "field"); err != nil {
		return nil, err
	}
	out := orderedObject{}
	for _, key := range allowed {
		entry, present := object[key]
		if !present {
			continue
		}
		checked, include, err := check(key, entry, field+"."+key)
		if err != nil {
			return nil, err
		}
		if include {
			out = append(out, member{key: key, value: checked})
		}
	}
	return out, nil
}

func posItemsOf(value any, kinds []physicalPosField, field string) ([]orderedObject, error) {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil, invalid("'" + field + "' must be a non-empty array; leave the field out for a request without items")
	}
	allowed := make([]string, len(kinds))
	kindOf := map[string]string{}
	for i, kind := range kinds {
		allowed[i] = kind.name
		kindOf[kind.name] = kind.kind
	}
	out := make([]orderedObject, 0, len(list))
	for index, item := range list {
		path := field + "[" + strconv.Itoa(index) + "]"
		object, err := posObjectOf(item, allowed, path, func(key string, entry any, entryPath string) (any, bool, error) {
			return posCheckValue(kindOf[key], entry, entryPath)
		})
		if err != nil {
			return nil, err
		}
		for _, kind := range kinds {
			if strings.HasSuffix(kind.kind, "*") && !object.has(kind.name) {
				return nil, invalid("'" + path + "." + kind.name + "' is required")
			}
		}
		out = append(out, object)
	}
	return out, nil
}

func (o orderedObject) has(key string) bool {
	_, ok := o.get(key)
	return ok
}

func (o orderedObject) get(key string) (any, bool) {
	for _, m := range o {
		if m.key == key {
			return m.value, true
		}
	}
	return nil, false
}

// posCheckValue checks one value against its kind. Money is canonical text
// only; a present null or blank amount is refused. One rule in every SDK:
// null is sent only for a nullable ("?") field — an explicit clear — and left
// out for any other optional field; "" is left out for an optional field and
// refused for a required one. The second result reports whether the field is
// sent.
func posCheckValue(kindSpec string, value any, field string) (any, bool, error) {
	required := strings.HasSuffix(kindSpec, "*")
	nullable := strings.HasSuffix(kindSpec, "?")
	kind := strings.TrimRight(kindSpec, "*?")
	if kind == "money" {
		text, err := posMinor(value, field, true)
		return text, err == nil, err
	}
	if value == nil {
		if nullable {
			return nil, true, nil
		}
		if required {
			return nil, false, invalid("'" + field + "' is required")
		}
		return nil, false, nil
	}
	if text, ok := value.(string); ok && text == "" && !required {
		return nil, false, nil
	}
	var out any
	var err error
	switch kind {
	case "string":
		out, err = posRequireString(value, field)
	case "int":
		out, err = posInteger(value, field)
	case "rowVersion":
		out, err = posRowVersion(value, field)
	case "bool":
		flag, ok := value.(bool)
		if !ok {
			return nil, false, invalid("'" + field + "' must be a boolean")
		}
		out = flag
	case "strings":
		list, ok := value.([]any)
		texts := make([]string, 0, len(list))
		for _, entry := range list {
			text, isText := entry.(string)
			if !isText || text == "" {
				ok = false
				break
			}
			texts = append(texts, text)
		}
		if !ok {
			return nil, false, invalid("'" + field + "' must be an array of non-empty strings")
		}
		out = texts
	case "target":
		out, err = posObjectOf(value, physicalPosTargetFields, field, func(_ string, entry any, path string) (any, bool, error) {
			return posCheckValue("string", entry, path)
		})
	case "saleItems":
		out, err = posItemsOf(value, physicalPosSaleItemKinds, field)
	case "refundItems":
		out, err = posItemsOf(value, physicalPosRefundItemKinds, field)
	default:
		return nil, false, invalid("unsupported field kind '" + kindSpec + "'")
	}
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// posBuildBody builds a request body from the field list of operation:
// card-data keys first (PhysicalPosCardDataRejectedError), then unknown
// fields, required fields and value types (ErrConfiguration), then
// card-number values in the scanned free-text fields. Nothing is sent when a
// check fails.
func posBuildBody(request map[string]any, operation string) (orderedObject, error) {
	label := operation + " request"
	if request == nil {
		return nil, invalid(label + " must be an object")
	}
	normalized, err := posNormalize(request, label)
	if err != nil {
		return nil, err
	}
	if err = posRejectCardData(normalized, "(body)"); err != nil {
		return nil, err
	}
	body, ok := normalized.(map[string]any)
	if !ok {
		return nil, invalid(label + " must be an object")
	}
	fields := physicalPosRequestFields[operation]
	allowed := make([]string, len(fields))
	for i, field := range fields {
		allowed[i] = field.name
	}
	if err = posAssertKeys(body, allowed, label, "field"); err != nil {
		return nil, err
	}
	out := orderedObject{}
	for _, field := range fields {
		value, present := body[field.name]
		if !present {
			if strings.HasSuffix(field.kind, "*") {
				return nil, invalid("'" + field.name + "' is required")
			}
			continue
		}
		checked, include, err := posCheckValue(field.kind, value, field.name)
		if err != nil {
			return nil, err
		}
		if include {
			out = append(out, member{key: field.name, value: checked})
		}
	}
	if err = posRejectCardNumbers(out); err != nil {
		return nil, err
	}
	return out, nil
}

// posRejectCardNumbers scans the free-text fields the gateway scans for a
// card number; the value never reaches the error.
func posRejectCardNumbers(body orderedObject) error {
	if reference, ok := body.get("merchantReference"); ok {
		if text, isText := reference.(string); isText && posContainsPanLikeValue(text, posPanStrict) {
			return posCardDataRejected("merchantReference")
		}
	}
	raw, _ := body.get("items")
	items, _ := raw.([]orderedObject)
	for index, item := range items {
		prefix := "items[" + strconv.Itoa(index) + "]."
		if name, ok := item.get("name"); ok {
			if text, isText := name.(string); isText && posContainsPanLikeValue(text, posPanItemText) {
				return posCardDataRejected(prefix + "name")
			}
		}
		if code, ok := item.get("taxGroupCode"); ok {
			if text, isText := code.(string); isText && posContainsPanLikeValue(text, posPanStrict) {
				return posCardDataRejected(prefix + "taxGroupCode")
			}
		}
	}
	return nil
}

// ── Filters ──────────────────────────────────────────────────────────────

var posFreeTextFilters = []string{"merchantReference", "q"}

// posBuildQuery checks the filters of a route. Values: text, an integer
// limit (1–100), a boolean includeTotal, minor-unit text for
// amountFromMinor / amountToMinor, and a time.Time for dates (dateFrom /
// dateTo are sent as the time's own calendar day YYYY-MM-DD, other dates as
// UTC ISO-8601 with milliseconds). nil and "" are left out.
func posBuildQuery(filters map[string]any, route string) (url.Values, error) {
	query := url.Values{}
	if len(filters) == 0 {
		return query, nil
	}
	normalized, err := posNormalize(filters, "physical POS filters")
	if err != nil {
		return nil, err
	}
	if err = posRejectCardData(normalized, "(query)"); err != nil {
		return nil, err
	}
	allowed := physicalPosFilters[route]
	if err = posAssertKeys(filters, allowed, "physical POS", "filter"); err != nil {
		return nil, err
	}
	for _, key := range allowed {
		value, present := filters[key]
		if !present {
			continue
		}
		if key == "amountFromMinor" || key == "amountToMinor" {
			text, err := posMinor(value, key, true)
			if err != nil {
				return nil, err
			}
			query.Set(key, text)
			continue
		}
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && text == "" {
			continue
		}
		text, err := posFilterValue(key, value)
		if err != nil {
			return nil, err
		}
		if slices.Contains(posFreeTextFilters, key) && posContainsPanLikeValue(text, posPanStrict) {
			return nil, posCardDataRejected(key)
		}
		query.Set(key, text)
	}
	return query, nil
}

func posFilterValue(key string, value any) (string, error) {
	switch key {
	case "limit":
		n, err := posInteger(value, key)
		if err != nil || n < 1 || n > PhysicalPosMaxLimit {
			return "", invalid("'limit' must be an integer between 1 and " + strconv.Itoa(PhysicalPosMaxLimit))
		}
		return strconv.FormatInt(n, 10), nil
	case "includeTotal":
		flag, ok := value.(bool)
		if !ok {
			return "", invalid("'includeTotal' must be a boolean")
		}
		return strconv.FormatBool(flag), nil
	}
	if pointer, ok := value.(*time.Time); ok {
		if pointer == nil {
			return "", invalid("'" + key + "' is not a valid date")
		}
		value = *pointer
	}
	if at, ok := value.(time.Time); ok {
		if at.IsZero() {
			return "", invalid("'" + key + "' is not a valid date")
		}
		if key == "dateFrom" || key == "dateTo" {
			return at.Format("2006-01-02"), nil
		}
		return at.UTC().Format("2006-01-02T15:04:05.000Z"), nil
	}
	text, ok := value.(string)
	if !ok {
		return "", invalid("'" + key + "' must be a string")
	}
	return text, nil
}

// ── Responses ────────────────────────────────────────────────────────────

// posRecord is data.<name> when present and not null, otherwise data.
func posRecord(r *Response, name string) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(r.Data, &fields) == nil {
		if value, ok := fields[name]; ok && string(value) != "null" {
			return value
		}
	}
	return r.Data
}

func posSafeInteger(raw json.RawMessage) *int64 {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '"' {
		return nil
	}
	var number json.Number
	if decodeNumbers(raw, &number) != nil {
		return nil
	}
	n, err := posInteger(number, "")
	if err != nil {
		return nil
	}
	return &n
}

func posPage(r *Response) *PhysicalPosPage {
	page := &PhysicalPosPage{Items: []json.RawMessage{}, RequestID: r.RequestID}
	var data map[string]json.RawMessage
	_ = json.Unmarshal(r.Data, &data)
	var items []json.RawMessage
	if json.Unmarshal(data["items"], &items) == nil && items != nil {
		page.Items = items
	}
	pageInfo := r.PageInfo
	if len(pageInfo) == 0 || string(pageInfo) == "null" {
		pageInfo = data["pageInfo"]
	}
	var info map[string]json.RawMessage
	if json.Unmarshal(pageInfo, &info) == nil {
		var cursor string
		if json.Unmarshal(info["nextCursor"], &cursor) == nil {
			page.NextCursor = cursor
		}
		var hasMore bool
		page.HasMore = json.Unmarshal(info["hasMore"], &hasMore) == nil && hasMore && page.NextCursor != ""
		if raw, ok := info["totalCount"]; ok {
			page.TotalCount = posSafeInteger(raw)
		}
	}
	return page
}

func posWriteResult(r *Response, key string) *PhysicalPosWriteResult {
	data := r.Data
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		data = json.RawMessage(`{}`)
	}
	var replayed bool
	_ = json.Unmarshal(fields["replayed"], &replayed)
	return &PhysicalPosWriteResult{Data: data, Replayed: replayed, StatusCode: r.StatusCode, IdempotencyKey: key, RequestID: r.RequestID}
}

// posIterate follows the cursor of a list lazily. A failed page is yielded
// once as the error and ends the sequence.
func posIterate(ctx context.Context, filters map[string]any, list func(context.Context, map[string]any) (*PhysicalPosPage, error)) iter.Seq2[json.RawMessage, error] {
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
			current["cursor"] = page.NextCursor
		}
	}
}

// ── Response money ───────────────────────────────────────────────────────

var (
	posMoneyFields = []string{
		"amountFromMinor", "amountMinor", "amountToMinor", "approvedAmountMinor", "baseMinor", "capturedMinor",
		"lineTotalMinor", "netBaseMinor", "netMinor", "netTaxMinor", "refundBaseMinor", "refundMinor", "refundTaxMinor",
		"refundedAmountMinor", "refundedMinor", "reservedAmountMinor", "reservedMinor", "reversalReservedMinor",
		"reversedMinor", "saleBaseMinor", "saleMinor", "saleTaxMinor", "unitAmountMinor", "unitPriceMinor",
		"voidBaseMinor", "voidMinor", "voidTaxMinor",
	}
	posMoneyContainers = []string{
		"data", "payment", "payments", "items", "refundItems", "taxes", "rows", "transaction", "transactions", "root",
		"children", "itemSummary", "amountTotals", "reported", "system", "difference", "totals", "lines",
	}
	posMoneyNullable = []string{"approvedAmountMinor", "unitAmountMinor"}
)

// posValidateMoney checks the declared physical POS money fields of a
// response or webhook payload: canonical minor-unit text (signed under
// difference and for net* fields). Counts and opaque metadata stay
// untouched.
func posValidateMoney(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if decodeNumbers(raw, &value) != nil {
		return nil
	}
	return posValidateMoneyValue(value, false)
}

func posValidateMoneyValue(value any, difference bool) error {
	switch v := value.(type) {
	case []any:
		for _, entry := range v {
			if err := posValidateMoneyValue(entry, difference); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, entry := range v {
			if slices.Contains(posMoneyFields, key) {
				if entry == nil && slices.Contains(posMoneyNullable, key) {
					continue
				}
				text, ok := entry.(string)
				if !ok || !validMinor(text, false, difference || strings.HasPrefix(key, "net")) {
					return invalid(key + " must be a canonical string in minor units")
				}
			} else if slices.Contains(posMoneyContainers, key) {
				if err := posValidateMoneyValue(entry, difference || key == "difference"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ── Sending and errors ───────────────────────────────────────────────────

// posSend sends one signed physical POS request (reads and management
// writes), checks the response money and maps the errors. key is the
// Idempotency-Key of a keyed write ("" for none).
func (c *Client) posSend(ctx context.Context, method, path string, body any, query url.Values, key string) (*Response, error) {
	r, err := c.Do(ctx, Request{Method: method, Path: physicalPosBase + path, Query: query, Body: body, IdempotencyKey: key})
	if err != nil {
		return nil, posMapError(err, r, key)
	}
	if err = posValidateMoney(r.Envelope); err != nil {
		return nil, err
	}
	return r, nil
}

type posErrorDetails struct {
	field, allowedOperation string
	retryAfter              time.Duration
}

func posDetailsOf(r *Response) posErrorDetails {
	var out posErrorDetails
	if r == nil {
		return out
	}
	var envelope struct {
		Details map[string]any `json:"details"`
	}
	if decodeNumbers(r.Envelope, &envelope) != nil {
		return out
	}
	out.field, _ = envelope.Details["field"].(string)
	out.allowedOperation, _ = envelope.Details["allowedOperation"].(string)
	out.retryAfter = posSeconds(envelope.Details["retryAfterSeconds"])
	return out
}

// posSeconds reads a number of seconds (a JSON number or numeric text).
func posSeconds(value any) time.Duration {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = string(v)
	case string:
		text = strings.TrimSpace(v)
	default:
		return 0
	}
	seconds, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 86400 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

// posRetryAfter is the Retry-After header in seconds, else
// details.retryAfterSeconds.
func posRetryAfter(r *Response) time.Duration {
	if r == nil {
		return 0
	}
	if delay := posSeconds(r.retryAfter); delay > 0 {
		return delay
	}
	return posDetailsOf(r).retryAfter
}

// posMapError maps the error of one answered request to the typed physical
// POS errors: card data (field path), void after end-of-day
// (allowedOperation), 429 (Retry-After). Other errors of a keyed write are
// wrapped in PhysicalPosWriteError with the key; errors raised before
// anything was sent are returned as they are.
func posMapError(err error, r *Response, key string) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		if key != "" && !errors.Is(err, ErrConfiguration) {
			return &PhysicalPosWriteError{IdempotencyKey: key, Err: err}
		}
		return err
	}
	switch {
	case apiErr.Code == PhysicalPosCardDataRejectedCode:
		return &PhysicalPosCardDataRejectedError{Field: posDetailsOf(r).field, StatusCode: apiErr.StatusCode, Code: apiErr.Code, RequestID: apiErr.RequestID, IdempotencyKey: key, cause: apiErr}
	case apiErr.Code == PhysicalPosVoidAfterSettlementCode:
		return &PhysicalPosVoidAfterSettlementError{AllowedOperation: posDetailsOf(r).allowedOperation, StatusCode: apiErr.StatusCode, Code: apiErr.Code, RequestID: apiErr.RequestID, IdempotencyKey: key, cause: apiErr}
	case apiErr.StatusCode == 429:
		return &PhysicalPosRateLimitError{RetryAfter: posRetryAfter(r), StatusCode: apiErr.StatusCode, Code: apiErr.Code, RequestID: apiErr.RequestID, IdempotencyKey: key, cause: apiErr}
	}
	if key != "" {
		return &PhysicalPosWriteError{IdempotencyKey: key, Err: err}
	}
	return err
}

// ── Payment writes: start, reversal, cancel ──────────────────────────────

type posAttempt int

const (
	posAmbiguous posAttempt = iota
	posNotStarted
	posDefinite
)

// posAttemptOutcome classifies one failed attempt of a payment write:
//   - ambiguous: no HTTP answer (connect, reset, timeout, an ended context —
//     also while the body is read) or a 5xx that does not say "not started".
//     The payment may exist.
//   - not started: a 5xx with a code in PhysicalPosNotStartedCodes.
//   - definite: any 4xx (429 and 409 included).
func posAttemptOutcome(err error) posAttempt {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return posAmbiguous
	}
	switch apiErr.Code {
	case "TRANSPORT_ERROR", "RESPONSE_READ_FAILED", "RESPONSE_TOO_LARGE":
		return posAmbiguous
	}
	if apiErr.StatusCode >= 500 {
		if slices.Contains(physicalPosNotStartedCodes, apiErr.Code) {
			return posNotStarted
		}
		return posAmbiguous
	}
	if apiErr.StatusCode >= 400 {
		return posDefinite
	}
	return posAmbiguous
}

// posLocalError reports an error raised before anything was sent.
func posLocalError(err error) bool {
	var apiErr *APIError
	return errors.Is(err, ErrConfiguration) || (errors.As(err, &apiErr) && apiErr.Code == "RANDOM_SOURCE_UNAVAILABLE")
}

func posHasPayment(r *Response) bool {
	if posValidateMoney(r.Envelope) != nil {
		return false
	}
	var data struct {
		Payment *struct {
			PaymentID string `json:"paymentId"`
		} `json:"payment"`
	}
	return json.Unmarshal(r.Data, &data) == nil && data.Payment != nil && data.Payment.PaymentID != ""
}

func posOutcomeUnknown(last error, response *Response, key string, attempts int) error {
	out := &PhysicalPosOutcomeUnknownError{IdempotencyKey: key, Attempts: attempts, cause: last}
	var apiErr *APIError
	if errors.As(last, &apiErr) {
		out.StatusCode, out.Code, out.RequestID = apiErr.StatusCode, apiErr.Code, apiErr.RequestID
		out.RetryAfter = posRetryAfter(response)
	}
	return out
}

func posBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(min(2000, 100<<(attempt-1))) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// posPaymentWrite sends a payment write with its own attempts (the
// transport's retries are off for it): 1 + MaxRetries attempts, the same key
// and body every time.
//
// Success is only a 2xx carrying a payment. A 2xx without a readable payment,
// no HTTP answer, or a 5xx that does not say "not started" leaves the outcome
// unknown; it is retried while attempts remain. A 4xx stops at once. When any
// attempt was ambiguous the call fails with PhysicalPosOutcomeUnknownError
// whatever the last answer was (a later 429 or 409 included), because the key
// may already hold a payment.
func (c *Client) posPaymentWrite(ctx context.Context, path string, body any, key string) (*PhysicalPosWriteResult, error) {
	attempts := 1 + c.maxRetries
	ambiguous := false
	var last error
	var lastResponse *Response
	made := 0
	for attempt := 1; attempt <= attempts; attempt++ {
		made = attempt
		r, err := c.send(ctx, Request{Method: "POST", Path: physicalPosBase + path, Body: body, IdempotencyKey: key}, sendOptions{noRetry: true})
		if err != nil {
			if posLocalError(err) {
				if ambiguous {
					return nil, posOutcomeUnknown(err, nil, key, made)
				}
				return nil, err
			}
			last, lastResponse = err, r
			outcome := posAttemptOutcome(err)
			if outcome == posAmbiguous {
				ambiguous = true
			}
			if outcome == posDefinite || ctx.Err() != nil {
				break
			}
			if attempt < attempts && posBackoff(ctx, attempt) != nil {
				break
			}
			continue
		}
		if posHasPayment(r) {
			return posWriteResult(r, key), nil
		}
		ambiguous = true
		last, lastResponse = &APIError{StatusCode: r.StatusCode, Code: "PAYMENT_MISSING", RequestID: r.RequestID}, r
		if attempt < attempts && posBackoff(ctx, attempt) != nil {
			break
		}
	}
	if ambiguous {
		return nil, posOutcomeUnknown(last, lastResponse, key, made)
	}
	return nil, posMapError(last, lastResponse, key)
}

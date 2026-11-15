package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// BankConnectClient — read your connected bank accounts, balances and
// transactions (/api/v1/bank-connect, product BANK_CONNECT).
//
// Every call is signed, reads included. Scopes: bank-connect:view for banks,
// connections, accounts and RequestSync; bank-connect:transactions:view for
// balances and transactions (403 API_INSUFFICIENT_SCOPE otherwise). The key
// decides the environment: a TEST key reads only test-environment accounts,
// a LIVE key only live ones. Connecting a bank and entering bank credentials
// happen in the Lunixi dashboard only. Another organisation's or the other
// environment's record is 404, the same as a record that does not exist.
//
// Money movement lives on two sub-clients, available when the connection
// offers it (capabilities OUTGOING_TRANSFER / FX_EXCHANGE): Transfers and
// FX. They need the bank-connect:transfer scope and a key with an IP
// allowlist.
//
// Filters are maps checked against BankConnectFilters before anything is
// sent: an unknown filter, a value outside its set, a limit outside 1–100,
// more than 100 account ids, an amount that is not minor-unit text, a
// minimum above the maximum, bookedTo without bookedFrom or a window over 92
// days returns ErrConfiguration. Single records are returned as the
// envelope's data; lists as CursorPage.
type BankConnectClient struct {
	client *Client
	// Transfers sends outgoing transfers from a connected account.
	Transfers *BankTransferClient
	// FX exchanges currency between two of your accounts on one connection.
	FX *BankFXClient
}

func newBankConnectClient(c *Client) *BankConnectClient {
	return &BankConnectClient{client: c, Transfers: &BankTransferClient{client: c}, FX: &BankFXClient{client: c}}
}

// ListBanks lists the bank catalogue: whether each bank can be connected and
// what is read from it. Filters: limit, cursor.
func (b *BankConnectClient) ListBanks(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return b.list(ctx, "/banks", filters, "listBanks")
}

// IterateBanks yields every bank of the catalogue, page by page.
func (b *BankConnectClient) IterateBanks(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, b.ListBanks)
}

// ListConnections lists your bank connections, newest first. Filters:
// limit, cursor, status (one BankConnectionStatus* value or a list), bankId.
func (b *BankConnectClient) ListConnections(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return b.list(ctx, "/connections", filters, "listConnections")
}

// IterateConnections yields every connection matching the filters.
func (b *BankConnectClient) IterateConnections(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, b.ListConnections)
}

// GetConnection reads one connection; 404 bank_connect.connection.not_found
// for an unknown one.
func (b *BankConnectClient) GetConnection(ctx context.Context, connectionID string) (json.RawMessage, error) {
	return b.record(ctx, "/connections/:connectionId", "connectionId", connectionID)
}

// ListAccounts lists your connected accounts, newest first, each with its
// data freshness. Filters: limit, cursor, connectionId, bankId, currency,
// status (BankAccountStatus*).
func (b *BankConnectClient) ListAccounts(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return b.list(ctx, "/accounts", filters, "listAccounts")
}

// IterateAccounts yields every account matching the filters.
func (b *BankConnectClient) IterateAccounts(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, b.ListAccounts)
}

// GetAccount reads one account; 404 bank_connect.account.not_found for an
// unknown one.
func (b *BankConnectClient) GetAccount(ctx context.Context, accountID string) (json.RawMessage, error) {
	return b.record(ctx, "/accounts/:accountId", "accountId", accountID)
}

// RequestSync asks for a fresh read of the account from the bank (HTTP 202).
//
// It never reads earlier than the bank's minimum interval: when the interval
// has not passed the answer is SCHEDULED with scheduledAt, and a request
// while one is already waiting joins it. Nothing is written, so no
// Idempotency-Key is needed. New transactions arrive by the
// bank_account.transaction.received webhook. 409
// bank_connect.connection.not_active when the connection is not ACTIVE or
// DEGRADED; 429 RATE_LIMITED when the key sent too many sync requests in the
// last minute.
func (b *BankConnectClient) RequestSync(ctx context.Context, accountID string) (json.RawMessage, error) {
	path, err := fillRoutePath(bankConnectBase+"/accounts/:accountId/sync", map[string]string{"accountId": accountID})
	if err != nil {
		return nil, err
	}
	r, err := b.client.Do(ctx, Request{Method: "POST", Path: path})
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// ListBalances lists the latest balance of each account (scope
// bank-connect:transactions:view); without accountId every active account of
// the key's environment. Filters: limit, cursor, accountId (one id or a list
// of at most 100), currency.
func (b *BankConnectClient) ListBalances(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return b.list(ctx, "/balances", filters, "listBalances")
}

// IterateBalances yields every balance matching the filters.
func (b *BankConnectClient) IterateBalances(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, b.ListBalances)
}

// ListTransactions lists transactions, newest recorded first (scope
// bank-connect:transactions:view).
//
// The order is receivedAt (when Lunixi recorded the transaction), which
// never changes, so paging forward never skips a transaction that arrives
// late with an older bank date. bookedFrom (inclusive) / bookedTo
// (exclusive) filter on the bank date: a time.Time or ISO-8601 text with a
// time zone, at most 92 days apart; bookedTo needs bookedFrom (bookedFrom
// alone runs up to now). minAmountMinor / maxAmountMinor are inclusive
// minor-unit text. Also limit, cursor, accountId (one id or a list),
// direction, canonicalType (one BankTransactionType* value or a list).
func (b *BankConnectClient) ListTransactions(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return b.list(ctx, "/transactions", filters, "listTransactions")
}

// IterateTransactions yields every transaction matching the filters.
func (b *BankConnectClient) IterateTransactions(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, b.ListTransactions)
}

// GetTransaction reads one transaction with its detail fields
// (balanceAfterMinor, documentNo, bank).
func (b *BankConnectClient) GetTransaction(ctx context.Context, transactionID string) (json.RawMessage, error) {
	return b.record(ctx, "/transactions/:transactionId", "transactionId", transactionID)
}

func (b *BankConnectClient) list(ctx context.Context, path string, filters map[string]any, route string) (*CursorPage, error) {
	query, err := bankConnectQuery(filters, route)
	if err != nil {
		return nil, err
	}
	r, err := b.client.Do(ctx, Request{Method: "GET", Path: bankConnectBase + path, Query: query})
	if err != nil {
		return nil, err
	}
	return envelopePage(r), nil
}

func (b *BankConnectClient) record(ctx context.Context, template, field, id string) (json.RawMessage, error) {
	path, err := fillRoutePath(bankConnectBase+template, map[string]string{field: id})
	if err != nil {
		return nil, err
	}
	r, err := b.client.Do(ctx, Request{Method: "GET", Path: path})
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// ── Filters ──────────────────────────────────────────────────────────────

func bankConnectQuery(filters map[string]any, route string) (url.Values, error) {
	allowed := bankConnectFilters[route]
	if err := unknownFilters(filters, allowed, "bank-connect"); err != nil {
		return nil, err
	}
	query := url.Values{}
	for _, key := range allowed {
		value := filters[key]
		if emptyFilter(value) {
			continue
		}
		text, err := bankConnectFilterValue(key, value, route)
		if err != nil {
			return nil, err
		}
		query.Set(key, text)
	}
	if route == "listTransactions" {
		if err := bankConnectRanges(filters, query); err != nil {
			return nil, err
		}
	}
	return query, nil
}

// unknownFilters refuses filter names outside allowed.
func unknownFilters(filters map[string]any, allowed []string, product string) error {
	var unknown []string
	for key := range filters {
		if !slices.Contains(allowed, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return invalid("unknown " + product + " filter(s): " + strings.Join(unknown, ", ") + ". Allowed: " + strings.Join(allowed, ", ") + ". The gateway rejects unknown query fields with 400")
}

func bankConnectFilterValue(key string, value any, route string) (string, error) {
	switch key {
	case "limit":
		return pageLimit(value, BankConnectMaxLimit)
	case "cursor":
		return pageCursor(value, bankConnectCursorMax)
	case "status":
		if route == "listAccounts" {
			return oneValue(key, value, BankAccountStatuses())
		}
		return valueList(key, value, BankConnectionStatuses())
	case "direction":
		return oneValue(key, value, BankTransactionDirections())
	case "canonicalType":
		return valueList(key, value, BankTransactionTypes())
	case "accountId":
		return accountIDList(value)
	case "bankId", "connectionId":
		return requiredText(value, key)
	case "currency":
		return currencyCode(value, key)
	case "bookedFrom", "bookedTo":
		return timeText(value, key)
	case "minAmountMinor", "maxAmountMinor":
		text, ok := value.(string)
		if !ok || !validMinor(text, false, false) {
			return "", invalid("'" + key + "' must be a canonical string in minor units")
		}
		return text, nil
	}
	return "", invalid("unknown bank-connect filter '" + key + "'")
}

// bankConnectRanges applies the gateway's window rules when both ends are
// readable: bookedTo needs bookedFrom, bookedFrom <= bookedTo, at most 92
// days; minAmountMinor <= maxAmountMinor.
func bankConnectRanges(filters map[string]any, query url.Values) error {
	if query.Has("bookedTo") && !query.Has("bookedFrom") {
		return invalid("'bookedTo' needs 'bookedFrom': a window without a start is unbounded")
	}
	from, okFrom := instantOf(filters["bookedFrom"])
	to, okTo := instantOf(filters["bookedTo"])
	if okFrom && okTo {
		if from.After(to) {
			return invalid("'bookedFrom' must not be after 'bookedTo'")
		}
		if to.Sub(from) > BankConnectBookedRangeMaxDays*24*time.Hour {
			return invalid("'bookedFrom' and 'bookedTo' may be at most " + strconv.Itoa(BankConnectBookedRangeMaxDays) + " days apart. Page through narrower windows")
		}
	}
	if query.Has("minAmountMinor") && query.Has("maxAmountMinor") {
		low, _ := strconv.ParseInt(query.Get("minAmountMinor"), 10, 64)
		high, _ := strconv.ParseInt(query.Get("maxAmountMinor"), 10, 64)
		if low > high {
			return invalid("'minAmountMinor' must not be greater than 'maxAmountMinor'")
		}
	}
	return nil
}

func pageLimit(value any, max int64) (string, error) {
	n, ok := integerOf(value)
	if !ok || n < 1 || n > max {
		return "", invalid("'limit' must be an integer from 1 to " + strconv.FormatInt(max, 10))
	}
	return strconv.FormatInt(n, 10), nil
}

func pageCursor(value any, max int) (string, error) {
	text, ok := value.(string)
	if !ok || len(text) > max {
		return "", invalid("'cursor' must be the nextCursor of the previous page, passed back unchanged")
	}
	return text, nil
}

func oneValue(key string, value any, allowed []string) (string, error) {
	text, ok := value.(string)
	if !ok || !slices.Contains(allowed, text) {
		return "", invalid("'" + key + "' must be one of " + strings.Join(allowed, ", "))
	}
	return text, nil
}

// valueList reads one value or a list of values of a set, sent
// comma-separated.
func valueList(key string, value any, allowed []string) (string, error) {
	values, ok := stringsOf(value)
	if !ok {
		return "", invalid("'" + key + "' must be one of " + strings.Join(allowed, ", ") + " or a list of them")
	}
	for _, entry := range values {
		if !slices.Contains(allowed, entry) {
			return "", invalid("'" + key + "' must be one of " + strings.Join(allowed, ", "))
		}
	}
	return strings.Join(values, ","), nil
}

func accountIDList(value any) (string, error) {
	ids, ok := stringsOf(value)
	if !ok {
		return "", invalid("'accountId' must be account ids (strings without commas)")
	}
	if len(ids) > BankConnectMaxAccountFilter {
		return "", invalid("'accountId' takes at most " + strconv.Itoa(BankConnectMaxAccountFilter) + " account ids")
	}
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, ",") {
			return "", invalid("'accountId' must be account ids (strings without commas)")
		}
		ids[i] = id
	}
	return strings.Join(ids, ","), nil
}

func requiredText(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", invalid("'" + field + "' is required")
	}
	return strings.TrimSpace(text), nil
}

func currencyCode(value any, field string) (string, error) {
	text, _ := value.(string)
	currency := strings.ToUpper(strings.TrimSpace(text))
	if !currencyPattern.MatchString(currency) {
		return "", invalid("'" + field + "' must be an ISO 4217 code such as TRY")
	}
	return currency, nil
}

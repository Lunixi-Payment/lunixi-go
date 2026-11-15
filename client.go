package lunixi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var safeCode = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// Client is safe for concurrent use. Create one client per merchant key.
type Client struct {
	baseURL      string
	keyID        string
	key          ed25519.PrivateKey
	http         *http.Client
	now          func() time.Time
	maxRetries   int
	maxResponse  int64
	tokenMu      sync.Mutex
	token        string
	tokenExpiry  time.Time
	refresh      chan struct{}
	Payments     *PaymentClient
	PaymentLinks *PaymentLinkClient
	Fraud        *FraudClient
	Reference    *ReferenceClient
	// PhysicalPos starts and reverses card payments on your devices and
	// reads payments, transactions, reports and end-of-day results.
	PhysicalPos *PhysicalPosClient
	// PhysicalPosManagement manages stores, registers, terminals, devices,
	// device keys and end-of-day policies.
	PhysicalPosManagement *PhysicalPosManagementClient
	// Wallet is the merchant surface of the closed-loop wallet: end users,
	// transfers, withdrawals, top-ups, payments, QR codes and bulk payouts.
	Wallet *WalletClient
	// BankConnect reads your connected bank accounts, balances and
	// transactions, and sends transfers and currency exchange orders.
	BankConnect *BankConnectClient
	// Sanction screens names, wallets, accounts and transactions and runs
	// ongoing monitoring.
	Sanction *SanctionClient
	// Insurance offers, sells and services insurance and submits leads.
	Insurance *InsuranceClient
	// Subscriptions bills customers on plans: subscriptions, plans,
	// customers, saved cards, invoices, the customer portal, catalog
	// products, features, add-ons and metered usage.
	Subscriptions *SubscriptionsClient
	// Kyc runs KYC, KYB and evidence sessions and reads your KYC policy and
	// webhook deliveries.
	Kyc *KycClient
	// Identity reads device and account-linking signals of the browser
	// fingerprint SDK.
	Identity *IdentityClient
	// Marketplace splits charges across sub-dealers and manages the
	// sub-dealers, commissions, payouts and the audit log.
	Marketplace *MarketplaceClient
}

// New validates configuration without making a network request.
func New(config Config) (*Client, error) {
	u, key, err := validateConfig(config)
	if err != nil {
		return nil, err
	}
	hc := http.Client{}
	if config.HTTPClient != nil {
		hc = *config.HTTPClient
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	hc.Timeout = config.Timeout
	if hc.Timeout == 0 {
		hc.Timeout = 30 * time.Second
	}
	max := config.MaxResponseBytes
	if max == 0 {
		max = 8 << 20
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	c := &Client{baseURL: strings.TrimSuffix(u.String(), "/"), keyID: config.KeyID, key: key, http: &hc, now: now, maxRetries: config.MaxRetries, maxResponse: max, refresh: make(chan struct{}, 1)}
	c.Payments = &PaymentClient{client: c}
	c.PaymentLinks = &PaymentLinkClient{client: c}
	c.Fraud = &FraudClient{client: c}
	c.Reference = &ReferenceClient{client: c}
	c.PhysicalPos = &PhysicalPosClient{client: c}
	c.PhysicalPosManagement = &PhysicalPosManagementClient{client: c}
	c.Wallet = newWalletClient(c)
	c.BankConnect = newBankConnectClient(c)
	c.Sanction = &SanctionClient{client: c}
	c.Insurance = &InsuranceClient{client: c}
	c.Subscriptions = newSubscriptionsClient(c)
	c.Kyc = &KycClient{client: c}
	c.Identity = &IdentityClient{client: c}
	c.Marketplace = &MarketplaceClient{client: c}
	return c, nil
}

// Request is a signed merchant API request. Path must be an unescaped path
// under /api/v1/ (or under /api/panel/v1/subscriptions/portal/, the customer
// portal links of subscriptions); use Query for query parameters. A stable IdempotencyKey is
// required by financial mutation helpers and controls safe automatic retries.
type Request struct {
	Method         string
	Path           string
	Query          url.Values
	Body           any
	IdempotencyKey string
}

// Response preserves the gateway envelope, including pageInfo and pending
// financial states. Inspect verified amount, currency and status before order
// fulfillment. A successful HTTP response alone is not proof of payment.
type Response struct {
	StatusCode  int
	RequestID   string
	Envelope    json.RawMessage
	Data        json.RawMessage
	PageInfo    json.RawMessage
	ContentType string
	Bytes       []byte
	retryAfter  string
}

// DecodeData decodes data without converting integer amounts to float64. A
// map/interface target receives json.Number values.
func (r *Response) DecodeData(target any) error {
	d := json.NewDecoder(bytes.NewReader(r.Data))
	d.UseNumber()
	return d.Decode(target)
}

// Do calls a documented merchant endpoint through the same signed transport
// as the high-level clients. It does not invent service-specific validation;
// callers using this escape hatch must follow the endpoint's public schema.
func (c *Client) Do(ctx context.Context, in Request) (*Response, error) {
	return c.do(ctx, in, "")
}

// do sends a request. A non-empty binaryAccept is sent as the Accept header
// and a 2xx body is returned in Response.Bytes without JSON decoding (PDF
// documents); error answers are still read as the JSON envelope.
func (c *Client) do(ctx context.Context, in Request, binaryAccept string) (*Response, error) {
	return c.send(ctx, in, sendOptions{accept: binaryAccept})
}

// sendOptions are transport switches of the internal clients.
type sendOptions struct {
	// accept is a binary Accept header (see do).
	accept string
	// noRetry turns off the retries after a transport error, 429 or 5xx;
	// physical POS payment writes run their own attempts. The single
	// authentication refresh after a 401 is kept.
	noRetry bool
	// unsigned sends the access token without the per-request signature
	// (the bearer-only routes of the subscription, KYC, identity and
	// marketplace dealer tables).
	unsigned bool
	// deleteBody lets a DELETE carry a body (the subscription card
	// deactivation and portal link revocation reasons).
	deleteBody bool
}

func (c *Client) send(ctx context.Context, in Request, opts sendOptions) (*Response, error) {
	binaryAccept := opts.accept
	method := strings.ToUpper(in.Method)
	if method != "GET" && method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return nil, invalid("HTTP method")
	}
	if err := validPath(in.Path); err != nil {
		return nil, err
	}
	if in.IdempotencyKey != "" {
		if err := validIdempotencyFor(in.Path, in.IdempotencyKey); err != nil {
			return nil, err
		}
	}
	if (method == "GET" || (method == "DELETE" && !opts.deleteBody)) && in.Body != nil {
		return nil, invalid("body is not allowed for this method")
	}
	if in.Path == directMethodPaymentPath && in.IdempotencyKey == "" {
		return nil, invalid("stable IdempotencyKey required for a direct method payment")
	}
	var body []byte
	var err error
	if in.Body != nil {
		body, err = CanonicalJSON(in.Body)
		if err != nil {
			return nil, err
		}
		if len(body) > 8<<20 {
			return nil, invalid("request body exceeds 8 MiB")
		}
	}
	if fiatRoute(in.Path) {
		if err = validateMoneyJSON(body, false); err != nil {
			return nil, err
		}
	}
	if err = validatePaymentBody(in.Path, body); err != nil {
		return nil, err
	}
	path := in.Path
	if len(in.Query) > 0 {
		path += "?" + in.Query.Encode()
	}
	if len(path) > 16384 {
		return nil, invalid("request URL exceeds limit")
	}
	safeRetry := method == "GET" || in.IdempotencyKey != ""
	refreshed := false
	for attempt := 0; ; attempt++ {
		token, e := c.accessToken(ctx)
		if e != nil {
			return nil, e
		}
		r, e := c.exchange(ctx, method, path, body, in.IdempotencyKey, token, binaryAccept, !opts.unsigned)
		if e == nil && r.StatusCode >= 200 && r.StatusCode < 300 {
			return r, nil
		}
		if e == nil && r.StatusCode == 401 && safeRetry && !refreshed {
			c.invalidate(token)
			refreshed = true
			// A refreshed authentication retry uses the same body and key.
			continue
		}
		if e == nil && method == "POST" && splitPending(in.Path, r) {
			return r, nil
		}
		retry := e != nil || r.StatusCode == 429 || r.StatusCode == 500 || r.StatusCode == 502 || r.StatusCode == 503 || r.StatusCode == 504
		if safeRetry && retry && !opts.noRetry && attempt < c.maxRetries {
			delay := time.Duration(100*(1<<uint(attempt))) * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if e != nil {
			return nil, e
		}
		return r, apiError(r)
	}
}

func validPath(path string) error {
	if !(strings.HasPrefix(path, "/api/v1/") || strings.HasPrefix(path, subscriptionPortalPanelBase+"/")) || len(path) > 4096 || strings.ContainsAny(path, "%?#\\\r\n\t") || strings.Contains(path, "//") || (path == "/api/v1/auth" || strings.HasPrefix(path, "/api/v1/auth/")) {
		return invalid("merchant API path")
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return invalid("path traversal")
		}
	}
	for _, r := range path {
		if r < 0x21 || r > 0x7e {
			return invalid("merchant API path")
		}
	}
	return nil
}

func validIdempotency(key string) bool { return len(key) >= 16 && safeHeader(key, 128) }

// validIdempotencyFor applies the key format of the route: payment links,
// physical POS, bank connect and insurance accept 8–200 printable ASCII
// characters, the wallet 1–128, subscriptions, KYC, identity and the
// marketplace dealer routes 1–200, every other route 16–128.
func validIdempotencyFor(path, key string) error {
	if routeTableKeyRoute(path) {
		if !validRouteTableKey(key) {
			return invalid("IdempotencyKey must be 1–200 printable ASCII characters without spaces for subscriptions, KYC, identity and marketplace dealers")
		}
		return nil
	}
	if bankConnectRoute(path) || insuranceRoute(path) {
		if !validPrintableKey(key) {
			return invalid("IdempotencyKey must be 8–200 printable ASCII characters without spaces for bank connect and insurance")
		}
		return nil
	}
	if walletRoute(path) {
		if !validWalletIdempotency(key) {
			return invalid("IdempotencyKey must be 1–128 printable ASCII characters without spaces for the wallet")
		}
		return nil
	}
	if paymentLinkRoute(path) {
		if !validPaymentLinkIdempotency(key) {
			return invalid("IdempotencyKey must be 8–200 printable ASCII characters without spaces for payment links")
		}
		return nil
	}
	if physicalPosRoute(path) {
		if !validPhysicalPosIdempotency(key) {
			return invalid("IdempotencyKey must be 8–200 printable ASCII characters without spaces for physical POS")
		}
		return nil
	}
	if !validIdempotency(key) {
		return invalid("IdempotencyKey must be 16–128 printable ASCII characters")
	}
	return nil
}

// exchange sends one HTTP request. signed adds the per-request Ed25519
// signature (X-Key-Id, X-Date, X-Nonce, X-Signature and, with a body,
// Digest); without it only the access token authenticates the call.
func (c *Client) exchange(ctx context.Context, method, path string, body []byte, idem, token, binaryAccept string, signed bool) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, invalid("request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if signed {
		nonceBytes := make([]byte, 16)
		if _, err = rand.Read(nonceBytes); err != nil {
			return nil, &APIError{Code: "RANDOM_SOURCE_UNAVAILABLE"}
		}
		nonce := hex.EncodeToString(nonceBytes)
		date := c.now().UTC().Format("2006-01-02T15:04:05Z")
		digest := ""
		if body != nil {
			digest = Digest(body)
			req.Header.Set("Digest", digest)
		}
		req.Header.Set("X-Key-Id", c.keyID)
		req.Header.Set("X-Date", date)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(c.key, []byte(CanonicalRequest(method, path, date, nonce, digest)))))
	}
	accept := "application/json"
	if binaryAccept != "" {
		accept = binaryAccept
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "lunixi-go/"+Version)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &APIError{Code: "TRANSPORT_ERROR"}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponse+1))
	if err != nil {
		return nil, &APIError{StatusCode: response.StatusCode, Code: "RESPONSE_READ_FAILED"}
	}
	if int64(len(raw)) > c.maxResponse {
		return nil, &APIError{StatusCode: response.StatusCode, Code: "RESPONSE_TOO_LARGE"}
	}
	r := &Response{StatusCode: response.StatusCode, ContentType: response.Header.Get("Content-Type")}
	if value := response.Header.Get("X-Request-Id"); safeHeader(value, 200) {
		r.RequestID = value
	}
	if value := response.Header.Get("Retry-After"); safeHeader(value, 64) {
		r.retryAfter = value
	}
	if binaryAccept != "" && r.StatusCode >= 200 && r.StatusCode < 300 {
		r.Bytes = raw
		return r, nil
	}
	if strings.Contains(strings.ToLower(r.ContentType), "json") || json.Valid(raw) {
		if !json.Valid(raw) {
			return nil, &APIError{StatusCode: response.StatusCode, Code: "INVALID_JSON_RESPONSE", RequestID: r.RequestID}
		}
		if fiatRoute(path) {
			if err := validateMoneyJSON(raw, true); err != nil {
				return nil, err
			}
		}
		r.Envelope = append(json.RawMessage(nil), raw...)
		var envelope struct {
			Data     json.RawMessage `json:"data"`
			PageInfo json.RawMessage `json:"pageInfo"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			r.Data = envelope.Data
			r.PageInfo = envelope.PageInfo
		}
		if len(r.Data) == 0 {
			r.Data = append(json.RawMessage(nil), raw...)
		}
	} else {
		r.Bytes = raw
	}
	return r, nil
}

func apiError(r *Response) *APIError {
	var value struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(r.Envelope, &value)
	if !safeCode.MatchString(value.Code) {
		value.Code = "HTTP_ERROR"
	}
	return &APIError{StatusCode: r.StatusCode, Code: value.Code, RequestID: r.RequestID}
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	token, expiry := c.token, c.tokenExpiry
	c.tokenMu.Unlock()
	if token != "" && c.now().Before(expiry) {
		return token, nil
	}
	select {
	case c.refresh <- struct{}{}:
		defer func() { <-c.refresh }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	c.tokenMu.Lock()
	token, expiry = c.token, c.tokenExpiry
	c.tokenMu.Unlock()
	if token != "" && c.now().Before(expiry) {
		return token, nil
	}
	r, err := c.exchange(ctx, "POST", "/api/v1/auth/token", nil, "", "", "", true)
	if err != nil {
		return "", err
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return "", apiError(r)
	}
	var auth struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(r.Data, &auth) != nil || !safeHeader(auth.AccessToken, 8192) || auth.ExpiresIn <= 0 || auth.ExpiresIn > 86400 {
		return "", &APIError{Code: "INVALID_AUTH_RESPONSE", RequestID: r.RequestID}
	}
	lifetime := time.Duration(auth.ExpiresIn) * time.Second
	skew := 30 * time.Second
	if lifetime <= skew {
		skew = lifetime / 2
	}
	c.tokenMu.Lock()
	c.token = auth.AccessToken
	c.tokenExpiry = c.now().Add(lifetime - skew)
	c.tokenMu.Unlock()
	return auth.AccessToken, nil
}

func (c *Client) invalidate(failed string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token == failed {
		c.token = ""
		c.tokenExpiry = time.Time{}
	}
}

// InvalidateToken clears the in-memory token cache, for example after key
// permissions change. Key rotation itself requires constructing a new client.
func (c *Client) InvalidateToken() {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.token = ""
	c.tokenExpiry = time.Time{}
}

func splitPending(path string, r *Response) bool {
	if path != "/api/v1/payments/split" || r.StatusCode != 400 {
		return false
	}
	var value struct {
		SplitPaymentID string `json:"splitPaymentId"`
		Status         string `json:"status"`
	}
	if json.Unmarshal(r.Data, &value) != nil || value.SplitPaymentID == "" {
		return false
	}
	switch value.Status {
	case "COMPOSING", "AUTHORIZING", "AWAITING_3DS", "AWAITING_ACTION", "AWAITING_CREDIT_APPROVAL", "AWAITING_REPLACEMENT_CARD", "AUTHORIZED", "CAPTURING":
		return true
	}
	return false
}

package lunixi

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var resourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$`)

// PaymentClient implements checkout forms, payment lifecycle and split
// payment merchant endpoints. It never determines an order's paid state.
type PaymentClient struct{ client *Client }

// BasketItem is a checkout basket line; Price is an integer minor-unit line
// total, not a per-unit float. Additional insurance data follows the gateway
// schema and is passed without guessing insurance-specific field rules.
type BasketItem struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	ItemType          string         `json:"itemType"`
	Price             string         `json:"price"`
	Category1         string         `json:"category1,omitempty"`
	Category2         string         `json:"category2,omitempty"`
	InsuranceData     map[string]any `json:"insuranceData,omitempty"`
	InsuranceCategory string         `json:"insuranceCategory,omitempty"`
	InsuranceTypes    []string       `json:"insuranceTypes,omitempty"`
}

// Buyer holds the documented buyer details used by hosted checkout. Values
// are sent to Lunixi only and must never be copied to application logs.
type Buyer struct {
	ID                  string `json:"id,omitempty"`
	Name                string `json:"name,omitempty"`
	Surname             string `json:"surname,omitempty"`
	IdentityNumber      string `json:"identityNumber,omitempty"`
	Email               string `json:"email,omitempty"`
	GSMNumber           string `json:"gsmNumber,omitempty"`
	IP                  string `json:"ip,omitempty"`
	City                string `json:"city,omitempty"`
	Country             string `json:"country,omitempty"`
	ZipCode             string `json:"zipCode,omitempty"`
	RegistrationDate    string `json:"registrationDate,omitempty"`
	LastLoginDate       string `json:"lastLoginDate,omitempty"`
	RegistrationAddress string `json:"registrationAddress,omitempty"`
}

// Address is a billing or shipping address accepted by the payment API.
type Address struct {
	ContactName string `json:"contactName,omitempty"`
	Address     string `json:"address,omitempty"`
	City        string `json:"city,omitempty"`
	Country     string `json:"country,omitempty"`
	ZipCode     string `json:"zipCode,omitempty"`
}

// IntentRequest creates a checkout form token. Amount is the basket total
// before a provider-calculated installment fee. Installments are selected in
// the shared form, not prescribed in the intent request.
type IntentRequest struct {
	Amount          string         `json:"amount"`
	Currency        string         `json:"currency"`
	OrderID         string         `json:"orderId"`
	CallbackURL     string         `json:"callbackUrl"`
	Buyer           *Buyer         `json:"buyer,omitempty"`
	BillingAddress  *Address       `json:"billingAddress,omitempty"`
	ShippingAddress *Address       `json:"shippingAddress,omitempty"`
	BasketItems     []BasketItem   `json:"basketItems,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	Preauth         bool           `json:"preauth,omitempty"`
	Locale          string         `json:"locale,omitempty"`
	Description     string         `json:"description,omitempty"`
	CustomerID      string         `json:"customerId,omitempty"`
	CardUserKey     string         `json:"cardUserKey,omitempty"`
	PaymentMethod   string         `json:"paymentMethod,omitempty"`
	WalletProvider  string         `json:"walletProvider,omitempty"`
	PaymentChannel  string         `json:"paymentChannel,omitempty"`
	PaymentGroup    string         `json:"paymentGroup,omitempty"`
	Force3D         *bool          `json:"force3D,omitempty"`
	Settings        map[string]any `json:"settings,omitempty"`
}

func (r IntentRequest) validate() error {
	if !positiveMoney(r.Amount) {
		return invalid("amount must be positive integer minor units within JSON safe range")
	}
	if len(r.Currency) != 3 || r.Currency != strings.ToUpper(r.Currency) {
		return invalid("currency must be a three-letter uppercase code")
	}
	for _, x := range r.Currency {
		if x < 'A' || x > 'Z' {
			return invalid("currency")
		}
	}
	if strings.TrimSpace(r.OrderID) == "" || len(r.OrderID) > 200 {
		return invalid("orderId")
	}
	u, e := url.Parse(r.CallbackURL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return invalid("callbackUrl must use HTTPS")
	}
	if len(r.BasketItems) > 0 {
		var total int64
		for _, item := range r.BasketItems {
			if !positiveMoney(item.Price) || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || (item.ItemType != "PHYSICAL" && item.ItemType != "VIRTUAL") {
				return invalid("basket item")
			}
			if moneyInteger(item.Price) > math.MaxInt64-total {
				return invalid("basket total overflow")
			}
			total += moneyInteger(item.Price)
		}
		if total != moneyInteger(r.Amount) {
			return invalid("basket total must equal amount")
		}
	}
	return nil
}

func positiveMoney(amount string) bool { return validMinor(amount, true, false) }
func moneyInteger(amount string) int64 { n, _ := strconv.ParseInt(amount, 10, 64); return n }
func idPath(id string) (string, error) {
	if !resourceID.MatchString(id) || id == "." || id == ".." {
		return "", invalid("resource id")
	}
	return id, nil
}
func requireKey(key string) error {
	if !validIdempotency(key) {
		return invalid("stable IdempotencyKey required (16–128 printable ASCII characters)")
	}
	return nil
}

// CreateIntent creates a hosted checkout session. Persist key per payment
// attempt before calling; reuse it after a timeout instead of charging again.
func (p *PaymentClient) CreateIntent(ctx context.Context, r IntentRequest, key string) (*Response, error) {
	if e := r.validate(); e != nil {
		return nil, e
	}
	if e := requireKey(key); e != nil {
		return nil, e
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/intents", Body: r, IdempotencyKey: key})
}

// Get reads the authoritative gateway payment state.
func (p *PaymentClient) Get(ctx context.Context, id string) (*Response, error) {
	id, e := idPath(id)
	if e != nil {
		return nil, e
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/" + id})
}

// List returns the original gateway envelope including pagination metadata.
func (p *PaymentClient) List(ctx context.Context, filters url.Values) (*Response, error) {
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments", Query: filters})
}

// Methods reads the merchant's enabled method/provider capabilities.
func (p *PaymentClient) Methods(ctx context.Context) (*Response, error) {
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/methods"})
}

// Capture captures the full authorization when amount is nil, or a specified
// positive integer amount. It must reference the original payment account.
func (p *PaymentClient) Capture(ctx context.Context, id string, amount *string, key string) (*Response, error) {
	return p.mutation(ctx, "capture", id, amount, "", key)
}

// Refund refunds the full refundable payment when amount is nil, or a partial
// amount. Unknown outcomes must be reconciled with the same key.
func (p *PaymentClient) Refund(ctx context.Context, id string, amount *string, reason, key string) (*Response, error) {
	return p.mutation(ctx, "refund", id, amount, reason, key)
}

// Void releases an authorization; it does not refund a captured payment.
func (p *PaymentClient) Void(ctx context.Context, id, key string) (*Response, error) {
	id, e := idPath(id)
	if e != nil {
		return nil, e
	}
	if e = requireKey(key); e != nil {
		return nil, e
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/" + id + "/void", IdempotencyKey: key})
}

func (p *PaymentClient) mutation(ctx context.Context, action, id string, amount *string, reason, key string) (*Response, error) {
	id, e := idPath(id)
	if e != nil {
		return nil, e
	}
	if e = requireKey(key); e != nil {
		return nil, e
	}
	if amount != nil && !positiveMoney(*amount) {
		return nil, invalid("amount")
	}
	if len(reason) > 500 {
		return nil, invalid("reason length")
	}
	body := map[string]any{"paymentIntentId": id}
	if amount != nil {
		body["amount"] = *amount
	}
	if reason != "" {
		body["reason"] = reason
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/" + id + "/" + action, Body: body, IdempotencyKey: key})
}

// Split validates the documented split amount/leg contract and starts or
// continues it. Supply the same body and key for every continuation. Pending
// split states can be returned on HTTP 400 and remain explicitly unpaid.
func (p *PaymentClient) Split(ctx context.Context, body map[string]any, key string) (*Response, error) {
	if e := requireKey(key); e != nil {
		return nil, e
	}
	raw, e := CanonicalJSON(body)
	if e != nil {
		return nil, e
	}
	var request struct {
		TotalAmount string `json:"totalAmount"`
		Currency    string `json:"currency"`
		OwnerRef    string `json:"ownerRef"`
		Legs        []struct {
			Amount string `json:"amount"`
		} `json:"legs"`
	}
	if json.Unmarshal(raw, &request) != nil || !positiveMoney(request.TotalAmount) || len(request.Currency) != 3 || request.OwnerRef == "" || len(request.Legs) < 1 || len(request.Legs) > 5 {
		return nil, invalid("split request")
	}
	var total int64
	for _, leg := range request.Legs {
		if !positiveMoney(leg.Amount) || moneyInteger(leg.Amount) > math.MaxInt64-total {
			return nil, invalid("split leg amount")
		}
		total += moneyInteger(leg.Amount)
	}
	if total != moneyInteger(request.TotalAmount) {
		return nil, invalid("split amount mismatch")
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/split", Body: body, IdempotencyKey: key})
}

// GetSplit returns all legs and aggregate amounts; COMPLETED alone should not
// replace amount/currency/balancing/reconciliation checks in the order adapter.
func (p *PaymentClient) GetSplit(ctx context.Context, id string) (*Response, error) {
	id, e := idPath(id)
	if e != nil {
		return nil, e
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/split/" + id})
}

// RefundSplit asks the gateway to distribute a refund across refundable legs.
// Partial or failed allocations return an APIError and require review.
func (p *PaymentClient) RefundSplit(ctx context.Context, id string, amount string, reason, key string) (*Response, error) {
	id, e := idPath(id)
	if e != nil {
		return nil, e
	}
	if e = requireKey(key); e != nil {
		return nil, e
	}
	if !positiveMoney(amount) || len(reason) > 500 {
		return nil, invalid("split refund")
	}
	body := map[string]any{"amount": amount}
	if reason != "" {
		body["reason"] = reason
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/split/" + id + "/refund", Body: body, IdempotencyKey: key})
}

var optionalOperationAmountRoute = regexp.MustCompile(`^/api/v1/payments/(?:split/)?[^/]+/(?:capture|refund)$`)

func validatePaymentBody(path string, body []byte) error {
	if optionalOperationAmountRoute.MatchString(path) && len(body) > 0 {
		var object map[string]json.RawMessage
		if json.Unmarshal(body, &object) != nil || object == nil {
			return invalid("operation body must be an object")
		}
		if raw, present := object["amount"]; present {
			var amount string
			if json.Unmarshal(raw, &amount) != nil || !positiveMoney(amount) {
				return invalid("explicit operation amount must be positive minor-unit text")
			}
		}
		return nil
	}
	if path != "/api/v1/payments/intents" && path != "/api/v1/payments/marketplace/intents" && path != "/api/v1/payments/direct/payment" {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return invalid("payment body must be an object")
	}
	for _, name := range []string{"price", "paidPrice", "autoCapture"} {
		if _, ok := object[name]; ok {
			return invalid("removed payment field: " + name)
		}
	}
	var amount string
	if json.Unmarshal(object["amount"], &amount) != nil || !positiveMoney(amount) {
		return invalid("amount must be positive integer minor units")
	}
	return nil
}

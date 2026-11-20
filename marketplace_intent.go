package lunixi

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
)

// MarketplaceSeller is one seller leg of a marketplace split: the
// aggregate amount of one sub-dealer (group the basket by vendor and add
// up). AmountMinor is positive minor-unit text; the legs add up to the
// payment amount. Scenario is an optional per-request commission scenario
// override (for example {"type": "DYNAMIC_RATE", "ratePercent": "5"}).
type MarketplaceSeller struct {
	SubDealerID string         `json:"subDealerId"`
	AmountMinor string         `json:"amountMinor"`
	Scenario    map[string]any `json:"scenario,omitempty"`
}

// MarketplaceIntentRequest is the body of CreateMarketplaceIntent
// (POST /payments/marketplace/intents): the checkout intent fields plus the
// sellers and their shares. The order details of IntentRequest are required
// (buyer IP optional).
type MarketplaceIntentRequest struct {
	IntentRequest
	Sellers           []MarketplaceSeller `json:"sellers"`
	MultiSellerPolicy string              `json:"multiSellerPolicy,omitempty"`
}

func (r MarketplaceIntentRequest) validate() error {
	if e := r.IntentRequest.check("A marketplace payment intent"); e != nil {
		return e
	}
	if r.MultiSellerPolicy != "" {
		if e := oneOf(r.MultiSellerPolicy, MarketplaceMultiSellerPolicies(), "multiSellerPolicy"); e != nil {
			return e
		}
	}
	_, e := sellerLegs(r.Sellers, r.Amount)
	return e
}

// sellerLegs checks the seller legs of a split (at least one; a non-blank
// subDealerId of at most 200 characters; a positive minor-unit AmountMinor;
// the legs adding up to amount) and returns them with trimmed ids.
func sellerLegs(sellers []MarketplaceSeller, amount string) ([]MarketplaceSeller, error) {
	if len(sellers) == 0 {
		return nil, invalid("a marketplace split requires at least one seller leg")
	}
	legs := make([]MarketplaceSeller, len(sellers))
	var total int64
	for i, seller := range sellers {
		label := "sellers[" + strconv.Itoa(i) + "]"
		id := strings.TrimSpace(seller.SubDealerID)
		if id == "" || len(id) > 200 {
			return nil, invalid(label + ".subDealerId is required")
		}
		if !positiveMoney(seller.AmountMinor) {
			return nil, invalid(label + ".amountMinor must be positive minor-unit text")
		}
		share := moneyInteger(seller.AmountMinor)
		if share > math.MaxInt64-total {
			return nil, invalid("sellers total overflow")
		}
		total += share
		legs[i] = MarketplaceSeller{SubDealerID: id, AmountMinor: seller.AmountMinor, Scenario: seller.Scenario}
	}
	if !positiveMoney(amount) || total != moneyInteger(amount) {
		return nil, invalid("the seller legs must add up to amount; the gateway refuses any other total")
	}
	return legs, nil
}

// CreateMarketplaceIntent creates a hosted checkout session split between
// sellers. The sellers' AmountMinor values must add up to Amount; any other
// total is refused before sending (the gateway refuses it with 400 and
// charges nothing). key is optional: pass a stable key per payment attempt to
// make retries safe, or "" to send none.
func (p *PaymentClient) CreateMarketplaceIntent(ctx context.Context, r MarketplaceIntentRequest, key string) (*Response, error) {
	if e := r.validate(); e != nil {
		return nil, e
	}
	if key != "" {
		if e := requireKey(key); e != nil {
			return nil, e
		}
	}
	if r.EnabledInstallments != nil {
		r.EnabledInstallments = slices.Sorted(slices.Values(r.EnabledInstallments))
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/marketplace/intents", Body: r, IdempotencyKey: key})
}

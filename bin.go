package lunixi

import (
	"context"
	"regexp"
	"slices"
	"strings"
)

const binPath = "/api/v1/bin"

// binOrPanPattern accepts a 6 or 8 digit BIN or a card number prefix of up to
// 19 digits. The value itself never appears in an error message.
var binOrPanPattern = regexp.MustCompile(`^[0-9]{6,19}$`)

func validateBinOrPan(value string) error {
	if !binOrPanPattern.MatchString(value) {
		return invalid("binOrPan must be 6 to 19 digits (a BIN or a card number prefix)")
	}
	return nil
}

// BinInfo reads the card type, bank and installment eligibility of a 6 or 8
// digit BIN or a card number prefix (POST /bin/info). Prefer sending the BIN
// only; a full card number is never logged or echoed by this SDK.
func (p *PaymentClient) BinInfo(ctx context.Context, binOrPan string) (*Response, error) {
	if e := validateBinOrPan(binOrPan); e != nil {
		return nil, e
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: binPath + "/info", Body: map[string]any{"binOrPan": binOrPan}})
}

// InstallmentOptionsRequest is the body of InstallmentOptions
// (POST /bin/installments). Give Amount (positive minor units, the basket
// total) with Currency, or IntentID to quote against an existing intent.
// Installment greater than 1 together with BinOrPan returns the binding
// quoteToken to send as the installmentQuoteToken of a card payment.
type InstallmentOptionsRequest struct {
	Amount        string `json:"amount,omitempty"`
	Currency      string `json:"currency,omitempty"`
	IntentID      string `json:"intentId,omitempty"`
	BinOrPan      string `json:"binOrPan,omitempty"`
	Installment   int    `json:"installment,omitempty"`
	CardBrand     string `json:"cardBrand,omitempty"`
	PaymentMethod string `json:"paymentMethod,omitempty"`
	// EnabledInstallments limits the table to these counts, e.g. []int{1, 2, 3}
	// for a product page that allows at most 3. Same rules as
	// IntentRequest.EnabledInstallments: integers 1–36, no duplicates, 1
	// included; nil means no restriction. Sent in ascending order.
	EnabledInstallments []int `json:"enabledInstallments,omitempty"`
}

func (r InstallmentOptionsRequest) body() (InstallmentOptionsRequest, error) {
	if r.Amount == "" && strings.TrimSpace(r.IntentID) == "" {
		return r, invalid("installment options require amount or intentId")
	}
	if r.Amount != "" && !positiveMoney(r.Amount) {
		return r, invalid("amount must be positive integer minor units within JSON safe range")
	}
	if r.Currency != "" {
		r.Currency = strings.ToUpper(r.Currency)
		if !currencyPattern.MatchString(r.Currency) {
			return r, invalid("currency must be a three-letter code")
		}
	}
	if r.IntentID != "" {
		if _, e := idPath(r.IntentID); e != nil {
			return r, invalid("intentId")
		}
	}
	if r.BinOrPan != "" {
		if e := validateBinOrPan(r.BinOrPan); e != nil {
			return r, e
		}
	}
	if r.Installment != 0 && (r.Installment < MinEnabledInstallment || r.Installment > MaxEnabledInstallment) {
		return r, invalid("installment must be from 1 to 36")
	}
	if e := validateEnabledInstallments(r.EnabledInstallments); e != nil {
		return r, e
	}
	if r.EnabledInstallments != nil {
		r.EnabledInstallments = slices.Sorted(slices.Values(r.EnabledInstallments))
	}
	return r, nil
}

// InstallmentOptions returns the installment table for an amount and card
// (POST /bin/installments). The provider's installment fee is quoted here; the
// merchant never adds it to amount.
func (p *PaymentClient) InstallmentOptions(ctx context.Context, request InstallmentOptionsRequest) (*Response, error) {
	body, e := request.body()
	if e != nil {
		return nil, e
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: binPath + "/installments", Body: body})
}

// CardTaxonomy lists the card brand, family and bank reference values
// (GET /bin/card-taxonomy).
func (p *PaymentClient) CardTaxonomy(ctx context.Context) (*Response, error) {
	return p.client.Do(ctx, Request{Method: "GET", Path: binPath + "/card-taxonomy"})
}

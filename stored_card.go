package lunixi

import (
	"context"
	"maps"
	"net/url"
	"strings"
)

const storedCardsPath = "/api/v1/payments/cards"

// StoreCardRequest is the body of StoreCard (POST /payments/cards): the card
// is verified and stored under CardUserKey, your stable identifier of the
// buyer. Card holds the gateway card fields (cardHolderName, cardNumber,
// expireMonth, expireYear, cvcNumber, ...). Card values are sent to Lunixi
// only; never log the request. CallbackURL receives the buyer after a 3D
// Secure verification.
type StoreCardRequest struct {
	Card        map[string]any `json:"card"`
	CardUserKey string         `json:"cardUserKey"`
	CallbackURL string         `json:"callbackUrl"`
	Currency    string         `json:"currency,omitempty"`
}

func (r StoreCardRequest) body() (StoreCardRequest, error) {
	if len(r.Card) == 0 {
		return r, invalid("store card requires card fields")
	}
	if err := validateCardUserKey(r.CardUserKey); err != nil {
		return r, err
	}
	u, err := url.Parse(r.CallbackURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return r, invalid("callbackUrl must use HTTPS")
	}
	if r.Currency != "" {
		r.Currency = strings.ToUpper(r.Currency)
		if !currencyPattern.MatchString(r.Currency) {
			return r, invalid("currency must be a three-letter code")
		}
	}
	r.Card = maps.Clone(r.Card)
	return r, nil
}

func validateCardUserKey(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 200 || strings.ContainsAny(value, "\r\n\t") {
		return invalid("cardUserKey is required (your stable buyer identifier, at most 200 characters)")
	}
	return nil
}

func storedCardPath(storedCardToken string) (string, error) {
	token, err := idPath(storedCardToken)
	if err != nil {
		return "", invalid("storedCardToken")
	}
	return storedCardsPath + "/" + token, nil
}

// StoreCard verifies and stores a card (POST /payments/cards). key is
// required; reuse it when retrying the same request.
func (p *PaymentClient) StoreCard(ctx context.Context, request StoreCardRequest, key string) (*Response, error) {
	body, err := request.body()
	if err != nil {
		return nil, err
	}
	if err = requireKey(key); err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: storedCardsPath, Body: body, IdempotencyKey: key})
}

// ListStoredCards lists the stored cards of a buyer (GET /payments/cards).
func (p *PaymentClient) ListStoredCards(ctx context.Context, cardUserKey string) (*Response, error) {
	if err := validateCardUserKey(cardUserKey); err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: storedCardsPath, Query: url.Values{"cardUserKey": {cardUserKey}}})
}

// GetStoredCard reads one stored card by its token (GET /payments/cards/{token}).
func (p *PaymentClient) GetStoredCard(ctx context.Context, storedCardToken string) (*Response, error) {
	path, err := storedCardPath(storedCardToken)
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: path})
}

// DeactivateStoredCard deactivates a stored card (DELETE /payments/cards/{token}).
// key is required.
func (p *PaymentClient) DeactivateStoredCard(ctx context.Context, storedCardToken, key string) (*Response, error) {
	path, err := storedCardPath(storedCardToken)
	if err != nil {
		return nil, err
	}
	if err = requireKey(key); err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "DELETE", Path: path, IdempotencyKey: key})
}

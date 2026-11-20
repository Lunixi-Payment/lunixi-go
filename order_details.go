package lunixi

import (
	"slices"
	"strconv"
	"strings"
)

// Order details every payment endpoint requires: the buyer, the billing
// address and at least one basket item; a basket with a PHYSICAL item also
// requires the shipping address. Payment providers do not start a payment
// without them.
//
// The gateway refuses a missing field with OrderFieldErrorCodeFieldRequired
// and an invalid one with OrderFieldErrorCodeFieldInvalid; details.field is
// the dotted path of the first failing field, details.reason an
// OrderFieldReason* value and details.fields every field failing for the same
// reason. The SDK checks the same rule before sending and names the same
// paths (MissingOrderDetails).

// Error codes of a missing or invalid order field.
const (
	OrderFieldErrorCodeFieldRequired = "payment.order.field_required"
	OrderFieldErrorCodeFieldInvalid  = "payment.order.field_invalid"
)

// details.reason of an order field error.
const (
	OrderFieldReasonRequired             = "required"
	OrderFieldReasonRequiredWhenPhysical = "required_when_physical"
	OrderFieldReasonInvalidType          = "invalid_type"
	OrderFieldReasonInvalidFormat        = "invalid_format"
	OrderFieldReasonTooLong              = "too_long"
	OrderFieldReasonTooFewItems          = "too_few_items"
)

var orderDetailsRequiredFields = []string{"buyer", "billingAddress", "basketItems"}

var buyerRequiredFields = []string{"name", "surname", "identityNumber", "email", "gsmNumber", "city", "country", "zipCode", "ip"}

var checkoutIntentBuyerOptionalFields = []string{"ip"}

var addressRequiredFields = []string{"contactName", "address", "city", "country", "zipCode"}

var basketItemRequiredFields = []string{"id", "name", "category1", "itemType", "price"}

// OrderDetailsRequiredFields returns the order objects every payment endpoint
// requires. The returned slice is a fresh copy.
func OrderDetailsRequiredFields() []string { return slices.Clone(orderDetailsRequiredFields) }

// BuyerRequiredFields returns the buyer fields that must be non-empty text.
// identityNumber is the Turkish ID or passport number; ip is the buyer's
// browser IP (IPv4 or IPv6). The returned slice is a fresh copy.
func BuyerRequiredFields() []string { return slices.Clone(buyerRequiredFields) }

// CheckoutIntentBuyerOptionalFields returns the buyer fields a checkout intent
// (CreateIntent, CreateMarketplaceIntent, Marketplace.CreateCheckoutIntent)
// may leave out: the Lunixi form takes the IP from the buyer's browser. A
// given ip is still sent and wins. The returned slice is a fresh copy.
func CheckoutIntentBuyerOptionalFields() []string {
	return slices.Clone(checkoutIntentBuyerOptionalFields)
}

// AddressRequiredFields returns the fields of billingAddress and
// shippingAddress that must be non-empty text. The returned slice is a fresh
// copy.
func AddressRequiredFields() []string { return slices.Clone(addressRequiredFields) }

// BasketItemRequiredFields returns the fields of each basketItems entry the
// gateway requires. The returned slice is a fresh copy.
func BasketItemRequiredFields() []string { return slices.Clone(basketItemRequiredFields) }

// OrderDetailsOptions selects the rule variant of MissingOrderDetails.
type OrderDetailsOptions struct {
	// WithoutBasket checks the buyer and billing address only (card storage,
	// wallet card top-up).
	WithoutBasket bool
	// CheckoutIntent does not require CheckoutIntentBuyerOptionalFields.
	CheckoutIntent bool
}

// MissingOrderDetails returns the paths of the missing order fields of a
// payment body, in the gateway's format: buyer, buyer.email,
// billingAddress.city, basketItems, basketItems.0.category1, shippingAddress.
// An empty list means the order details are complete. body is any value that
// encodes to a JSON object (a request struct or a map); an error is returned
// only when it cannot be encoded.
func MissingOrderDetails(body any, options OrderDetailsOptions) ([]string, error) {
	value, err := toGeneric(body)
	if err != nil {
		return nil, err
	}
	object, _ := value.(map[string]any)
	missing := missingOrderDetails(object, options)
	if missing == nil {
		missing = []string{}
	}
	return missing, nil
}

func missingOrderDetails(body map[string]any, options OrderDetailsOptions) []string {
	buyerFields := buyerRequiredFields
	if options.CheckoutIntent {
		buyerFields = slices.DeleteFunc(slices.Clone(buyerRequiredFields), func(field string) bool {
			return slices.Contains(checkoutIntentBuyerOptionalFields, field)
		})
	}
	missing := missingFieldsOf(body["buyer"], buyerFields, "buyer")
	missing = append(missing, missingFieldsOf(body["billingAddress"], addressRequiredFields, "billingAddress")...)
	if options.WithoutBasket {
		return missing
	}
	items, _ := body["basketItems"].([]any)
	if len(items) == 0 {
		return append(missing, "basketItems")
	}
	physical := false
	for index, entry := range items {
		path := "basketItems." + strconv.Itoa(index)
		item, ok := entry.(map[string]any)
		if !ok {
			missing = append(missing, path)
			continue
		}
		for _, field := range basketItemRequiredFields {
			value, has := item[field]
			if text, isText := value.(string); !has || value == nil || (isText && strings.TrimSpace(text) == "") {
				missing = append(missing, path+"."+field)
			}
		}
		physical = physical || item["itemType"] == "PHYSICAL"
	}
	shipping, hasShipping := body["shippingAddress"]
	if hasShipping && shipping == nil {
		hasShipping = false
	}
	switch {
	case physical && !hasShipping:
		missing = append(missing, "shippingAddress")
	case hasShipping:
		missing = append(missing, missingFieldsOf(shipping, addressRequiredFields, "shippingAddress")...)
	}
	return missing
}

// missingFieldsOf names the object itself when it is absent or not an object,
// otherwise each listed field that is not non-empty text.
func missingFieldsOf(value any, fields []string, path string) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return []string{path}
	}
	var missing []string
	for _, field := range fields {
		if text, isText := object[field].(string); !isText || strings.TrimSpace(text) == "" {
			missing = append(missing, path+"."+field)
		}
	}
	return missing
}

// requireOrderDetails fails with ErrConfiguration naming the missing order
// fields (never their values).
func requireOrderDetails(body map[string]any, label string, options OrderDetailsOptions) error {
	if missing := missingOrderDetails(body, options); len(missing) > 0 {
		return invalid(label + " requires the order details: " + strings.Join(missing, ", "))
	}
	return nil
}

// requireOrderDetailsOf applies requireOrderDetails to a typed request.
func requireOrderDetailsOf(request any, label string, options OrderDetailsOptions) error {
	value, err := toGeneric(request)
	if err != nil {
		return err
	}
	object, _ := value.(map[string]any)
	return requireOrderDetails(object, label, options)
}

// orderDetailsRoute is a POST route that carries the order details.
type orderDetailsRoute struct {
	label   string
	options OrderDetailsOptions
}

var orderDetailsRoutes = map[string]orderDetailsRoute{
	"/api/v1/payments/intents":             {"A payment intent", OrderDetailsOptions{CheckoutIntent: true}},
	"/api/v1/payments/marketplace/intents": {"A marketplace payment intent", OrderDetailsOptions{CheckoutIntent: true}},
	"/api/v1/payments/marketplace/2d":      {"A marketplace card charge", OrderDetailsOptions{}},
	"/api/v1/payments/marketplace/3d":      {"A marketplace card charge", OrderDetailsOptions{}},
	directMethodPaymentPath:                {"A direct method payment", OrderDetailsOptions{}},
	"/api/v1/payments/split":               {"A split payment", OrderDetailsOptions{}},
	storedCardsPath:                        {"Storing a card", OrderDetailsOptions{WithoutBasket: true}},
	"/api/v1/wallet/topups/card":           {"A wallet card top-up", OrderDetailsOptions{WithoutBasket: true}},
}

// validateOrderDetailsBody applies the order details rule to the encoded body
// of a payment route, typed helpers and Client.Do alike.
func validateOrderDetailsBody(method, path string, body []byte) error {
	route, ok := orderDetailsRoutes[path]
	if !ok || method != "POST" {
		return nil
	}
	object := map[string]any{}
	if len(body) > 0 {
		value, err := decodeGeneric(body)
		if err != nil {
			return err
		}
		if object, ok = value.(map[string]any); !ok {
			return invalid(route.label + " body must be an object")
		}
	}
	return requireOrderDetails(object, route.label, route.options)
}

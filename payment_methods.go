package lunixi

import (
	"bytes"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Public payment method codes (paymentMethod). "QR" is accepted as an alias
// of PaymentMethodQRCode by ListPaymentMethods and NormalizePaymentMethod.
const (
	PaymentMethodCard           = "CARD"
	PaymentMethodBNPL           = "BNPL"
	PaymentMethodShoppingLoan   = "SHOPPING_LOAN"
	PaymentMethodPromissoryNote = "PROMISSORY_NOTE"
	PaymentMethodMealCard       = "MEAL_CARD"
	PaymentMethodGiftCard       = "GIFT_CARD"
	PaymentMethodCash           = "CASH"
	PaymentMethodCashOnDelivery = "CASH_ON_DELIVERY"
	PaymentMethodWallet         = "WALLET"
	PaymentMethodBankTransfer   = "BANK_TRANSFER"
	PaymentMethodOpenBanking    = "OPEN_BANKING"
	// PaymentMethodMobileBilling charges the buyer's mobile phone bill.
	PaymentMethodMobileBilling = "MOBILE_BILLING"
	PaymentMethodQRCode        = "QR_CODE"
	PaymentMethodCrypto        = "CRYPTO"
	PaymentMethodOther         = "OTHER"
)

// What the buyer must do when a payment response carries action (action.type).
const (
	// PaymentActionTypeOTP: send the one-time code with ConfirmPaymentAction.
	PaymentActionTypeOTP = "OTP"
	// PaymentActionTypeRedirect: send the buyer to action.redirect.url (for
	// cards, the bank's 3D Secure page).
	PaymentActionTypeRedirect = "REDIRECT"
	// PaymentActionTypeDisplay: show the buyer something to act on outside the
	// page, such as bank transfer details (Response.BankTransferInstruction) or
	// a code.
	PaymentActionTypeDisplay = "DISPLAY"
	// PaymentActionTypeAppApproval: the buyer approves in the provider's app;
	// the result arrives by webhook.
	PaymentActionTypeAppApproval = "APP_APPROVAL"
	// PaymentActionTypeInput: send the fields listed in action.input.fields
	// with ConfirmPaymentAction.
	PaymentActionTypeInput = "INPUT"
)

// providerSelection of a method in ListPaymentMethods.
const (
	// ProviderSelectionRouted: card routing picks the provider; providerCode
	// is not sent.
	ProviderSelectionRouted = "ROUTED"
	// ProviderSelectionOptional: providerCode may be omitted.
	ProviderSelectionOptional = "OPTIONAL"
	// ProviderSelectionRequired: more than one provider is connected and the
	// buyer picks the brand; providerCode must be sent.
	ProviderSelectionRequired = "REQUIRED"
)

// providerSelectionMode of a method in ListPaymentMethods: who picks the provider.
const (
	// ProviderSelectionModeBuyer: the buyer picks the brand. With more than one
	// provider connected providerCode is required (400 payment.provider.required).
	ProviderSelectionModeBuyer = "BUYER"
	// ProviderSelectionModePlatform: providerCode is optional; without it the
	// platform picks one of the connected providers before any money moves
	// (422 payment.provider.none_eligible when none can take the payment).
	ProviderSelectionModePlatform = "PLATFORM"
)

// ShoppingLoanMaxBasketItems is the most basket items a SHOPPING_LOAN payment
// can carry. The basket is required: the lender sets the loan term from the
// items' product categories.
const ShoppingLoanMaxBasketItems = 100

// Limits of a method block or of confirm fields, the same as the gateway.
const (
	methodFieldsMaxKeys       = 32
	methodFieldMaxValueLength = 1024
	methodFieldMaxGroupItems  = 10
	methodFieldMaxNestedKeys  = 16
)

const directMethodPaymentPath = "/api/v1/payments/direct/payment"

var (
	methodCodePattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)
	providerCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	fieldKeyPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`)
	confirmRoute        = regexp.MustCompile(`^/api/v1/payments/[^/]+/actions/[^/]+/confirm$`)
)

var directMethodPaymentFieldsWithoutBlocks = []string{
	"paymentMethod", "amount", "currency", "orderId", "paymentChannel", "paymentGroup", "callbackUrl", "locale",
	"description", "customerId", "preauth", "buyer", "shippingAddress", "billingAddress", "basketItems", "card",
	"installment", "installmentQuoteToken", "force3D", "metadata",
}

var methodBlockFields = []string{"mobileBilling", "wallet", "promissoryNote", "mealCard", "openBanking", "shoppingLoan", "bankTransfer"}

var cardOnlyFields = []string{"card", "installment", "installmentQuoteToken", "force3D"}

var paymentActionConfirmFields = []string{"otp", "consentAccepted", "fields"}

// MethodBlockFields returns the method blocks POST /payments/direct/payment
// accepts besides card. The block of the chosen method is required and a block
// for another method is rejected. The returned slice is a fresh copy.
func MethodBlockFields() []string { return slices.Clone(methodBlockFields) }

// DirectMethodPaymentFields returns every body field of
// POST /payments/direct/payment, method blocks included. The returned slice is
// a fresh copy.
func DirectMethodPaymentFields() []string {
	return slices.Concat(directMethodPaymentFieldsWithoutBlocks, methodBlockFields)
}

// CardOnlyFields returns the fields that belong to a CARD payment only. The
// returned slice is a fresh copy.
func CardOnlyFields() []string { return slices.Clone(cardOnlyFields) }

// PaymentActionConfirmFields returns the confirm body fields a merchant server
// sends. Checkout token fields (token, checkoutSessionProof) are never sent by
// this SDK. The returned slice is a fresh copy.
func PaymentActionConfirmFields() []string { return slices.Clone(paymentActionConfirmFields) }

// NormalizePaymentMethod checks a payment method code and returns its
// canonical form; "QR" becomes "QR_CODE".
func NormalizePaymentMethod(code string) (string, error) {
	if !methodCodePattern.MatchString(code) {
		return "", invalid("paymentMethod must be a payment method code such as MOBILE_BILLING")
	}
	if code == "QR" {
		return PaymentMethodQRCode, nil
	}
	return code, nil
}

// BlockNameOf returns the name of a method's block in the request body: the
// camelCase form of the method code (BANK_TRANSFER becomes bankTransfer). It
// is also returned as blockName by ListPaymentMethods.
func BlockNameOf(paymentMethod string) (string, error) {
	code, err := NormalizePaymentMethod(paymentMethod)
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(code)
	var name strings.Builder
	for i := 0; i < len(lower); i++ {
		if lower[i] == '_' && i+1 < len(lower) && (isLowerASCII(lower[i+1]) || isDigitASCII(lower[i+1])) {
			name.WriteByte(strings.ToUpper(lower[i+1 : i+2])[0])
			i++
			continue
		}
		name.WriteByte(lower[i])
	}
	return name.String(), nil
}

func isLowerASCII(b byte) bool { return b >= 'a' && b <= 'z' }
func isDigitASCII(b byte) bool { return b >= '0' && b <= '9' }

// decodeGeneric decodes JSON into maps, slices and json.Number values.
func decodeGeneric(raw []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil, invalid("JSON body")
	}
	return value, nil
}

// toGeneric converts Go values to the decoded JSON form validators inspect.
func toGeneric(value any) (any, error) {
	raw, err := CanonicalJSON(value)
	if err != nil {
		return nil, err
	}
	return decodeGeneric(raw)
}

func sortedKeys(object map[string]any) []string { return slices.Sorted(maps.Keys(object)) }

func present(object map[string]any, key string) bool {
	value, ok := object[key]
	return ok && value != nil
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func validProviderCode(value any) bool {
	text, ok := value.(string)
	return ok && providerCodePattern.MatchString(text)
}

// validateMethodFields checks the structure of a method block or of confirm
// fields: at most 32 fields named with a letter followed by letters and digits;
// each value a string (at most 1024 characters), a number, a boolean, an object
// of such values (ADDRESS, at most 16 fields) or a list of at most 10 objects
// (GROUP) whose values are scalars or objects. The fields themselves are the
// provider's declaration and are validated by the gateway.
func validateMethodFields(value any, path string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return invalid(path + " must be an object of field values")
	}
	if len(object) > methodFieldsMaxKeys {
		return invalid(path + " can carry at most 32 fields")
	}
	for _, key := range sortedKeys(object) {
		if key == "providerCode" {
			return invalid(path + " must not carry providerCode; pass it as its own argument")
		}
		if err := validateFieldKey(key, path); err != nil {
			return err
		}
		if err := validateFieldValue(object[key], path+"."+key, true); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldKey(key, path string) error {
	if !fieldKeyPattern.MatchString(key) || key == "__proto__" || key == "constructor" || key == "prototype" {
		return invalid(path + " has an invalid field name: field names start with a letter and contain only letters and digits")
	}
	return nil
}

func scalarFieldValue(value any) bool {
	switch v := value.(type) {
	case string:
		return utf16Length(v) <= methodFieldMaxValueLength
	case json.Number, bool:
		return true
	}
	return false
}

func validateFieldValue(value any, path string, allowList bool) error {
	if scalarFieldValue(value) {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) > methodFieldMaxNestedKeys {
			return invalid(path + " can carry at most 16 fields")
		}
		for _, key := range sortedKeys(v) {
			if err := validateFieldKey(key, path); err != nil {
				return err
			}
			if !scalarFieldValue(v[key]) {
				return invalid(path + "." + key + " must be a string (at most 1024 characters), a number or a boolean")
			}
		}
		return nil
	case []any:
		if !allowList {
			break
		}
		if len(v) > methodFieldMaxGroupItems {
			return invalid(path + " can carry at most 10 items")
		}
		for index, entry := range v {
			itemPath := path + "[" + strconv.Itoa(index) + "]"
			item, ok := entry.(map[string]any)
			if !ok {
				return invalid(itemPath + " must be an object")
			}
			if len(item) > methodFieldsMaxKeys {
				return invalid(itemPath + " can carry at most 32 fields")
			}
			for _, key := range sortedKeys(item) {
				if err := validateFieldKey(key, itemPath); err != nil {
					return err
				}
				if err := validateFieldValue(item[key], itemPath+"."+key, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return invalid(path + " must be a string (at most 1024 characters), a number, a boolean, an object or a list of objects")
}

// validateRawBlock checks a method block of a raw body: an optional
// providerCode with the provider code pattern, and the other fields' structure.
// skip names fields validated by their own rules (the promissory-note schedule).
func validateRawBlock(block map[string]any, path string, skip ...string) error {
	fields := make(map[string]any, len(block))
	for key, value := range block {
		if key == "providerCode" {
			if !validProviderCode(value) {
				return invalid(path + ".providerCode must be a provider code from ListPaymentMethods (letters, digits, _ and -)")
			}
			continue
		}
		if slices.Contains(skip, key) {
			continue
		}
		fields[key] = value
	}
	return validateMethodFields(fields, path)
}

// validateDirectMethodPayment applies the POST /payments/direct/payment rules
// to a raw body: known fields only, the required order fields, card fields only
// with CARD, exactly the chosen method's block, the block structure, the
// promissory-note plan, the shopping-loan basket, no providerCode for
// BANK_TRANSFER and boolean preauth/force3D.
func validateDirectMethodPayment(body []byte) error {
	value, err := decodeGeneric(body)
	if err != nil {
		return err
	}
	payload, ok := value.(map[string]any)
	if !ok {
		return invalid("direct method payment body must be an object")
	}
	allowed := DirectMethodPaymentFields()
	for _, key := range sortedKeys(payload) {
		if !slices.Contains(allowed, key) {
			return invalid("unknown direct method payment field: " + key)
		}
	}
	for _, field := range []string{"paymentMethod", "amount", "currency", "orderId"} {
		if !present(payload, field) || payload[field] == "" {
			return invalid("direct method payments require " + field)
		}
	}
	text, ok := payload["paymentMethod"].(string)
	if !ok {
		return invalid("paymentMethod must be a payment method code")
	}
	method, err := NormalizePaymentMethod(text)
	if err != nil {
		return err
	}
	own, _ := BlockNameOf(method)
	if method != PaymentMethodCard {
		for _, field := range cardOnlyFields {
			if present(payload, field) {
				return invalid(field + " can only be sent with paymentMethod CARD")
			}
		}
		if !slices.Contains(methodBlockFields, own) {
			return invalid("paymentMethod " + method + " cannot be paid through direct/payment yet; ListPaymentMethods lists the methods that can")
		}
	}
	for _, field := range methodBlockFields {
		if field != own && present(payload, field) {
			return invalid("paymentMethod " + method + " takes only the " + own + " block; " + field + " must not be sent")
		}
	}
	if !present(payload, own) {
		return invalid("paymentMethod " + method + " requires the " + own + " block")
	}
	block, ok := payload[own].(map[string]any)
	if !ok {
		return invalid(own + " must be an object")
	}
	if method != PaymentMethodCard {
		if _, has := block["providerCode"]; has && method == PaymentMethodBankTransfer {
			return invalid("paymentMethod BANK_TRANSFER takes no providerCode in the bankTransfer block")
		}
		var skip []string
		if method == PaymentMethodPromissoryNote {
			// The schedule has its own rules (up to 24 rows).
			skip = []string{"schedule"}
		}
		if err = validateRawBlock(block, own, skip...); err != nil {
			return err
		}
	}
	if method == PaymentMethodPromissoryNote {
		amount, _ := payload["amount"].(string)
		if err = validatePromissoryNotePlan(block, amount, own); err != nil {
			return err
		}
	}
	if method == PaymentMethodShoppingLoan {
		items, _ := payload["basketItems"].([]any)
		if err = validateShoppingLoanBasketSize(len(items)); err != nil {
			return err
		}
	}
	for _, field := range []string{"preauth", "force3D"} {
		if raw, has := payload[field]; has {
			if _, isBool := raw.(bool); !isBool {
				return invalid(field + " must be a boolean")
			}
		}
	}
	return nil
}

func validateShoppingLoanBasketSize(count int) error {
	if count == 0 {
		return invalid("paymentMethod SHOPPING_LOAN requires basketItems: the lender sets the loan term from the basket")
	}
	if count > ShoppingLoanMaxBasketItems {
		return invalid("paymentMethod SHOPPING_LOAN takes at most 100 basket items")
	}
	return nil
}

// validateConfirmBody applies the confirm rules to a raw body: only otp
// (digits, 1–16), consentAccepted (boolean) and fields (method block
// structure), at least one of them.
func validateConfirmBody(body []byte) error {
	if len(body) == 0 {
		return invalid("ConfirmPaymentAction needs otp, consentAccepted or fields")
	}
	value, err := decodeGeneric(body)
	if err != nil {
		return err
	}
	input, ok := value.(map[string]any)
	if !ok {
		return invalid("confirm body must be an object: otp, consentAccepted, fields")
	}
	for _, key := range sortedKeys(input) {
		if !slices.Contains(paymentActionConfirmFields, key) {
			return invalid("unknown confirm field: " + key + "; a merchant server sends only otp, consentAccepted and fields")
		}
	}
	sent := false
	if present(input, "otp") {
		otp, ok := input["otp"].(string)
		if !ok || !otpPattern.MatchString(otp) {
			return invalid("otp must be the code the buyer entered, as a string of digits")
		}
		sent = true
	}
	if present(input, "consentAccepted") {
		if _, ok := input["consentAccepted"].(bool); !ok {
			return invalid("consentAccepted must be a boolean")
		}
		sent = true
	}
	if present(input, "fields") {
		if err = validateMethodFields(input["fields"], "fields"); err != nil {
			return err
		}
		sent = true
	}
	if !sent {
		return invalid("ConfirmPaymentAction needs otp, consentAccepted or fields")
	}
	return nil
}

var otpPattern = regexp.MustCompile(`^[0-9]{1,16}$`)

package lunixi

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// DirectMethodPaymentRequest builds the body of POST /payments/direct/payment:
// a payment with any method started from the merchant server. Create it with
// NewDirectMethodPaymentRequest, choose the method with WithMethodFields or
// WithCard and add options with the other With methods. Each With method
// returns the request for chaining; the first invalid value is kept and
// reported by Body and DirectMethodPayment, before any request is sent.
type DirectMethodPaymentRequest struct {
	amount, currency, orderID string
	method                    string
	block                     map[string]any
	plan                      map[string]any
	installment               *int
	installmentQuoteToken     string
	force3D, preauth          *bool
	callbackURL, locale       string
	description, customerID   string
	paymentChannel            string
	paymentGroup              string
	buyer                     *Buyer
	billingAddress            *Address
	shippingAddress           *Address
	basketItems               []BasketItem
	metadata                  map[string]any
	err                       error
}

// NewDirectMethodPaymentRequest starts a direct method payment body. amount is
// positive minor-unit text (the basket total without a provider fee), currency
// a three-letter code and orderID your order reference.
func NewDirectMethodPaymentRequest(amount, currency, orderID string) *DirectMethodPaymentRequest {
	r := &DirectMethodPaymentRequest{amount: amount, currency: strings.ToUpper(currency), orderID: orderID}
	switch {
	case !positiveMoney(amount):
		r.fail(invalid("amount must be positive integer minor units within JSON safe range"))
	case !currencyPattern.MatchString(r.currency):
		r.fail(invalid("currency must be a three-letter code"))
	case strings.TrimSpace(orderID) == "" || len(orderID) > 200:
		r.fail(invalid("orderId"))
	}
	return r
}

func (r *DirectMethodPaymentRequest) fail(err error) *DirectMethodPaymentRequest {
	if r.err == nil {
		r.err = err
	}
	return r
}

// WithMethodFields pays with a method other than card. fields are the values
// of the provider's inputFields from ListPaymentMethods (for example
// {"gsmNumber": "+905321234567"} for mobile billing, {"bankCode": "0062"} for
// BANK_TRANSFER); nil or an empty map when the provider asks for nothing.
// providerCode is "" to leave it out; it is required when the method lists more
// than one provider (providerSelection REQUIRED) and is always read from
// ListPaymentMethods. BANK_TRANSFER takes no providerCode. For PROMISSORY_NOTE
// give the plan with WithPromissoryNoteSchedule or WithPromissoryNoteCount.
func (r *DirectMethodPaymentRequest) WithMethodFields(paymentMethod, providerCode string, fields map[string]any) *DirectMethodPaymentRequest {
	code, err := NormalizePaymentMethod(paymentMethod)
	if err != nil {
		return r.fail(err)
	}
	if code == PaymentMethodCard {
		return r.fail(invalid("use WithCard to pay by card"))
	}
	blockName, _ := BlockNameOf(code)
	if !slices.Contains(methodBlockFields, blockName) {
		return r.fail(invalid("paymentMethod " + code + " cannot be paid through direct/payment yet; ListPaymentMethods lists the methods that can"))
	}
	if fields == nil {
		fields = map[string]any{}
	}
	value, err := toGeneric(fields)
	if err != nil {
		return r.fail(err)
	}
	if err = validateMethodFields(value, blockName); err != nil {
		return r.fail(err)
	}
	block := value.(map[string]any)
	if code == PaymentMethodPromissoryNote {
		if _, has := block["schedule"]; has {
			return r.fail(invalid("pass the promissory-note schedule with WithPromissoryNoteSchedule, not in the fields"))
		}
		if raw, has := block["noteCount"]; has {
			text, err := noteCountText(raw, "promissoryNote.noteCount")
			if err != nil {
				return r.fail(err)
			}
			block["noteCount"] = text
		}
	}
	if providerCode != "" {
		if code == PaymentMethodBankTransfer {
			return r.fail(invalid("paymentMethod BANK_TRANSFER takes no providerCode: pass \"\" and the buyer's bank as {\"bankCode\": ...}"))
		}
		if !providerCodePattern.MatchString(providerCode) {
			return r.fail(invalid("providerCode must be a provider code from ListPaymentMethods (letters, digits, _ and -)"))
		}
		block["providerCode"] = providerCode
	}
	r.method, r.block = code, block
	return r
}

// WithPromissoryNoteSchedule sets the promissory-note plan as one note per row.
// Amounts are positive minor-unit text adding up to the payment amount; due
// dates are YYYY-MM-DD in ascending order, at most 24 rows. That a due date is
// in the future is checked by the gateway. It replaces a note count.
func (r *DirectMethodPaymentRequest) WithPromissoryNoteSchedule(rows []PromissoryNoteScheduleRow) *DirectMethodPaymentRequest {
	if err := validateScheduleRows(rows, "promissoryNote.schedule"); err != nil {
		return r.fail(err)
	}
	schedule := make([]any, len(rows))
	for i, row := range rows {
		schedule[i] = map[string]any{"amount": row.Amount, "dueDate": row.DueDate}
	}
	r.plan = map[string]any{"schedule": schedule}
	return r
}

// WithPromissoryNoteCount sets the promissory-note plan as a number of notes
// (1–24, within your provider connection limits); it is sent as text. The
// amount is split evenly and due dates follow your connection settings. It
// replaces a schedule.
func (r *DirectMethodPaymentRequest) WithPromissoryNoteCount(count int) *DirectMethodPaymentRequest {
	text, err := noteCountText(count, "promissoryNote.noteCount")
	if err != nil {
		return r.fail(err)
	}
	r.plan = map[string]any{"noteCount": text}
	return r
}

// WithCard pays by card: new card fields, or cardUserKey and cardToken of a
// stored card. Add WithForce3D(true) for 3D Secure.
func (r *DirectMethodPaymentRequest) WithCard(card map[string]any) *DirectMethodPaymentRequest {
	if len(card) == 0 {
		return r.fail(invalid("WithCard requires card fields"))
	}
	r.method, r.block = PaymentMethodCard, maps.Clone(card)
	return r
}

// WithForce3D is card only; true charges with 3D Secure and the response
// carries a REDIRECT action.
func (r *DirectMethodPaymentRequest) WithForce3D(value bool) *DirectMethodPaymentRequest {
	r.force3D = &value
	return r
}

// WithInstallment is card only. quoteToken ("" to leave it out) binds the
// charge to an installment quote; the quoted fee is added to amount.
func (r *DirectMethodPaymentRequest) WithInstallment(count int, quoteToken string) *DirectMethodPaymentRequest {
	if count < 1 {
		return r.fail(invalid("installment must be a positive integer"))
	}
	r.installment = &count
	r.installmentQuoteToken = quoteToken
	return r
}

// WithPreauth true holds the amount (AUTHORIZED); take it with Capture or
// release it with Void. A method that does not support it is refused with 422
// payment.preauth.unsupported_for_method.
func (r *DirectMethodPaymentRequest) WithPreauth(value bool) *DirectMethodPaymentRequest {
	r.preauth = &value
	return r
}

// WithCallbackURL is where the buyer's browser returns after a REDIRECT action.
func (r *DirectMethodPaymentRequest) WithCallbackURL(value string) *DirectMethodPaymentRequest {
	if strings.TrimSpace(value) == "" {
		return r.fail(invalid("callbackUrl"))
	}
	r.callbackURL = value
	return r
}

// WithLocale sets the locale of buyer-facing texts.
func (r *DirectMethodPaymentRequest) WithLocale(value string) *DirectMethodPaymentRequest {
	if strings.TrimSpace(value) == "" {
		return r.fail(invalid("locale"))
	}
	r.locale = value
	return r
}

// WithDescription sets the payment description.
func (r *DirectMethodPaymentRequest) WithDescription(value string) *DirectMethodPaymentRequest {
	r.description = value
	return r
}

// WithCustomerID sets your customer reference.
func (r *DirectMethodPaymentRequest) WithCustomerID(value string) *DirectMethodPaymentRequest {
	r.customerID = value
	return r
}

// WithPaymentChannel sets paymentChannel.
func (r *DirectMethodPaymentRequest) WithPaymentChannel(value string) *DirectMethodPaymentRequest {
	r.paymentChannel = value
	return r
}

// WithPaymentGroup sets paymentGroup.
func (r *DirectMethodPaymentRequest) WithPaymentGroup(value string) *DirectMethodPaymentRequest {
	r.paymentGroup = value
	return r
}

// WithBuyer sets the buyer; values are sent to Lunixi only.
func (r *DirectMethodPaymentRequest) WithBuyer(value Buyer) *DirectMethodPaymentRequest {
	r.buyer = &value
	return r
}

// WithBillingAddress sets the billing address.
func (r *DirectMethodPaymentRequest) WithBillingAddress(value Address) *DirectMethodPaymentRequest {
	r.billingAddress = &value
	return r
}

// WithShippingAddress sets the shipping address.
func (r *DirectMethodPaymentRequest) WithShippingAddress(value Address) *DirectMethodPaymentRequest {
	r.shippingAddress = &value
	return r
}

// WithBasketItems sets the basket. SHOPPING_LOAN requires 1 to 100 items.
func (r *DirectMethodPaymentRequest) WithBasketItems(items []BasketItem) *DirectMethodPaymentRequest {
	r.basketItems = slices.Clone(items)
	return r
}

// WithMetadata sets merchant metadata.
func (r *DirectMethodPaymentRequest) WithMetadata(value map[string]any) *DirectMethodPaymentRequest {
	r.metadata = maps.Clone(value)
	return r
}

// Body returns the request body, or the first invalid value. The raw body
// rules of DirectMethodPayment and Client.Do are applied again when sending.
func (r *DirectMethodPaymentRequest) Body() (map[string]any, error) {
	if r == nil {
		return nil, invalid("direct method payment request")
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.method == "" {
		return nil, invalid("choose the payment method with WithMethodFields or WithCard")
	}
	if r.method != PaymentMethodCard && (r.installment != nil || r.installmentQuoteToken != "" || r.force3D != nil) {
		return nil, invalid("installment, installmentQuoteToken and force3D can only be sent with a card payment (WithCard)")
	}
	block := maps.Clone(r.block)
	if r.method == PaymentMethodPromissoryNote {
		if r.plan != nil {
			if _, has := block["noteCount"]; has {
				return nil, invalid("the promissory-note plan is given twice: noteCount in the fields and WithPromissoryNoteSchedule/WithPromissoryNoteCount; use one")
			}
			maps.Copy(block, r.plan)
		}
		if err := validatePromissoryNotePlan(block, r.amount, "promissoryNote"); err != nil {
			return nil, err
		}
	} else if r.plan != nil {
		return nil, invalid("a promissory-note plan can only be sent with paymentMethod PROMISSORY_NOTE")
	}
	if r.method == PaymentMethodShoppingLoan {
		if err := validateShoppingLoanBasketSize(len(r.basketItems)); err != nil {
			return nil, err
		}
	}
	blockName, _ := BlockNameOf(r.method)
	body := map[string]any{"paymentMethod": r.method, "amount": r.amount, "currency": r.currency, "orderId": r.orderID, blockName: block}
	set := func(key, value string) {
		if value != "" {
			body[key] = value
		}
	}
	set("installmentQuoteToken", r.installmentQuoteToken)
	set("callbackUrl", r.callbackURL)
	set("locale", r.locale)
	set("description", r.description)
	set("customerId", r.customerID)
	set("paymentChannel", r.paymentChannel)
	set("paymentGroup", r.paymentGroup)
	if r.installment != nil {
		body["installment"] = *r.installment
	}
	if r.force3D != nil {
		body["force3D"] = *r.force3D
	}
	if r.preauth != nil {
		body["preauth"] = *r.preauth
	}
	if r.buyer != nil {
		body["buyer"] = *r.buyer
	}
	if r.billingAddress != nil {
		body["billingAddress"] = *r.billingAddress
	}
	if r.shippingAddress != nil {
		body["shippingAddress"] = *r.shippingAddress
	}
	if r.basketItems != nil {
		body["basketItems"] = slices.Clone(r.basketItems)
	}
	if r.metadata != nil {
		body["metadata"] = maps.Clone(r.metadata)
	}
	return body, nil
}

// DirectMethodPayment starts a payment with any method from the merchant
// server (POST /payments/direct/payment). key is required: persist it per
// payment attempt and reuse it when retrying the same payment. This is also the
// only server-side card charge (WithCard, plus WithForce3D(true) for 3D Secure).
//
// The gateway envelope comes back as it is: Data holds paymentId, orderId,
// status, paymentMethod, providerCode and, when the buyer must act, action
// (HTTP 202; see PaymentActionType*). A declined payment is an APIError. With
// providerSelectionMode PLATFORM and no providerCode the platform picks the
// provider; 422 payment.provider.none_eligible means nothing was charged.
func (p *PaymentClient) DirectMethodPayment(ctx context.Context, request *DirectMethodPaymentRequest, key string) (*Response, error) {
	body, err := request.Body()
	if err != nil {
		return nil, err
	}
	if err = requireKey(key); err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: directMethodPaymentPath, Body: body, IdempotencyKey: key})
}

// ListPaymentMethods reads the payment methods open for the account
// (GET /payments/methods): per method its blockName, providerSelection,
// providerSelectionMode, preauth support, limits, split behaviour and the
// providers with the inputFields to send in the method block. paymentMethod ""
// lists every method; a code lists that method only ("QR" is sent as QR_CODE)
// and gives an empty list when it is not open for the account.
func (p *PaymentClient) ListPaymentMethods(ctx context.Context, paymentMethod string) (*Response, error) {
	if paymentMethod == "" {
		return p.Methods(ctx)
	}
	code, err := NormalizePaymentMethod(paymentMethod)
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/methods", Query: url.Values{"paymentMethod": {code}}})
}

// PaymentActionConfirmRequest is the body of ConfirmPaymentAction: the
// buyer's one-time code (digits), the agreement consent when
// action.consent.required is true, or the values an INPUT action asks for. At
// least one is required. Checkout token fields are never sent.
type PaymentActionConfirmRequest struct {
	OTP             string         `json:"otp,omitempty"`
	ConsentAccepted *bool          `json:"consentAccepted,omitempty"`
	Fields          map[string]any `json:"fields,omitempty"`
}

func actionPath(paymentID, actionID, operation string) (string, error) {
	payment, err := idPath(paymentID)
	if err != nil {
		return "", invalid("paymentId is required (data.paymentId of the payment response)")
	}
	action, err := idPath(actionID)
	if err != nil {
		return "", invalid("actionId is required (data.action.id of the payment response)")
	}
	return "/api/v1/payments/" + payment + "/actions/" + action + "/" + operation, nil
}

// ConfirmPaymentAction completes the open action of a payment
// (POST /payments/{paymentId}/actions/{actionId}/confirm). It does not start a
// new charge; repeating the same confirm returns the recorded result. A
// rejected code is a 422 with details.attemptsRemaining and the action stays
// open.
func (p *PaymentClient) ConfirmPaymentAction(ctx context.Context, paymentID, actionID string, input PaymentActionConfirmRequest) (*Response, error) {
	path, err := actionPath(paymentID, actionID, "confirm")
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: path, Body: input})
}

// ResendPaymentAction sends a new one-time code
// (POST /payments/{paymentId}/actions/{actionId}/resend, no body). The previous
// action is closed; confirm with the new data.action.id.
func (p *PaymentClient) ResendPaymentAction(ctx context.Context, paymentID, actionID string) (*Response, error) {
	path, err := actionPath(paymentID, actionID, "resend")
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: path})
}

// PaymentActionBankTransfer is the transfer instruction of a BANK_TRANSFER
// payment's DISPLAY action: the merchant's own bank account the buyer sends
// the money to.
type PaymentActionBankTransfer struct {
	// BankCode is the 4-digit bank code of the merchant's bank.
	BankCode string `json:"bankCode"`
	BankName string `json:"bankName"`
	// HolderName is the account holder; the buyer enters it as the recipient.
	HolderName string `json:"holderName"`
	// IBAN without spaces.
	IBAN string `json:"iban"`
	// AmountMinor is the amount to send, in minor units.
	AmountMinor string `json:"amountMinor"`
	Currency    string `json:"currency"`
	// PaymentCode is written by the buyer in the transfer description; it
	// matches the transfer to the payment.
	PaymentCode string `json:"paymentCode"`
	ExpiresAt   string `json:"expiresAt"`
	// OtherBank is true when the buyer chose other banks
	// (BankTransferOtherBanksCode); the instruction is for EFT/FAST.
	OtherBank bool `json:"otherBank"`
}

// BankTransferInstruction reads data.action.bankTransfer of a payment
// response. It returns nil when there is no action, the action type is not
// DISPLAY or the instruction is missing. The instruction is display data only;
// the payment's paid state still comes from the gateway result.
func (r *Response) BankTransferInstruction() *PaymentActionBankTransfer {
	if r == nil || len(r.Data) == 0 {
		return nil
	}
	var data struct {
		Action *struct {
			Type         string                     `json:"type"`
			BankTransfer *PaymentActionBankTransfer `json:"bankTransfer"`
		} `json:"action"`
	}
	if json.Unmarshal(r.Data, &data) != nil || data.Action == nil || data.Action.Type != PaymentActionTypeDisplay {
		return nil
	}
	return data.Action.BankTransfer
}

package lunixi

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// PaymentLinkRequest is the body of PaymentLinks.Create
// (POST /payments/links). Fields follow the gateway's order; empty values
// are left out. Amounts are minor-unit text: "150000" is 1,500.00 TRY. Usage,
// AmountMode and Currency are fixed at creation. The SDK checks types and
// value sets; business rules (title length, amount ranges, which fields a mode
// allows) are enforced by the gateway, which answers 422
// payment_link.validation.invalid with details.errors.
type PaymentLinkRequest struct {
	// Environment is optional: the signing key decides it, and an explicit
	// value must match the key (403 payment_link.environment.mismatch).
	Environment string `json:"environment,omitempty"`
	// Usage is PaymentLinkUsageSingleUse or PaymentLinkUsageMultiUse.
	Usage string `json:"usage"`
	// AmountMode is PaymentLinkAmountModeFixed, Open or Itemized.
	AmountMode string `json:"amountMode"`
	Currency   string `json:"currency"`
	// AmountMinor is the amount of a FIXED link.
	AmountMinor string `json:"amountMinor,omitempty"`
	// MinAmountMinor (required) and MaxAmountMinor bound an OPEN link.
	MinAmountMinor string `json:"minAmountMinor,omitempty"`
	MaxAmountMinor string `json:"maxAmountMinor,omitempty"`
	// SuggestedAmountsMinor offers up to six ascending amounts on an OPEN link.
	SuggestedAmountsMinor []string `json:"suggestedAmountsMinor,omitempty"`
	// QuantityEnabled lets the buyer choose a quantity (FIXED + MULTI_USE
	// only) between MinQuantity and MaxQuantity (1–999).
	QuantityEnabled bool `json:"quantityEnabled,omitempty"`
	MinQuantity     *int `json:"minQuantity,omitempty"`
	MaxQuantity     *int `json:"maxQuantity,omitempty"`
	// Items are the lines of an ITEMIZED link (1–50).
	Items []PaymentLinkItem `json:"items,omitempty"`
	// Capacity is the total units the link can sell (MULTI_USE only); nil is
	// unlimited.
	Capacity *int   `json:"capacity,omitempty"`
	Title    string `json:"title"`
	// Description is plain text (no HTML or Markdown).
	Description string `json:"description,omitempty"`
	// ImageAssetID is a published image from UploadImage or ConfirmImageUpload.
	ImageAssetID   string `json:"imageAssetId,omitempty"`
	SuccessMessage string `json:"successMessage,omitempty"`
	// SuccessRedirectURL must use https://.
	SuccessRedirectURL string `json:"successRedirectUrl,omitempty"`
	TaxNote            string `json:"taxNote,omitempty"`
	// Locale is the payment page language, PaymentLinkLocaleTR or EN.
	Locale string `json:"locale,omitempty"`
	// StartsAt, ExpiresAt and EventAt are RFC 3339 times; EventAt is display
	// only (for example the date of an event ticket).
	StartsAt  string `json:"startsAt,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	EventAt   string `json:"eventAt,omitempty"`
	// ButtonLabelKey is a PaymentLinkButtonLabel* value; empty uses your
	// organisation's default.
	ButtonLabelKey         string                   `json:"buttonLabelKey,omitempty"`
	RequireTermsAcceptance bool                     `json:"requireTermsAcceptance,omitempty"`
	BuyerFields            *PaymentLinkBuyerFields  `json:"buyerFields,omitempty"`
	CustomFields           []PaymentLinkCustomField `json:"customFields,omitempty"`
	// Recipient is who the link is for; stored encrypted.
	Recipient *PaymentLinkRecipient `json:"recipient,omitempty"`
	// Prefill holds buyer details filled in on the server side; never sent to
	// the browser.
	Prefill *PaymentLinkPrefill `json:"prefill,omitempty"`
	// Verification asks the buyer to confirm a one-time code sent to the
	// recipient e-mail before paying.
	Verification *PaymentLinkVerification `json:"verification,omitempty"`
	// Reminders are reminder e-mails of a SINGLE_USE link with a recipient.
	Reminders *PaymentLinkReminders `json:"reminders,omitempty"`
	// Checkout narrows the checkout of the link.
	Checkout             *PaymentLinkCheckout `json:"checkout,omitempty"`
	Reference            string               `json:"reference,omitempty"`
	Tags                 []string             `json:"tags,omitempty"`
	BranchCode           string               `json:"branchCode,omitempty"`
	SalesChannel         string               `json:"salesChannel,omitempty"`
	Campaign             string               `json:"campaign,omitempty"`
	AgentCode            string               `json:"agentCode,omitempty"`
	MerchantNotifyEmails []string             `json:"merchantNotifyEmails,omitempty"`
	// Metadata is up to 20 string pairs returned to you in webhooks.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PaymentLinkItem is one line of an ITEMIZED link. ItemKey is the line's
// permanent identity (stock counters hang off it); empty lets the gateway
// assign one. A MinQuantity of 0 makes the line optional.
type PaymentLinkItem struct {
	Name           string `json:"name"`
	UnitPriceMinor string `json:"unitPriceMinor"`
	ItemKey        string `json:"itemKey,omitempty"`
	Description    string `json:"description,omitempty"`
	MinQuantity    *int   `json:"minQuantity,omitempty"`
	MaxQuantity    *int   `json:"maxQuantity,omitempty"`
	// Capacity is the units available for this line across all payments;
	// nil is unlimited.
	Capacity     *int   `json:"capacity,omitempty"`
	ImageAssetID string `json:"imageAssetId,omitempty"`
	// ProductCategory is a ProductCategory* code; not checked locally.
	ProductCategory string `json:"productCategory,omitempty"`
}

// PaymentLinkCustomField is a typed question the buyer answers on the
// payment page. Options are the choices of a SELECT field.
type PaymentLinkCustomField struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	FieldType string   `json:"fieldType"`
	Required  bool     `json:"required,omitempty"`
	Options   []string `json:"options,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"`
	HelpText  string   `json:"helpText,omitempty"`
}

// PaymentLinkBuyerFields sets each buyer field to
// BuyerFieldRequirementRequired; empty leaves it out, which also means
// REQUIRED. The link page always asks all five fields.
type PaymentLinkBuyerFields struct {
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	Address        string `json:"address,omitempty"`
	IdentityNumber string `json:"identityNumber,omitempty"`
}

// PaymentLinkRecipient is who the link is for (Phone in E.164).
type PaymentLinkRecipient struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// PaymentLinkPrefill holds buyer details filled in on the server side
// (Country is ISO 3166-1 alpha-2).
type PaymentLinkPrefill struct {
	Name           string `json:"name,omitempty"`
	Surname        string `json:"surname,omitempty"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	IdentityNumber string `json:"identityNumber,omitempty"`
	Address        string `json:"address,omitempty"`
	City           string `json:"city,omitempty"`
	Country        string `json:"country,omitempty"`
	ZipCode        string `json:"zipCode,omitempty"`
}

// PaymentLinkVerification requires a one-time code before paying. Channel
// empty means PaymentLinkDeliveryChannelEmail, the only available channel.
type PaymentLinkVerification struct {
	Required bool   `json:"required"`
	Channel  string `json:"channel"`
}

// PaymentLinkReminders sends reminder e-mails every 24–168 hours, at most
// 1–3 times.
type PaymentLinkReminders struct {
	Enabled       bool `json:"enabled"`
	IntervalHours int  `json:"intervalHours"`
	MaxCount      int  `json:"maxCount"`
}

// PaymentLinkCheckout narrows the checkout: RequireThreeDs can only tighten,
// PaymentMethods is a subset of your form settings (empty = your form
// settings) and EnabledInstallments follows IntentRequest.EnabledInstallments
// (nil or empty = no restriction).
type PaymentLinkCheckout struct {
	RequireThreeDs      bool     `json:"requireThreeDs"`
	PaymentMethods      []string `json:"paymentMethods"`
	EnabledInstallments []int    `json:"enabledInstallments,omitempty"`
}

// PaymentLinkSendRequest is the body of PaymentLinks.Send. Without
// RecipientEmail and RecipientPhone the link's recipient is used.
type PaymentLinkSendRequest struct {
	Channel        string `json:"channel"`
	RecipientEmail string `json:"recipientEmail,omitempty"`
	RecipientPhone string `json:"recipientPhone,omitempty"`
	Locale         string `json:"locale,omitempty"`
}

// PaymentLinkActionOptions are the optional fields of Pause, Resume and
// Deactivate. ExpectedRowVersion nil skips the version check; a value fails
// with 409 payment_link.link.version_conflict if the link changed meanwhile.
type PaymentLinkActionOptions struct {
	ExpectedRowVersion *int64 `json:"expectedRowVersion,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

func (r PaymentLinkRequest) body() (PaymentLinkRequest, error) {
	if r.Environment != "" {
		if e := oneOf(r.Environment, paymentLinkEnvironments, "environment"); e != nil {
			return r, e
		}
	}
	if e := oneOf(r.Usage, paymentLinkUsages, "usage"); e != nil {
		return r, e
	}
	if e := oneOf(r.AmountMode, paymentLinkAmountModes, "amountMode"); e != nil {
		return r, e
	}
	r.Currency = strings.ToUpper(r.Currency)
	if !currencyPattern.MatchString(r.Currency) {
		return r, invalid("currency must be a three-letter code")
	}
	if strings.TrimSpace(r.Title) == "" {
		return r, invalid("title is required")
	}
	for name, amount := range map[string]string{"amountMinor": r.AmountMinor, "minAmountMinor": r.MinAmountMinor, "maxAmountMinor": r.MaxAmountMinor} {
		if amount != "" && !validMinor(amount, false, false) {
			return r, invalid(name + " must be minor-unit text")
		}
	}
	for _, amount := range r.SuggestedAmountsMinor {
		if !validMinor(amount, false, false) {
			return r, invalid("suggestedAmountsMinor must be minor-unit text")
		}
	}
	for _, item := range r.Items {
		if e := item.validate(); e != nil {
			return r, e
		}
	}
	if e := optionalAssetID(r.ImageAssetID, "imageAssetId"); e != nil {
		return r, e
	}
	if r.Locale != "" {
		if e := oneOf(r.Locale, paymentLinkLocales, "locale"); e != nil {
			return r, e
		}
	}
	if r.ButtonLabelKey != "" {
		if e := oneOf(r.ButtonLabelKey, paymentLinkButtonLabels, "buttonLabelKey"); e != nil {
			return r, e
		}
	}
	if r.BuyerFields != nil {
		for _, requirement := range []string{r.BuyerFields.Name, r.BuyerFields.Email, r.BuyerFields.Phone, r.BuyerFields.Address, r.BuyerFields.IdentityNumber} {
			if requirement != "" {
				if e := oneOf(requirement, buyerFieldRequirements, "buyerFields value"); e != nil {
					return r, e
				}
			}
		}
	}
	for _, field := range r.CustomFields {
		if strings.TrimSpace(field.Key) == "" || strings.TrimSpace(field.Label) == "" {
			return r, invalid("custom field key and label are required")
		}
		if e := oneOf(field.FieldType, customFieldTypes, "custom field fieldType"); e != nil {
			return r, e
		}
	}
	if r.Verification != nil {
		verification := *r.Verification
		if verification.Channel == "" {
			verification.Channel = PaymentLinkDeliveryChannelEmail
		}
		if e := oneOf(verification.Channel, paymentLinkChannels, "verification channel"); e != nil {
			return r, e
		}
		r.Verification = &verification
	}
	if r.Checkout != nil {
		checkout, e := r.Checkout.normalized()
		if e != nil {
			return r, e
		}
		r.Checkout = &checkout
	}
	return r, nil
}

func (item PaymentLinkItem) validate() error {
	if strings.TrimSpace(item.Name) == "" {
		return invalid("payment link item name is required")
	}
	if !validMinor(item.UnitPriceMinor, false, false) {
		return invalid("payment link item unitPriceMinor must be minor-unit text")
	}
	return optionalAssetID(item.ImageAssetID, "items[].imageAssetId")
}

func (c PaymentLinkCheckout) normalized() (PaymentLinkCheckout, error) {
	if c.PaymentMethods == nil {
		c.PaymentMethods = []string{}
	}
	if len(c.EnabledInstallments) == 0 {
		c.EnabledInstallments = nil
		return c, nil
	}
	if e := validateEnabledInstallments(c.EnabledInstallments); e != nil {
		return c, e
	}
	c.EnabledInstallments = slices.Sorted(slices.Values(c.EnabledInstallments))
	return c, nil
}

func optionalAssetID(value, field string) error {
	if value != "" && !uuidPattern.MatchString(value) {
		return invalid(field + " must be an image asset id (uuid)")
	}
	return nil
}

var paymentLinkTimeFields = []string{"startsAt", "expiresAt", "eventAt"}

// paymentLinkUpdateBody builds expectedRowVersion followed by the changed
// fields in the gateway's field order. A time.Time value is sent as RFC 3339
// UTC; nil clears a field.
func paymentLinkUpdateBody(expectedRowVersion int64, changes map[string]any) (orderedObject, error) {
	if expectedRowVersion < 0 || expectedRowVersion > MaxSafeInteger {
		return nil, invalid("expectedRowVersion must be the link's current rowVersion (a non-negative integer)")
	}
	if len(changes) == 0 {
		return nil, invalid("payment link update needs at least one changed field")
	}
	for key := range changes {
		if slices.Contains(paymentLinkImmutableFields, key) {
			return nil, invalid(key + " is fixed when a payment link is created; create a new link instead")
		}
		if !slices.Contains(paymentLinkSpecFields, key) {
			return nil, invalid("unknown payment link update field: " + key)
		}
	}
	body := orderedObject{{key: "expectedRowVersion", value: expectedRowVersion}}
	for _, key := range paymentLinkSpecFields {
		value, present := changes[key]
		if !present {
			continue
		}
		if slices.Contains(paymentLinkTimeFields, key) {
			switch v := value.(type) {
			case nil:
			case time.Time:
				value = v.UTC().Format(time.RFC3339Nano)
			case string:
				if strings.TrimSpace(v) == "" {
					return nil, invalid(key + " must be an RFC 3339 time or nil")
				}
			default:
				return nil, invalid(key + " must be an RFC 3339 time or nil")
			}
		}
		body = append(body, member{key: key, value: value})
	}
	return body, nil
}

// orderedObject is a JSON object whose keys keep the given order.
type orderedObject []member

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(m.key)
		out.Write(key)
		out.WriteByte(':')
		value, err := json.Marshal(m.value)
		if err != nil {
			return nil, err
		}
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func (r PaymentLinkSendRequest) validate() error {
	if e := oneOf(r.Channel, paymentLinkChannels, "send channel"); e != nil {
		return e
	}
	if r.Locale != "" {
		return oneOf(r.Locale, paymentLinkLocales, "send locale")
	}
	return nil
}

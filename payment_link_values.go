package lunixi

import (
	"slices"
	"strings"
)

// Closed value sets of the payment-link API (/api/v1/payments/links). The
// gateway answers any other value with 400.

// Payment link usage, fixed at creation.
const (
	// PaymentLinkUsageSingleUse can be paid once (an invoice or a personal
	// payment request).
	PaymentLinkUsageSingleUse = "SINGLE_USE"
	// PaymentLinkUsageMultiUse can be paid many times (donations, tickets, a
	// product page).
	PaymentLinkUsageMultiUse = "MULTI_USE"
)

// Payment link amount modes, fixed at creation.
const (
	// PaymentLinkAmountModeFixed: the merchant sets amountMinor.
	PaymentLinkAmountModeFixed = "FIXED"
	// PaymentLinkAmountModeOpen: the buyer enters the amount within
	// minAmountMinor and maxAmountMinor.
	PaymentLinkAmountModeOpen = "OPEN"
	// PaymentLinkAmountModeItemized: the buyer picks quantities of items.
	PaymentLinkAmountModeItemized = "ITEMIZED"
)

// Payment link states.
const (
	PaymentLinkStateActive      = "ACTIVE"
	PaymentLinkStatePaused      = "PAUSED"
	PaymentLinkStateCompleted   = "COMPLETED"
	PaymentLinkStateExpired     = "EXPIRED"
	PaymentLinkStateDeactivated = "DEACTIVATED"
	// PaymentLinkStateTakenDown: removed by Lunixi; every write on the link
	// answers 409 payment_link.link.taken_down.
	PaymentLinkStateTakenDown = "TAKEN_DOWN"
)

// Payment link attempt states.
const (
	PaymentLinkAttemptStateOpen        = "OPEN"
	PaymentLinkAttemptStateBound       = "BOUND"
	PaymentLinkAttemptStateSucceeded   = "SUCCEEDED"
	PaymentLinkAttemptStateFailed      = "FAILED"
	PaymentLinkAttemptStateExpired     = "EXPIRED"
	PaymentLinkAttemptStateAbandoned   = "ABANDONED"
	PaymentLinkAttemptStateNeedsReview = "NEEDS_REVIEW"
)

// Payment link environments. The signing key decides the environment; an
// explicit value must match it.
const (
	PaymentLinkEnvironmentTest = "TEST"
	PaymentLinkEnvironmentLive = "LIVE"
)

// BuyerFieldRequirementRequired is the requirement of a buyer field on the
// payment page. Payment providers require the buyer, the identity number and
// the billing address, so the link page always asks all five buyer fields:
// REQUIRED is the only value (the gateway refuses any other with 400; leaving
// a field out means REQUIRED).
const BuyerFieldRequirementRequired = "REQUIRED"

// Types of a payment link custom field (no file uploads).
const (
	CustomFieldTypeText     = "TEXT"
	CustomFieldTypeNumber   = "NUMBER"
	CustomFieldTypeEmail    = "EMAIL"
	CustomFieldTypePhone    = "PHONE"
	CustomFieldTypeDate     = "DATE"
	CustomFieldTypeSelect   = "SELECT"
	CustomFieldTypeCheckbox = "CHECKBOX"
	CustomFieldTypeTCKN     = "TCKN"
	CustomFieldTypeVKN      = "VKN"
)

// Payment link button labels.
const (
	PaymentLinkButtonLabelPay    = "PAY"
	PaymentLinkButtonLabelDonate = "DONATE"
	PaymentLinkButtonLabelBuy    = "BUY"
	PaymentLinkButtonLabelBook   = "BOOK"
)

// Payment link delivery channels of Send and of the buyer verification.
const (
	PaymentLinkDeliveryChannelEmail = "EMAIL"
	PaymentLinkDeliveryChannelSMS   = "SMS"
)

// Payment link QR image formats.
const (
	PaymentLinkQRFormatSVG = "svg"
	PaymentLinkQRFormatPNG = "png"
)

// Payment page languages.
const (
	PaymentLinkLocaleTR = "tr"
	PaymentLinkLocaleEN = "en"
)

// Image types a payment link image can be declared as. Confirmation checks
// the stored bytes against the declared type; SVG is never accepted.
const (
	PaymentLinkImageContentTypePNG  = "image/png"
	PaymentLinkImageContentTypeJPEG = "image/jpeg"
	PaymentLinkImageContentTypeWebP = "image/webp"
)

// Bounds of the QR image size in pixels.
const (
	PaymentLinkQRMinSize = 128
	PaymentLinkQRMaxSize = 2048
)

// paymentLinkSpecFields lists every field of a payment-link definition in the
// gateway's REST spelling and order. Create bodies carry only these (plus
// environment), update bodies only these (plus expectedRowVersion).
var paymentLinkSpecFields = []string{
	"usage", "amountMode", "currency", "amountMinor", "minAmountMinor", "maxAmountMinor", "suggestedAmountsMinor",
	"quantityEnabled", "minQuantity", "maxQuantity", "items", "capacity", "title", "description", "imageAssetId",
	"successMessage", "successRedirectUrl", "taxNote", "locale", "startsAt", "expiresAt", "eventAt", "buttonLabelKey",
	"requireTermsAcceptance", "buyerFields", "customFields", "recipient", "prefill", "verification", "reminders",
	"checkout", "reference", "tags", "branchCode", "salesChannel", "campaign", "agentCode", "merchantNotifyEmails",
	"metadata",
}

// paymentLinkImmutableFields are fixed when a link is created.
var paymentLinkImmutableFields = []string{"usage", "amountMode", "currency"}

var paymentLinkActions = []string{"pause", "resume", "deactivate"}

const paymentLinkImagePublished = "PUBLISHED"

var (
	paymentLinkUsages        = []string{PaymentLinkUsageSingleUse, PaymentLinkUsageMultiUse}
	paymentLinkAmountModes   = []string{PaymentLinkAmountModeFixed, PaymentLinkAmountModeOpen, PaymentLinkAmountModeItemized}
	paymentLinkStates        = []string{PaymentLinkStateActive, PaymentLinkStatePaused, PaymentLinkStateCompleted, PaymentLinkStateExpired, PaymentLinkStateDeactivated, PaymentLinkStateTakenDown}
	paymentLinkAttemptStates = []string{PaymentLinkAttemptStateOpen, PaymentLinkAttemptStateBound, PaymentLinkAttemptStateSucceeded, PaymentLinkAttemptStateFailed, PaymentLinkAttemptStateExpired, PaymentLinkAttemptStateAbandoned, PaymentLinkAttemptStateNeedsReview}
	paymentLinkEnvironments  = []string{PaymentLinkEnvironmentTest, PaymentLinkEnvironmentLive}
	buyerFieldRequirements   = []string{BuyerFieldRequirementRequired}
	customFieldTypes         = []string{CustomFieldTypeText, CustomFieldTypeNumber, CustomFieldTypeEmail, CustomFieldTypePhone, CustomFieldTypeDate, CustomFieldTypeSelect, CustomFieldTypeCheckbox, CustomFieldTypeTCKN, CustomFieldTypeVKN}
	paymentLinkButtonLabels  = []string{PaymentLinkButtonLabelPay, PaymentLinkButtonLabelDonate, PaymentLinkButtonLabelBuy, PaymentLinkButtonLabelBook}
	paymentLinkChannels      = []string{PaymentLinkDeliveryChannelEmail, PaymentLinkDeliveryChannelSMS}
	paymentLinkQRFormats     = []string{PaymentLinkQRFormatSVG, PaymentLinkQRFormatPNG}
	paymentLinkLocales       = []string{PaymentLinkLocaleTR, PaymentLinkLocaleEN}
	paymentLinkImageTypes    = []string{PaymentLinkImageContentTypePNG, PaymentLinkImageContentTypeJPEG, PaymentLinkImageContentTypeWebP}
	paymentLinkUploadSchemes = []string{"https", "http"}
)

// PaymentLinkSpecFields returns a copy of the payment-link definition fields
// in the gateway's order.
func PaymentLinkSpecFields() []string { return slices.Clone(paymentLinkSpecFields) }

// PaymentLinkImmutableFields returns a copy of the fields fixed at creation
// (usage, amountMode, currency); Update refuses them.
func PaymentLinkImmutableFields() []string { return slices.Clone(paymentLinkImmutableFields) }

const paymentLinksPath = "/api/v1/payments/links"

func paymentLinkRoute(path string) bool {
	return path == paymentLinksPath || strings.HasPrefix(path, paymentLinksPath+"/")
}

// validPaymentLinkIdempotency applies the payment-link key format: 8–200
// printable ASCII characters without spaces.
func validPaymentLinkIdempotency(key string) bool { return len(key) >= 8 && safeHeader(key, 200) }

func oneOf(value string, allowed []string, field string) error {
	if !slices.Contains(allowed, value) {
		return invalid(field + " must be one of " + strings.Join(allowed, ", "))
	}
	return nil
}

package lunixi

import (
	"slices"
	"strings"
)

// Insurance (/api/v1/insurance, product INSURANCE_SERVICE): value sets,
// request fields, query filters and webhook events.

// Who collects the premium (premiumCollectedBy). PROVIDER orders cannot be
// completed through this API yet.
const (
	InsurancePremiumCollectedByMerchant = "MERCHANT"
	InsurancePremiumCollectedByProvider = "PROVIDER"
)

// Status of a quote option. Only a QUOTED option that has not expired
// becomes an order.
const (
	InsuranceQuoteStatusRequested = "REQUESTED"
	InsuranceQuoteStatusQuoted    = "QUOTED"
	InsuranceQuoteStatusExpired   = "EXPIRED"
	InsuranceQuoteStatusRejected  = "REJECTED"
	InsuranceQuoteStatusError     = "ERROR"
)

// The order's overall status. It is not stored: every read derives it from
// the four axes, in this order — REFUNDED, CANCELLED, FAILED,
// ACTION_REQUIRED, PROCESSING, PARTIALLY_ACTIVE, ACTIVE, CREATED.
const (
	InsuranceOrderStatusCreated         = "CREATED"
	InsuranceOrderStatusProcessing      = "PROCESSING"
	InsuranceOrderStatusActionRequired  = "ACTION_REQUIRED"
	InsuranceOrderStatusPartiallyActive = "PARTIALLY_ACTIVE"
	InsuranceOrderStatusActive          = "ACTIVE"
	InsuranceOrderStatusFailed          = "FAILED"
	InsuranceOrderStatusCancelled       = "CANCELLED"
	InsuranceOrderStatusRefunded        = "REFUNDED"
)

// Premium collection axis (collectionStatus).
const (
	InsuranceCollectionStatusNotStarted     = "NOT_STARTED"
	InsuranceCollectionStatusPending        = "PENDING"
	InsuranceCollectionStatusActionRequired = "ACTION_REQUIRED"
	InsuranceCollectionStatusCollected      = "COLLECTED"
	InsuranceCollectionStatusFailed         = "FAILED"
	InsuranceCollectionStatusUnknown        = "UNKNOWN"
)

// Policy axis (policyStatus) of an order and of each of its policies.
const (
	InsurancePolicyStatusNotRequested    = "NOT_REQUESTED"
	InsurancePolicyStatusPending         = "PENDING"
	InsurancePolicyStatusPartiallyIssued = "PARTIALLY_ISSUED"
	InsurancePolicyStatusIssued          = "ISSUED"
	InsurancePolicyStatusFailedRetryable = "FAILED_RETRYABLE"
	InsurancePolicyStatusFailedFinal     = "FAILED_FINAL"
	InsurancePolicyStatusUnknown         = "UNKNOWN"
	InsurancePolicyStatusCancelPending   = "CANCEL_PENDING"
	InsurancePolicyStatusCancelled       = "CANCELLED"
)

// Premium refund axis (refundStatus).
const (
	InsuranceRefundStatusNone     = "NONE"
	InsuranceRefundStatusRequired = "REQUIRED"
	InsuranceRefundStatusPending  = "PENDING"
	InsuranceRefundStatusRefunded = "REFUNDED"
	InsuranceRefundStatusFailed   = "FAILED"
)

// Policy document axis (documentStatus); it does not change the overall
// status.
const (
	InsuranceDocumentStatusNotAvailable = "NOT_AVAILABLE"
	InsuranceDocumentStatusPending      = "PENDING"
	InsuranceDocumentStatusAvailable    = "AVAILABLE"
	InsuranceDocumentStatusFailed       = "FAILED"
)

// How the premium of an order is paid (paymentMode).
const (
	InsurancePaymentModeLunixiPayment   = "LUNIXI_PAYMENT"
	InsurancePaymentModeExternalPayment = "EXTERNAL_PAYMENT"
	InsurancePaymentModeNoPayment       = "NO_PAYMENT"
)

// Where an order was created (surface).
const (
	InsuranceOrderSurfaceMerchantAPI  = "MERCHANT_API"
	InsuranceOrderSurfaceCheckoutForm = "CHECKOUT_FORM"
	InsuranceOrderSurfacePanel        = "PANEL"
)

// Status of a lead: what Lunixi knows about sending it to the insurance
// provider (the provider does not report back on a lead). PENDING: the
// provider call is still running. SUBMITTED: the provider received it
// (insurance.lead.submitted). UNKNOWN: the request reached the provider but
// the outcome is not known; it is never sent again — open a new lead with a
// new Idempotency-Key if needed. FAILED: the provider refused it or it never
// reached the provider (errorCode).
const (
	InsuranceLeadStatusPending   = "PENDING"
	InsuranceLeadStatusSubmitted = "SUBMITTED"
	InsuranceLeadStatusUnknown   = "UNKNOWN"
	InsuranceLeadStatusFailed    = "FAILED"
)

// Lunixi reference code lists (ListReferenceValues). DISTRICT, DEVICE_MODEL,
// VEHICLE_MODEL, BANK_BRANCH and PET_BREED need parentCode (province, brand,
// bank, CAT | DOG); COUNTRY, CITY and BANK take none. National standards are
// the codes as they are (ISO 3166-1 alpha-2 country, licence-plate province
// code 01–81); the others are Lunixi codes.
const (
	InsuranceReferenceKindCountry               = "COUNTRY"
	InsuranceReferenceKindCity                  = "CITY"
	InsuranceReferenceKindDistrict              = "DISTRICT"
	InsuranceReferenceKindDeviceBrand           = "DEVICE_BRAND"
	InsuranceReferenceKindDeviceModel           = "DEVICE_MODEL"
	InsuranceReferenceKindVehicleBrand          = "VEHICLE_BRAND"
	InsuranceReferenceKindVehicleModel          = "VEHICLE_MODEL"
	InsuranceReferenceKindVehicleType           = "VEHICLE_TYPE"
	InsuranceReferenceKindProductCategory       = "PRODUCT_CATEGORY"
	InsuranceReferenceKindVehicleUsage          = "VEHICLE_USAGE"
	InsuranceReferenceKindLiabilityLimit        = "LIABILITY_LIMIT"
	InsuranceReferenceKindOccupation            = "OCCUPATION"
	InsuranceReferenceKindPetBreed              = "PET_BREED"
	InsuranceReferenceKindDwellingType          = "DWELLING_TYPE"
	InsuranceReferenceKindDwellingFloor         = "DWELLING_FLOOR"
	InsuranceReferenceKindBank                  = "BANK"
	InsuranceReferenceKindBankBranch            = "BANK_BRANCH"
	InsuranceReferenceKindFinanceInstitution    = "FINANCE_INSTITUTION"
	InsuranceReferenceKindMedicalProcedureCover = "MEDICAL_PROCEDURE_COVER"
)

// Insurance webhook events (subscription insurance.*). Dedupe on the event
// id (data.eventId is the same value). Order events are read with
// ParseInsuranceWebhook, the lead event with ParseInsuranceLeadWebhook.
const (
	// InsuranceEventPolicyIssued: the order's policies were issued.
	InsuranceEventPolicyIssued = "insurance.policy.issued"
	// InsuranceEventPolicyFailed: the policy could not be issued or the
	// premium was not collected; data.errorCode says why.
	InsuranceEventPolicyFailed = "insurance.policy.failed"
	// InsuranceEventRefundRequired: the collected premium has to be
	// refunded; data.errorCode says why.
	InsuranceEventRefundRequired = "insurance.refund.required"
	// InsuranceEventPolicyDocumentReady: a policy document can be downloaded
	// (policies[].documentUrl).
	InsuranceEventPolicyDocumentReady = "insurance.policy.document_ready"
	// InsuranceEventPolicyCancelled: the provider confirmed the cancellation.
	InsuranceEventPolicyCancelled = "insurance.policy.cancelled"
	// InsuranceEventRefundCompleted: the premium refund completed.
	InsuranceEventRefundCompleted = "insurance.refund.completed"
	// InsuranceEventLeadSubmitted: the insurance provider received a lead.
	// data is a lead summary, not an order.
	InsuranceEventLeadSubmitted = "insurance.lead.submitted"
)

// InsuranceEventPattern is the subscription pattern of every insurance
// event.
const InsuranceEventPattern = "insurance.*"

// Limits of the insurance requests.
const (
	// InsuranceMaxLimit is the largest page (limit 1–100).
	InsuranceMaxLimit = 100
	// InsuranceMaxContextItems is the most basket items one context.items
	// may carry.
	InsuranceMaxContextItems = 100
	// InsuranceExportOrderCap is the most orders in one reconciliation CSV
	// file; a longer export ends with a continuation line.
	InsuranceExportOrderCap = 20000
	// InsuranceLeadNoteMaxLength is the longest lead note (characters).
	InsuranceLeadNoteMaxLength = 1000
)

const (
	insuranceBase         = "/api/v1/insurance"
	insuranceCursorMax    = 512
	insuranceTextMax      = 128
	insurancePDFAccept    = "application/pdf, application/json"
	insuranceCSVAccept    = "text/csv, application/json"
	insuranceShortTextMax = 64
)

// insuranceTypeObjectFields are the type objects of the gateway's type body,
// one per insurance type (InsuranceProduct.typeObject).
var insuranceTypeObjectFields = []string{
	"tickets", "stay", "device", "product", "vehicle", "trip", "rental", "instrument",
	"event", "accident", "procedure", "pet", "residence", "abroadStay",
}

var (
	insuranceQuoteFields = slices.Concat([]string{
		"insuranceType", "schemaVersion", "currency", "paymentIntentId", "context",
		"merchantOrderId", "policyholder", "insured",
	}, insuranceTypeObjectFields)
	insuranceLeadFields = slices.Concat([]string{
		"insuranceType", "schemaVersion", "policyholder", "insured",
	}, insuranceTypeObjectFields, []string{"shipment", "company", "note"})
	insuranceProductSearchFields   = []string{"currency", "insuranceType", "context"}
	insuranceOrderFields           = []string{"quoteId", "merchantOrderId", "consent"}
	insuranceConsentFields         = []string{"termsVersion", "termsSha256", "acceptedAt"}
	insuranceExternalPaymentFields = []string{"externalPaymentId", "paidAmount", "paidCurrency", "paidAt", "paymentProvider"}
	insuranceExternalRefundFields  = []string{"externalRefundId", "refundedAt"}
)

// insuranceFilters are the query filters each read accepts (the gateway
// rejects any other with 400).
var insuranceFilters = map[string][]string{
	"listProducts":             {"currency", "paymentIntentId", "insuranceType"},
	"listReferenceValues":      {"parentCode", "limit", "cursor"},
	"listOrders":               {"limit", "cursor", "paymentIntentId", "merchantOrderId"},
	"getReconciliationSummary": {"from", "to", "connectionId", "insuranceType", "currency"},
	"listReconciliationOrders": {"from", "to", "connectionId", "insuranceType", "currency", "limit", "cursor"},
	"exportReconciliation":     {"from", "to", "connectionId", "insuranceType", "currency", "cursor"},
	"listLeads":                {"limit", "cursor", "insuranceType", "status"},
}

// InsuranceQuoteFields returns the top-level fields CreateQuote accepts:
// after the person fields come the type objects, one per insurance type —
// tickets (a list) and the single objects stay, device, product, vehicle,
// trip, rental, instrument, event, accident, procedure, pet, residence,
// abroadStay. The field rules inside a type object are the gateway's.
func InsuranceQuoteFields() []string { return slices.Clone(insuranceQuoteFields) }

// InsuranceLeadFields returns the top-level fields CreateLead accepts: the
// type body of a quote without the basket, currency and order fields, plus
// shipment and company (lead schemas only) and the free note.
func InsuranceLeadFields() []string { return slices.Clone(insuranceLeadFields) }

// InsuranceProductSearchFields returns the fields SearchProducts accepts.
func InsuranceProductSearchFields() []string { return slices.Clone(insuranceProductSearchFields) }

// InsuranceOrderFields returns the fields CreateOrder accepts.
func InsuranceOrderFields() []string { return slices.Clone(insuranceOrderFields) }

// InsuranceConsentFields returns the fields of CreateOrder's consent.
func InsuranceConsentFields() []string { return slices.Clone(insuranceConsentFields) }

// InsuranceExternalPaymentFields returns the fields of ConfirmPayment (a
// premium collected outside Lunixi).
func InsuranceExternalPaymentFields() []string { return slices.Clone(insuranceExternalPaymentFields) }

// InsuranceExternalRefundFields returns the fields of RefundPremium's
// externalRefund (a premium collected outside Lunixi).
func InsuranceExternalRefundFields() []string { return slices.Clone(insuranceExternalRefundFields) }

// InsuranceFilters returns a copy of the query filters each insurance read
// accepts (route name → filter names).
func InsuranceFilters() map[string][]string { return cloneLists(insuranceFilters) }

// InsuranceReferenceKinds lists the reference kinds in wire order.
func InsuranceReferenceKinds() []string {
	return []string{InsuranceReferenceKindCountry, InsuranceReferenceKindCity, InsuranceReferenceKindDistrict, InsuranceReferenceKindDeviceBrand, InsuranceReferenceKindDeviceModel, InsuranceReferenceKindVehicleBrand, InsuranceReferenceKindVehicleModel, InsuranceReferenceKindVehicleType, InsuranceReferenceKindProductCategory, InsuranceReferenceKindVehicleUsage, InsuranceReferenceKindLiabilityLimit, InsuranceReferenceKindOccupation, InsuranceReferenceKindPetBreed, InsuranceReferenceKindDwellingType, InsuranceReferenceKindDwellingFloor, InsuranceReferenceKindBank, InsuranceReferenceKindBankBranch, InsuranceReferenceKindFinanceInstitution, InsuranceReferenceKindMedicalProcedureCover}
}

// InsuranceLeadStatuses lists the lead statuses in wire order.
func InsuranceLeadStatuses() []string {
	return []string{InsuranceLeadStatusPending, InsuranceLeadStatusSubmitted, InsuranceLeadStatusUnknown, InsuranceLeadStatusFailed}
}

// InsuranceOrderStatuses lists the overall order statuses in wire order.
func InsuranceOrderStatuses() []string {
	return []string{InsuranceOrderStatusCreated, InsuranceOrderStatusProcessing, InsuranceOrderStatusActionRequired, InsuranceOrderStatusPartiallyActive, InsuranceOrderStatusActive, InsuranceOrderStatusFailed, InsuranceOrderStatusCancelled, InsuranceOrderStatusRefunded}
}

// InsurancePolicyStatuses lists the policy statuses in wire order.
func InsurancePolicyStatuses() []string {
	return []string{InsurancePolicyStatusNotRequested, InsurancePolicyStatusPending, InsurancePolicyStatusPartiallyIssued, InsurancePolicyStatusIssued, InsurancePolicyStatusFailedRetryable, InsurancePolicyStatusFailedFinal, InsurancePolicyStatusUnknown, InsurancePolicyStatusCancelPending, InsurancePolicyStatusCancelled}
}

// InsuranceEventTypes lists every insurance webhook event.
func InsuranceEventTypes() []string {
	return append(InsuranceOrderEventTypes(), InsuranceLeadEventTypes()...)
}

// InsuranceOrderEventTypes lists the events about an order; their data is
// InsuranceWebhookData.
func InsuranceOrderEventTypes() []string {
	return []string{InsuranceEventPolicyIssued, InsuranceEventPolicyFailed, InsuranceEventRefundRequired, InsuranceEventPolicyDocumentReady, InsuranceEventPolicyCancelled, InsuranceEventRefundCompleted}
}

// InsuranceLeadEventTypes lists the events about a lead; their data is
// InsuranceLeadWebhookData.
func InsuranceLeadEventTypes() []string { return []string{InsuranceEventLeadSubmitted} }

func insuranceRoute(path string) bool {
	return path == insuranceBase || strings.HasPrefix(path, insuranceBase+"/")
}

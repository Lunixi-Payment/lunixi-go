package lunixi

import (
	"slices"
	"strings"
)

// Physical POS values, request fields and filters (/api/v1/physical-pos).
//
// Statuses and operations are open sets on the wire: the SDK lists the values
// the gateway sends today and never rejects one it does not know. The field
// and filter lists are closed: the gateway rejects an unknown body field or
// query parameter with 400, so the SDK checks them before sending.

const physicalPosBase = "/api/v1/physical-pos"

// Physical POS payment operations. SALE, PREAUTH_OPEN and REFUND_UNMATCHED
// start a payment (CreatePayment); VOID, REFUND, PREAUTH_CLOSE and
// PREAUTH_CANCEL reverse one (CreateReversal).
const (
	PhysicalPosOperationSale            = "SALE"
	PhysicalPosOperationPreauthOpen     = "PREAUTH_OPEN"
	PhysicalPosOperationRefundUnmatched = "REFUND_UNMATCHED"
	PhysicalPosOperationVoid            = "VOID"
	PhysicalPosOperationRefund          = "REFUND"
	PhysicalPosOperationPreauthClose    = "PREAUTH_CLOSE"
	PhysicalPosOperationPreauthCancel   = "PREAUTH_CANCEL"
)

// Physical POS payment statuses (paymentStatus).
const (
	PhysicalPosStatusNotStarted        = "NOT_STARTED"
	PhysicalPosStatusInProgress        = "IN_PROGRESS"
	PhysicalPosStatusApproved          = "APPROVED"
	PhysicalPosStatusPartiallyApproved = "PARTIALLY_APPROVED"
	PhysicalPosStatusDeclined          = "DECLINED"
	PhysicalPosStatusCancelled         = "CANCELLED"
	PhysicalPosStatusExpired           = "EXPIRED"
	PhysicalPosStatusFailed            = "FAILED"
	PhysicalPosStatusUnknown           = "UNKNOWN"
	PhysicalPosStatusManualReview      = "MANUAL_REVIEW"
)

// Physical POS reversal statuses of a root payment (reversalStatus).
const (
	PhysicalPosReversalStatusNone              = "NONE"
	PhysicalPosReversalStatusPartiallyRefunded = "PARTIALLY_REFUNDED"
	PhysicalPosReversalStatusRefunded          = "REFUNDED"
	PhysicalPosReversalStatusVoided            = "VOIDED"
	PhysicalPosReversalStatusPreauthClosed     = "PREAUTH_CLOSED"
	PhysicalPosReversalStatusPreauthCancelled  = "PREAUTH_CANCELLED"
	PhysicalPosReversalStatusPreauthExpired    = "PREAUTH_EXPIRED"
)

// Physical POS webhook event types. physical_pos.payment.created and
// .in_progress are internal and never delivered. Reversals (void, refund,
// pre-authorisation close / cancel) use the same payment events; tell them
// apart with data.operation and data.rootPaymentId, and the original
// payment's source with data.original.origin.
//
// PhysicalPosEventPaymentDeviceSale reports a sale made directly on the device
// (origin DEVICE_DIRECT): it arrives approved, with deviceOccurredAt; no
// approved event is sent for it.
//
// End-of-day events (physical_pos.settlement.*) have their own payloads:
// completed (the device's result with its bank lines; any source), failed (a
// scheduled run could not finish in its window), skipped (a scheduled run was
// skipped for a reason you can fix) and overdue (no result within the
// policy's threshold). They carry no historySeq; deduplicate them by the
// event id.
const (
	PhysicalPosEventPaymentApproved              = "physical_pos.payment.approved"
	PhysicalPosEventPaymentPartiallyApproved     = "physical_pos.payment.partially_approved"
	PhysicalPosEventPaymentDeclined              = "physical_pos.payment.declined"
	PhysicalPosEventPaymentCancelled             = "physical_pos.payment.cancelled"
	PhysicalPosEventPaymentExpired               = "physical_pos.payment.expired"
	PhysicalPosEventPaymentFailed                = "physical_pos.payment.failed"
	PhysicalPosEventPaymentUnknown               = "physical_pos.payment.unknown"
	PhysicalPosEventPaymentManualReview          = "physical_pos.payment.manual_review"
	PhysicalPosEventPaymentDeviceSale            = "physical_pos.payment.device_sale"
	PhysicalPosEventFiscalDocumentIssued         = "physical_pos.fiscal_document.issued"
	PhysicalPosEventFiscalDocumentFailed         = "physical_pos.fiscal_document.failed"
	PhysicalPosEventFiscalDocumentCancelled      = "physical_pos.fiscal_document.cancelled"
	PhysicalPosEventFiscalDocumentActionRequired = "physical_pos.fiscal_document.action_required"
	PhysicalPosEventSettlementCompleted          = "physical_pos.settlement.completed"
	PhysicalPosEventSettlementFailed             = "physical_pos.settlement.failed"
	PhysicalPosEventSettlementOverdue            = "physical_pos.settlement.overdue"
	PhysicalPosEventSettlementSkipped            = "physical_pos.settlement.skipped"
	// PhysicalPosEventPattern subscribes to every physical POS event.
	PhysicalPosEventPattern = "physical_pos.*"
)

// Categories of a physical POS webhook event (PhysicalPosWebhookEvent.Category).
const (
	PhysicalPosWebhookCategoryPayment        = "payment"
	PhysicalPosWebhookCategoryFiscalDocument = "fiscal_document"
	PhysicalPosWebhookCategorySettlement     = "settlement"
	PhysicalPosWebhookCategoryOther          = "other"
)

// Physical POS limits.
const (
	// PhysicalPosMaxLimit is the largest page size of a physical POS list.
	PhysicalPosMaxLimit = 100
	// PhysicalPosRowVersionMax is the largest rowVersion / expectedRowVersion.
	PhysicalPosRowVersionMax int64 = 2147483647
)

// Error codes with their own error type.
const (
	// PhysicalPosCardDataRejectedCode: the request carried card data.
	PhysicalPosCardDataRejectedCode = "physical_pos.payment.card_data_rejected"
	// PhysicalPosVoidAfterSettlementCode: the payment is already in a device
	// end-of-day and can only be refunded.
	PhysicalPosVoidAfterSettlementCode = "physical_pos.payment.void_after_settlement"
)

var physicalPosEventTypes = []string{
	PhysicalPosEventPaymentApproved, PhysicalPosEventPaymentPartiallyApproved, PhysicalPosEventPaymentDeclined,
	PhysicalPosEventPaymentCancelled, PhysicalPosEventPaymentExpired, PhysicalPosEventPaymentFailed,
	PhysicalPosEventPaymentUnknown, PhysicalPosEventPaymentManualReview, PhysicalPosEventPaymentDeviceSale,
	PhysicalPosEventFiscalDocumentIssued, PhysicalPosEventFiscalDocumentFailed, PhysicalPosEventFiscalDocumentCancelled,
	PhysicalPosEventFiscalDocumentActionRequired, PhysicalPosEventSettlementCompleted, PhysicalPosEventSettlementFailed,
	PhysicalPosEventSettlementOverdue, PhysicalPosEventSettlementSkipped,
}

// physicalPosNotStartedCodes are the 5xx codes after which the gateway
// guarantees nothing was written (the payment did not start). Any other 5xx,
// and a request without an HTTP answer, leaves the outcome of a payment write
// unknown.
var physicalPosNotStartedCodes = []string{
	"physical_pos.presence.unavailable",
	"physical_pos.signing.unavailable",
	"physical_pos.service.busy",
	"physical_pos.service.clock_skew",
	"physical_pos.service.not_configured",
}

var (
	physicalPosTargetFields     = []string{"deviceId", "vendorCode", "serialNumber", "externalRef"}
	physicalPosSaleItemFields   = []string{"lineNo", "name", "quantityMilli", "unitCode", "unitPriceMinor", "taxGroupCode", "lineTotalMinor", "barcode"}
	physicalPosRefundItemFields = []string{"lineNo", "quantityMilli", "amountMinor"}
)

// PhysicalPosEventTypes returns every delivered physical POS event type.
func PhysicalPosEventTypes() []string { return slices.Clone(physicalPosEventTypes) }

// PhysicalPosNotStartedCodes returns the 5xx error codes that guarantee a
// payment write started nothing.
func PhysicalPosNotStartedCodes() []string { return slices.Clone(physicalPosNotStartedCodes) }

// PhysicalPosPaymentTargetFields returns the fields of a payment target:
// exactly one of deviceId, vendorCode + serialNumber, or externalRef.
func PhysicalPosPaymentTargetFields() []string { return slices.Clone(physicalPosTargetFields) }

// PhysicalPosSaleItemFields returns the fields of a sale item (createPayment items).
func PhysicalPosSaleItemFields() []string { return slices.Clone(physicalPosSaleItemFields) }

// PhysicalPosRefundItemFields returns the fields of a refund item (createReversal items).
func PhysicalPosRefundItemFields() []string { return slices.Clone(physicalPosRefundItemFields) }

// physicalPosField is one body field with its kind: string, int, money,
// bool, strings (list of strings), rowVersion, target, saleItems,
// refundItems. A "*" suffix marks a required field; "?" allows null (an
// explicit clear).
type physicalPosField struct{ name, kind string }

func posFields(pairs ...string) []physicalPosField {
	out := make([]physicalPosField, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, physicalPosField{pairs[i], pairs[i+1]})
	}
	return out
}

var physicalPosSaleItemKinds = posFields(
	"lineNo", "int*", "name", "string*", "quantityMilli", "int*", "unitCode", "string*", "unitPriceMinor", "money*",
	"taxGroupCode", "string*", "lineTotalMinor", "money*", "barcode", "string",
)

var physicalPosRefundItemKinds = posFields("lineNo", "int*", "quantityMilli", "int*", "amountMinor", "money")

// physicalPosRequestFields are the request bodies by operation, in the
// gateway's field order.
var physicalPosRequestFields = map[string][]physicalPosField{
	"createPayment": posFields(
		"operation", "string*", "target", "target*", "amountMinor", "money*", "currency", "string*",
		"installmentCount", "int?", "instrument", "string?", "merchantReference", "string?",
		"allowDuplicateReference", "bool", "startTimeoutSeconds", "int?", "terminalAcquirerId", "string?",
		"items", "saleItems", "interaction", "string?", "terminalProviderId", "string?",
	),
	"createReversal": posFields(
		"operation", "string*", "amountMinor", "money", "currency", "string*", "merchantReference", "string?",
		"startTimeoutSeconds", "int?", "target", "target?", "items", "refundItems",
	),
	"createStore": posFields(
		"code", "string*", "name", "string*", "city", "string", "district", "string", "addressLine", "string",
		"timezone", "string", "externalRef", "string",
	),
	"updateStore": posFields(
		"rowVersion", "rowVersion*", "name", "string", "city", "string?", "district", "string?", "addressLine", "string?",
		"timezone", "string", "externalRef", "string?",
	),
	"statusChange":   posFields("rowVersion", "rowVersion*", "reasonCode", "string*"),
	"createRegister": posFields("code", "string*", "name", "string*"),
	"updateRegister": posFields("rowVersion", "rowVersion*", "name", "string"),
	"createTerminal": posFields(
		"code", "string*", "displayName", "string*", "vendorCode", "string", "virtualSerial", "string",
		"vendorTerminalRef", "string",
	),
	"updateTerminal": posFields("rowVersion", "rowVersion*", "displayName", "string", "vendorCode", "string"),
	"addTerminalAcquirer": posFields(
		"vendorAcquirerRef", "string*", "merchantNumber", "string*", "terminalNumber", "string*", "label", "string",
		"currencies", "strings*",
	),
	"updateTerminalAcquirer": posFields("rowVersion", "rowVersion*", "label", "string?", "currencies", "strings", "status", "string"),
	"addTerminalProvider": posFields(
		"vendorAccountId", "string*", "merchantCode", "string*", "terminalCode", "string*", "productCode", "string",
		"label", "string",
	),
	"updateTerminalProvider": posFields("rowVersion", "rowVersion*", "label", "string?", "status", "string"),
	"changeTerminalDevice":   posFields("rowVersion", "rowVersion*", "deviceId", "string*", "reasonCode", "string*"),
	"createDevice": posFields(
		"vendorCode", "string*", "deviceModelId", "string*", "serialNumber", "string*", "displayName", "string",
		"externalRef", "string", "storeId", "string", "registerId", "string", "integrationChannel", "string*",
		"channelVariant", "string", "appMode", "string",
	),
	"updateDevice": posFields(
		"rowVersion", "rowVersion*", "displayName", "string?", "externalRef", "string?", "integrationChannel", "string",
		"channelVariant", "string?", "appMode", "string?",
	),
	"assignDevice":           posFields("rowVersion", "rowVersion*", "storeId", "string*", "registerId", "string?", "reasonCode", "string*"),
	"changeDeviceLifecycle":  posFields("rowVersion", "rowVersion*", "target", "string*", "reasonCode", "string*"),
	"revokeDeviceKey":        posFields("rowVersion", "rowVersion*", "reasonCode", "string*"),
	"revertDeviceEnrollment": posFields("reasonCode", "string*"),
	"revokeActivationCode":   posFields("reasonCode", "string*"),
	"setSettlementPolicy": posFields(
		"mode", "string*", "scheduleTime", "string", "timeZone", "string", "scheduleDays", "strings",
		"retryWindowMinutes", "int", "retryIntervalMinutes", "int", "offlineBehavior", "string",
		"overdueAlertHours", "int", "expectedRowVersion", "rowVersion",
	),
}

var (
	physicalPosPage       = []string{"limit", "cursor"}
	physicalPosEntityPage = []string{"limit", "cursor", "includeTotal"}
	physicalPosReport     = []string{"dateFrom", "dateTo", "deviceId", "storeId"}
)

func posFilterList(base []string, more ...string) []string {
	return append(slices.Clone(base), more...)
}

// physicalPosFilters are the query parameters per route; anything else is
// rejected before sending.
var physicalPosFilters = map[string][]string{
	"listPayments": posFilterList(physicalPosPage, "deviceId", "paymentStatus", "createdFrom", "createdTo", "merchantReference"),
	"listTransactions": posFilterList(physicalPosPage, "deviceId", "storeId", "registerId", "paymentStatus", "createdFrom",
		"createdTo", "merchantReference", "amountFromMinor", "amountToMinor", "origin", "operation"),
	"listAmbiguousPayments": posFilterList(physicalPosPage, "deviceId", "paymentStatus"),
	"getSalesSummary":       posFilterList(physicalPosReport, "groupBy"),
	"getTaxSummary":         posFilterList(physicalPosReport),
	"listSettlements":       posFilterList(physicalPosPage, "deviceId", "lineStatus", "createdFrom", "createdTo"),
	"listSettlementRuns":    posFilterList(physicalPosEntityPage, "deviceId", "source", "runStatus", "createdFrom", "createdTo"),
	"listDeviceModels":      {"vendorCode", "deviceType", "status"},
	"listStores":            posFilterList(physicalPosEntityPage, "status", "q"),
	"listRegisters":         posFilterList(physicalPosEntityPage, "status"),
	"listTerminals":         posFilterList(physicalPosEntityPage, "status", "q"),
	"listTerminalAcquirers": posFilterList(physicalPosEntityPage),
	"listTerminalProviders": posFilterList(physicalPosEntityPage),
	"listDevices": posFilterList(physicalPosEntityPage, "storeId", "registerId", "vendorCode", "deviceModelId", "deviceType",
		"lifecycleStatus", "ownershipStatus", "integrationChannel", "q"),
	"history": posFilterList(physicalPosPage),
}

// PhysicalPosRequestFields returns the body fields of a physical POS request
// in the gateway's order, by operation name: createPayment, createReversal,
// createStore, updateStore, statusChange (archive / restore / close / retire
// bodies), createRegister, updateRegister, createTerminal, updateTerminal,
// addTerminalAcquirer, updateTerminalAcquirer, addTerminalProvider,
// updateTerminalProvider, changeTerminalDevice, createDevice, updateDevice,
// assignDevice, changeDeviceLifecycle, revokeDeviceKey,
// revertDeviceEnrollment, revokeActivationCode, setSettlementPolicy. Nil for
// an unknown name.
func PhysicalPosRequestFields(operation string) []string {
	fields, ok := physicalPosRequestFields[operation]
	if !ok {
		return nil
	}
	out := make([]string, len(fields))
	for i, field := range fields {
		out[i] = field.name
	}
	return out
}

// PhysicalPosFilters returns the query parameters a physical POS list or
// report accepts, by route name (listPayments, listTransactions,
// listAmbiguousPayments, getSalesSummary, getTaxSummary, listSettlements,
// listSettlementRuns, listDeviceModels, listStores, listRegisters,
// listTerminals, listTerminalAcquirers, listTerminalProviders, listDevices,
// and history for the audit, link, assignment and key lists). Nil for an
// unknown name.
func PhysicalPosFilters(route string) []string {
	filters, ok := physicalPosFilters[route]
	if !ok {
		return nil
	}
	return slices.Clone(filters)
}

func physicalPosRoute(path string) bool {
	return path == physicalPosBase || strings.HasPrefix(path, physicalPosBase+"/")
}

// validPhysicalPosIdempotency: 8–200 printable ASCII characters, no space.
func validPhysicalPosIdempotency(key string) bool { return len(key) >= 8 && safeHeader(key, 200) }

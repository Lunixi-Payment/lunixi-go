package lunixi

import (
	"regexp"
	"strings"
)

// Bank accounts (/api/v1/bank-connect, product BANK_CONNECT): value sets,
// query filters, the transfer and exchange route table and webhook events.

// Status of a bank connection. Data is read only while a connection is
// ACTIVE or DEGRADED (DEGRADED: the last reads from the bank failed, so the
// data may be older; see the account's freshness). RequestSync on any other
// status is 409 bank_connect.connection.not_active.
const (
	BankConnectionStatusPendingDocuments = "PENDING_DOCUMENTS"
	BankConnectionStatusUnderReview      = "UNDER_REVIEW"
	BankConnectionStatusPendingSetup     = "PENDING_SETUP"
	BankConnectionStatusActive           = "ACTIVE"
	BankConnectionStatusDegraded         = "DEGRADED"
	BankConnectionStatusSuspended        = "SUSPENDED"
	BankConnectionStatusDisconnected     = "DISCONNECTED"
)

// Status of a connected bank account.
const (
	BankAccountStatusActive   = "ACTIVE"
	BankAccountStatusInactive = "INACTIVE"
)

// Direction of a bank transaction: money in (CREDIT) or out (DEBIT).
// amountMinor is always positive.
const (
	BankTransactionDirectionCredit = "CREDIT"
	BankTransactionDirectionDebit  = "DEBIT"
)

// Lunixi's bank-independent transaction type (canonicalType).
const (
	BankTransactionTypeTransferIn          = "TRANSFER_IN"
	BankTransactionTypeTransferOut         = "TRANSFER_OUT"
	BankTransactionTypeInternalIn          = "INTERNAL_IN"
	BankTransactionTypeInternalOut         = "INTERNAL_OUT"
	BankTransactionTypePosSettlement       = "POS_SETTLEMENT"
	BankTransactionTypePosSettlementRefund = "POS_SETTLEMENT_REFUND"
	BankTransactionTypeFee                 = "FEE"
	BankTransactionTypeTax                 = "TAX"
	BankTransactionTypeCashIn              = "CASH_IN"
	BankTransactionTypeCashOut             = "CASH_OUT"
	BankTransactionTypeCard                = "CARD"
	BankTransactionTypeLoan                = "LOAN"
	BankTransactionTypeFX                  = "FX"
	BankTransactionTypeInterest            = "INTEREST"
	BankTransactionTypeOther               = "OTHER"
)

// Availability of a bank in the catalogue. AVAILABLE: the bank can be
// connected (connections are started in the Lunixi dashboard).
const (
	BankAvailabilityAvailable  = "AVAILABLE"
	BankAvailabilityComingSoon = "COMING_SOON"
)

// What a bank or a connection offers (capabilities). OUTGOING_TRANSFER and
// FX_EXCHANGE: the connection can send transfers (BankConnect.Transfers) and
// exchange currency (BankConnect.FX).
const (
	BankCapabilityAccountList      = "ACCOUNT_LIST"
	BankCapabilityBalance          = "BALANCE"
	BankCapabilityTransactions     = "TRANSACTIONS"
	BankCapabilityOutgoingTransfer = "OUTGOING_TRANSFER"
	BankCapabilityFXExchange       = "FX_EXCHANGE"
)

// Answer of RequestSync. QUEUED: the read starts now or is already waiting.
// SCHEDULED: it runs at scheduledAt.
const (
	BankSyncStatusQueued    = "QUEUED"
	BankSyncStatusScheduled = "SCHEDULED"
)

// BankConnectEventTransactionReceived: a new transaction (CREDIT or DEBIT)
// was read from one of your connected accounts. Sent once per transaction,
// for transactions first seen by a regular read after the connection became
// active; the history loaded when an account is first connected does not
// send it. The event id is the same on every redelivery: dedupe on it. Read
// the data with ParseBankAccountTransactionReceived.
const BankConnectEventTransactionReceived = "bank_account.transaction.received"

// BankConnectEventPattern is the subscription pattern of every bank-account
// event.
const BankConnectEventPattern = "bank_account.*"

// Status of an outgoing bank transfer. PENDING_OTP: started in the
// dashboard, waiting for the user's e-mail code (an API-key transfer never
// has it). SUBMITTED: approved and queued or sent to the bank. PROCESSING:
// the bank is processing it. COMPLETED: the bank completed it; it can still
// become RETURNED if the receiving bank sends it back. FAILED, RETURNED and
// CANCELED are final.
const (
	BankTransferStatusPendingOtp = "PENDING_OTP"
	BankTransferStatusSubmitted  = "SUBMITTED"
	BankTransferStatusProcessing = "PROCESSING"
	BankTransferStatusCompleted  = "COMPLETED"
	BankTransferStatusFailed     = "FAILED"
	BankTransferStatusReturned   = "RETURNED"
	BankTransferStatusCanceled   = "CANCELED"
)

// The rail the bank chose: own accounts, same bank, FAST, EFT; UNKNOWN until
// it says.
const (
	BankTransferRailInternal  = "INTERNAL"
	BankTransferRailIntrabank = "INTRABANK"
	BankTransferRailFast      = "FAST"
	BankTransferRailEft       = "EFT"
	BankTransferRailUnknown   = "UNKNOWN"
)

// Where a transfer or an exchange order was started.
const (
	BankTransferInitiatedViaPanel = "PANEL"
	BankTransferInitiatedViaAPI   = "API"
)

// Status of a currency exchange order. An order is created SUBMITTED.
const (
	BankFXOrderStatusSubmitted = "SUBMITTED"
	BankFXOrderStatusCompleted = "COMPLETED"
	BankFXOrderStatusFailed    = "FAILED"
)

// Which amount of an exchange quote you fixed: SOURCE (the amount you sell)
// or TARGET (the amount you buy).
const (
	BankFXRequestedSideSource = "SOURCE"
	BankFXRequestedSideTarget = "TARGET"
)

// Transfer webhook events (product BANK_CONNECT, bank_transfer.*), one per
// transfer and type; the event id is the same on every redelivery. Read the
// data with ParseBankTransferWebhook. They are not the payment events of a
// BANK_TRANSFER payment (PaymentBankTransferEvent*, payment.bank_transfer.*).
// Exchange orders have no webhook.
const (
	// BankTransferEventCreated: a transfer was created (API: SUBMITTED;
	// dashboard: PENDING_OTP).
	BankTransferEventCreated = "bank_transfer.created"
	// BankTransferEventCompleted: the bank completed the transfer.
	BankTransferEventCompleted = "bank_transfer.completed"
	// BankTransferEventFailed: the transfer failed; failureCode says why.
	BankTransferEventFailed = "bank_transfer.failed"
	// BankTransferEventReturned: the receiving bank sent a completed
	// transfer back.
	BankTransferEventReturned = "bank_transfer.returned"
)

// BankTransferEventPattern is the subscription pattern of every transfer
// event.
const BankTransferEventPattern = "bank_transfer.*"

// Limits of the bank-connect lists and transfer bodies.
const (
	// BankConnectMaxLimit is the largest page (limit 1–100; the gateway's
	// default is 25).
	BankConnectMaxLimit = 100
	// BankConnectMaxAccountFilter is the most account ids one accountId
	// filter may carry.
	BankConnectMaxAccountFilter = 100
	// BankConnectBookedRangeMaxDays is the widest bookedFrom … bookedTo
	// window of one transaction request.
	BankConnectBookedRangeMaxDays = 92
	// BankTransferDescriptionMax is the longest description of a transfer or
	// an exchange order (characters).
	BankTransferDescriptionMax = 140
	// BankTransferReceiverNameMin and BankTransferReceiverNameMax bound
	// receiverName (characters).
	BankTransferReceiverNameMin = 2
	BankTransferReceiverNameMax = 140
	// BankTransferIBANMax is the longest receiverIban the gateway accepts
	// (spaces included); the service checks the TR IBAN checksum.
	BankTransferIBANMax = 42
)

// bankTransferPaymentPurposePattern: a code from ListPurposes.
var bankTransferPaymentPurposePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

const bankConnectCursorMax = 2048

const bankConnectBase = "/api/v1/bank-connect"

// bankConnectFilters are the query filters each read accepts (the gateway
// rejects any other with 400).
var bankConnectFilters = map[string][]string{
	"listBanks":        {"limit", "cursor"},
	"listConnections":  {"limit", "cursor", "status", "bankId"},
	"listAccounts":     {"limit", "cursor", "connectionId", "bankId", "currency", "status"},
	"listBalances":     {"limit", "cursor", "accountId", "currency"},
	"listTransactions": {"limit", "cursor", "accountId", "bookedFrom", "bookedTo", "direction", "minAmountMinor", "maxAmountMinor", "canonicalType"},
}

// bankTransferRoutes is the single source of method, path, body fields,
// query fields and the Idempotency-Key rule of BankTransferClient and
// BankFXClient (the gateway runs with forbidNonWhitelisted: any other field
// is a 400). Every route is signed and needs bank-connect:transfer plus a key
// with an IP allowlist.
var bankTransferRoutes = map[string]Route{
	"listTransferPurposes": {Method: "GET", Path: bankConnectBase + "/transfer-purposes", Query: []string{"connectionId"}},
	"createTransfer": {
		Method: "POST", Path: bankConnectBase + "/transfers", IdempotencyKey: true,
		Required: []string{"accountId", "receiverIban", "receiverName", "amountMinor"},
		Optional: []string{"currency", "paymentPurpose", "description"},
	},
	"listTransfers": {Method: "GET", Path: bankConnectBase + "/transfers", Query: []string{"limit", "cursor", "accountId", "status", "createdFrom", "createdTo"}},
	"getTransfer":   {Method: "GET", Path: bankConnectBase + "/transfers/:transferId"},
	"listFxRates":   {Method: "GET", Path: bankConnectBase + "/fx/rates", Query: []string{"connectionId"}},
	"createFxQuote": {
		Method: "POST", Path: bankConnectBase + "/fx/quotes",
		Required: []string{"sourceAccountId", "targetAccountId"},
		Optional: []string{"sourceAmountMinor", "targetAmountMinor"},
	},
	"createFxOrder": {
		Method: "POST", Path: bankConnectBase + "/fx/orders", IdempotencyKey: true,
		Required: []string{"quoteId"},
		Optional: []string{"description"},
	},
	"listFxOrders": {Method: "GET", Path: bankConnectBase + "/fx/orders", Query: []string{"limit", "cursor", "status"}},
	"getFxOrder":   {Method: "GET", Path: bankConnectBase + "/fx/orders/:orderId"},
}

// BankConnectFilters returns a copy of the query filters each bank-account
// read accepts (route name → filter names).
func BankConnectFilters() map[string][]string { return cloneLists(bankConnectFilters) }

// BankTransferRoutes returns a copy of the transfer and exchange route table
// (route name → Route), the same table as the other Lunixi SDKs.
func BankTransferRoutes() map[string]Route { return cloneRoutes(bankTransferRoutes) }

// BankConnectionStatuses lists the connection statuses in wire order.
func BankConnectionStatuses() []string {
	return []string{BankConnectionStatusPendingDocuments, BankConnectionStatusUnderReview, BankConnectionStatusPendingSetup, BankConnectionStatusActive, BankConnectionStatusDegraded, BankConnectionStatusSuspended, BankConnectionStatusDisconnected}
}

// BankAccountStatuses lists the account statuses in wire order.
func BankAccountStatuses() []string {
	return []string{BankAccountStatusActive, BankAccountStatusInactive}
}

// BankTransactionDirections lists the transaction directions in wire order.
func BankTransactionDirections() []string {
	return []string{BankTransactionDirectionCredit, BankTransactionDirectionDebit}
}

// BankTransactionTypes lists the canonical transaction types in wire order.
func BankTransactionTypes() []string {
	return []string{BankTransactionTypeTransferIn, BankTransactionTypeTransferOut, BankTransactionTypeInternalIn, BankTransactionTypeInternalOut, BankTransactionTypePosSettlement, BankTransactionTypePosSettlementRefund, BankTransactionTypeFee, BankTransactionTypeTax, BankTransactionTypeCashIn, BankTransactionTypeCashOut, BankTransactionTypeCard, BankTransactionTypeLoan, BankTransactionTypeFX, BankTransactionTypeInterest, BankTransactionTypeOther}
}

// BankCapabilities lists the bank and connection capabilities in wire order.
func BankCapabilities() []string {
	return []string{BankCapabilityAccountList, BankCapabilityBalance, BankCapabilityTransactions, BankCapabilityOutgoingTransfer, BankCapabilityFXExchange}
}

// BankTransferStatuses lists the transfer statuses in wire order.
func BankTransferStatuses() []string {
	return []string{BankTransferStatusPendingOtp, BankTransferStatusSubmitted, BankTransferStatusProcessing, BankTransferStatusCompleted, BankTransferStatusFailed, BankTransferStatusReturned, BankTransferStatusCanceled}
}

// BankTransferRails lists the transfer rails in wire order.
func BankTransferRails() []string {
	return []string{BankTransferRailInternal, BankTransferRailIntrabank, BankTransferRailFast, BankTransferRailEft, BankTransferRailUnknown}
}

// BankFXOrderStatuses lists the exchange order statuses in wire order.
func BankFXOrderStatuses() []string {
	return []string{BankFXOrderStatusSubmitted, BankFXOrderStatusCompleted, BankFXOrderStatusFailed}
}

// BankConnectEventTypes lists the bank-account webhook events.
func BankConnectEventTypes() []string { return []string{BankConnectEventTransactionReceived} }

// BankTransferEventTypes lists the transfer webhook events.
func BankTransferEventTypes() []string {
	return []string{BankTransferEventCreated, BankTransferEventCompleted, BankTransferEventFailed, BankTransferEventReturned}
}

func bankConnectRoute(path string) bool {
	return path == bankConnectBase || strings.HasPrefix(path, bankConnectBase+"/")
}

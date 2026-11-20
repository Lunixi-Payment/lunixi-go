package lunixi

import (
	"regexp"
	"strings"
)

// Wallet — the merchant (server-to-server) API of the closed-loop wallet,
// /api/v1/wallet/* (product WALLET_SERVICE).

// Account type of a wallet end user.
const (
	WalletAccountTypePersonal = "PERSONAL"
	WalletAccountTypeBusiness = "BUSINESS"
)

// Status of a wallet end user.
const (
	WalletEndUserStatusActive = "ACTIVE"
	WalletEndUserStatusFrozen = "FROZEN"
	WalletEndUserStatusClosed = "CLOSED"
)

// KYC level of a wallet end user.
const (
	WalletKycLevelNone        = "NONE"
	WalletKycLevelLow         = "LOW"
	WalletKycLevelSubstantial = "SUBSTANTIAL"
	WalletKycLevelHigh        = "HIGH"
)

// Channel of a wallet one-time code (otpChannel).
const (
	WalletOtpChannelEmail = "EMAIL"
	WalletOtpChannelSMS   = "SMS"
	WalletOtpChannelPush  = "PUSH"
)

// Type of a wallet QR code. Static codes carry no amount; dynamic ones
// require it.
const (
	WalletQRTypeP2PStatic       = "P2P_STATIC"
	WalletQRTypeP2PDynamic      = "P2P_DYNAMIC"
	WalletQRTypeMerchantStatic  = "MERCHANT_STATIC"
	WalletQRTypeMerchantDynamic = "MERCHANT_DYNAMIC"
)

// Image format of WalletQR.Image.
const (
	WalletQRImageFormatSVG = "svg"
	WalletQRImageFormatPNG = "png"
)

// Target of a bulk payout item (targetType).
const (
	WalletBulkTargetTypeWallet = "WALLET"
	WalletBulkTargetTypeIBAN   = "IBAN"
)

// Status of a wallet corporate account.
const (
	WalletCorporateAccountStatusActive   = "ACTIVE"
	WalletCorporateAccountStatusArchived = "ARCHIVED"
)

// WalletIdempotencyKeyMax is the longest wallet Idempotency-Key (the gateway
// accepts 1–128 characters).
const WalletIdempotencyKeyMax = 128

// walletAmountPattern: minor units as digit text ("12550" is 125.50 TRY).
var walletAmountPattern = regexp.MustCompile(`^\d{1,18}$`)

// walletAmountFields are the body fields that carry a minor-unit amount.
var walletAmountFields = []string{"amount"}

const walletQRImageAccept = "image/svg+xml, image/png, application/json"

// walletRoutes is the single description of every wallet route the SDK
// calls: method, path template, whether the gateway requires an
// Idempotency-Key, and the body / query fields the gateway DTO accepts (it
// runs with forbidNonWhitelisted, so an unknown body field is a 400).
var walletRoutes = map[string]Route{
	// Ping and programs
	"ping":                {Method: "GET", Path: "/api/v1/wallet/ping"},
	"listMyPrograms":      {Method: "GET", Path: "/api/v1/wallet/programs/me"},
	"setCollectionAnchor": {Method: "PUT", Path: "/api/v1/wallet/programs/:programId/collection-anchor", Optional: []string{"collectionEndUserId", "reason", "expectedVersion"}},

	// End users
	"createEndUser":          {Method: "POST", Path: "/api/v1/wallet/end-users", IdempotencyKey: true, Required: []string{"programId", "accountType", "phone"}, Optional: []string{"externalCustomerId", "kycRef", "kycLevelCache", "displayName", "corporateAccountId", "defaultCurrency"}},
	"listEndUsers":           {Method: "GET", Path: "/api/v1/wallet/end-users", Query: []string{"programId", "accountType", "status", "corporateAccountId", "merchantCategoryState", "limit", "cursor"}},
	"getEndUserByExternalId": {Method: "GET", Path: "/api/v1/wallet/end-users/by-external/:externalCustomerId", Query: []string{"programId"}},
	"getEndUser":             {Method: "GET", Path: "/api/v1/wallet/end-users/:endUserId"},
	"freezeEndUser":          {Method: "POST", Path: "/api/v1/wallet/end-users/:endUserId/freeze", IdempotencyKey: true, Optional: []string{"reason"}},
	"getBalances":            {Method: "GET", Path: "/api/v1/wallet/end-users/:endUserId/balances"},
	"getKycLevel":            {Method: "GET", Path: "/api/v1/wallet/end-users/:endUserId/kyc-level"},
	"updateKycLevel":         {Method: "PUT", Path: "/api/v1/wallet/end-users/:endUserId/kyc-level", IdempotencyKey: true, Required: []string{"kycLevel", "sourceRef"}, Optional: []string{"externalCustomerId", "programId", "reason", "effectiveAt", "expectedCurrentLevel"}},
	"getLimits":              {Method: "GET", Path: "/api/v1/wallet/end-users/:endUserId/limits", Query: []string{"operationType", "currency"}},
	"createWallet":           {Method: "POST", Path: "/api/v1/wallet/end-users/:endUserId/wallets", IdempotencyKey: true, Optional: []string{"name", "currencies"}},

	// Wallets and accounts
	"createWalletAccount": {Method: "POST", Path: "/api/v1/wallet/wallets/:walletId/accounts", IdempotencyKey: true, Required: []string{"currency"}},
	"lookupWallet":        {Method: "POST", Path: "/api/v1/wallet/wallets/lookup", Optional: []string{"programId", "phone", "walletNo"}},

	// Deposits (bank transfer in)
	"listDepositInstructions":  {Method: "GET", Path: "/api/v1/wallet/end-users/:endUserId/deposit-instructions"},
	"createDepositInstruction": {Method: "POST", Path: "/api/v1/wallet/end-users/:endUserId/deposit-instructions", Required: []string{"walletAccountId"}, Optional: []string{"currency"}},
	"simulateInboundCredit":    {Method: "POST", Path: "/api/v1/wallet/bank-integrations/inbound-credits", Required: []string{"programId", "railId", "amount", "currency", "rawReference"}, Optional: []string{"senderName", "senderIban", "creditedIban", "receiptNumber"}},

	// Corporate accounts
	"createCorporateAccount":     {Method: "POST", Path: "/api/v1/wallet/corporate-accounts", IdempotencyKey: true, Required: []string{"programId", "code", "name", "ownerEndUserId"}, Optional: []string{"fundingCurrency"}},
	"listCorporateAccounts":      {Method: "GET", Path: "/api/v1/wallet/corporate-accounts", Query: []string{"programId", "status", "limit", "cursor"}},
	"importCorporateEmployees":   {Method: "POST", Path: "/api/v1/wallet/corporate-accounts/:corporateAccountId/employees/import", IdempotencyKey: true, Required: []string{"rows"}},
	"offboardCorporateEmployees": {Method: "POST", Path: "/api/v1/wallet/corporate-accounts/:corporateAccountId/employees/offboard", IdempotencyKey: true, Required: []string{"rows"}, Optional: []string{"reason"}},

	// Fees
	"quoteFees": {Method: "POST", Path: "/api/v1/wallet/fees/quote", Required: []string{"programId", "operationType", "currency", "amount"}, Optional: []string{"at"}},

	// Wallet-to-wallet transfers
	"quoteW2W":    {Method: "POST", Path: "/api/v1/wallet/transfers/w2w/quote", Required: []string{"endUserId", "sourceWalletAccountId", "amount", "currency"}, Optional: []string{"destinationWalletNo", "destinationPhone", "paymentPurpose"}},
	"initiateW2W": {Method: "POST", Path: "/api/v1/wallet/transfers/w2w", IdempotencyKey: true, Required: []string{"endUserId", "sourceWalletAccountId", "amount", "currency"}, Optional: []string{"destinationWalletNo", "destinationPhone", "paymentPurpose", "scheduleVersionId", "otpDestination", "otpChannel", "locale"}},
	"completeW2W": {Method: "POST", Path: "/api/v1/wallet/transfers/w2w/:operationId/complete", IdempotencyKey: true, Required: []string{"endUserId", "operationId", "integrityHash", "nonce"}, Optional: []string{"otpCode", "trustedActionToken"}},

	// Transfers to an IBAN
	"quoteW2Iban":    {Method: "POST", Path: "/api/v1/wallet/transfers/w2iban/quote", Required: []string{"endUserId", "sourceWalletAccountId", "beneficiaryName", "iban", "amount", "currency"}, Optional: []string{"bankCode", "paymentPurpose"}},
	"initiateW2Iban": {Method: "POST", Path: "/api/v1/wallet/transfers/w2iban", IdempotencyKey: true, Required: []string{"endUserId", "sourceWalletAccountId", "beneficiaryName", "iban", "amount", "currency"}, Optional: []string{"bankCode", "paymentPurpose", "scheduleVersionId", "otpDestination", "otpChannel", "locale"}},
	"completeW2Iban": {Method: "POST", Path: "/api/v1/wallet/transfers/w2iban/:operationId/complete", IdempotencyKey: true, Required: []string{"endUserId", "operationId", "integrityHash", "nonce"}, Optional: []string{"otpCode", "trustedActionToken"}},

	// Withdrawals to the end user's own bank account
	"quoteBankWithdrawal":    {Method: "POST", Path: "/api/v1/wallet/withdrawals/bank/quote", Required: []string{"endUserId", "sourceWalletAccountId", "beneficiaryName", "iban", "amount", "currency"}, Optional: []string{"bankCode", "paymentPurpose"}},
	"initiateBankWithdrawal": {Method: "POST", Path: "/api/v1/wallet/withdrawals/bank", IdempotencyKey: true, Required: []string{"endUserId", "sourceWalletAccountId", "beneficiaryName", "iban", "amount", "currency"}, Optional: []string{"bankCode", "paymentPurpose", "scheduleVersionId", "otpDestination", "otpChannel", "locale"}},
	"completeBankWithdrawal": {Method: "POST", Path: "/api/v1/wallet/withdrawals/bank/:operationId/complete", IdempotencyKey: true, Required: []string{"endUserId", "operationId", "integrityHash", "nonce"}, Optional: []string{"otpCode", "trustedActionToken"}},
	"getWithdrawal":          {Method: "GET", Path: "/api/v1/wallet/withdrawals/:operationId"},

	// Operations
	"listOperations":      {Method: "GET", Path: "/api/v1/wallet/operations", Query: []string{"endUserId", "walletAccountId", "status", "operationType", "destinationWalletAccountId", "limit", "cursor"}},
	"getOperation":        {Method: "GET", Path: "/api/v1/wallet/operations/:operationId"},
	"getOperationReceipt": {Method: "GET", Path: "/api/v1/wallet/operations/:operationId/receipt"},

	// Top-ups
	"startCardTopup": {Method: "POST", Path: "/api/v1/wallet/topups/card", IdempotencyKey: true, Required: []string{"creditWalletAccountId", "amount", "currency", "buyer", "billingAddress"}, Optional: []string{"fundingMerchantId", "savedCardUserKey", "savedCardToken", "savedConsumerToken", "savedPublicCardStorageToken", "returnUrl", "shippingAddress"}},
	"getTopup":       {Method: "GET", Path: "/api/v1/wallet/topups/:operationId"},
	"refundTopup":    {Method: "POST", Path: "/api/v1/wallet/topups/:operationId/refund", Required: []string{"reason", "requesterPrincipal", "approverPrincipal"}},
	"agentTopup":     {Method: "POST", Path: "/api/v1/wallet/agent-topups", IdempotencyKey: true, Required: []string{"agentId", "creditWalletAccountId", "amount", "currency"}},

	// Payments to the merchant (payment request, payment code)
	"requestPayment":        {Method: "POST", Path: "/api/v1/wallet/payments/buy", IdempotencyKey: true, Required: []string{"amount", "currency", "paymentPurpose"}, Optional: []string{"buyerWalletNo", "buyerPhone", "orderRef", "cashierRef", "otpDestination", "otpChannel", "locale"}},
	"approvePayment":        {Method: "POST", Path: "/api/v1/wallet/payments/:operationId/approve", IdempotencyKey: true, Required: []string{"buyerEndUserId", "integrityHash", "nonce"}, Optional: []string{"otpCode", "trustedActionToken"}},
	"collectByCode":         {Method: "POST", Path: "/api/v1/wallet/collect", IdempotencyKey: true, Required: []string{"code", "amount", "currency"}, Optional: []string{"orderRef", "paymentPurpose", "locale"}},
	"getPayment":            {Method: "GET", Path: "/api/v1/wallet/payments/:operationId"},
	"getBusinessCommission": {Method: "GET", Path: "/api/v1/wallet/business-accounts/:businessAccountId/commission"},

	// QR codes
	"createQr":   {Method: "POST", Path: "/api/v1/wallet/qr", IdempotencyKey: true, Required: []string{"qrType", "currency"}, Optional: []string{"targetEndUserId", "targetExternalCustomerId", "programId", "amount", "expiresInSeconds", "metadata"}},
	"listQr":     {Method: "GET", Path: "/api/v1/wallet/qr", Query: []string{"programId", "qrType", "status", "targetEndUserId", "limit", "cursor"}},
	"getQr":      {Method: "GET", Path: "/api/v1/wallet/qr/:qrId"},
	"getQrImage": {Method: "GET", Path: "/api/v1/wallet/qr/:qrId/image", Query: []string{"format", "scale"}, Binary: true},
	"revokeQr":   {Method: "POST", Path: "/api/v1/wallet/qr/:qrId/revoke"},
	"parseQr":    {Method: "POST", Path: "/api/v1/wallet/qr/parse", Required: []string{"payload"}},

	// Bulk payouts
	"createBulkPayout":      {Method: "POST", Path: "/api/v1/wallet/bulk-payouts", IdempotencyKey: true, Required: []string{"programId", "sourceWalletAccountId", "currency", "items"}, Optional: []string{"reason"}},
	"listBulkPayouts":       {Method: "GET", Path: "/api/v1/wallet/bulk-payouts", Query: []string{"programId", "status", "limit"}},
	"getBulkPayout":         {Method: "GET", Path: "/api/v1/wallet/bulk-payouts/:batchId"},
	"listBulkPayoutItems":   {Method: "GET", Path: "/api/v1/wallet/bulk-payouts/:batchId/items", Query: []string{"status", "limit"}},
	"submitBulkPayout":      {Method: "POST", Path: "/api/v1/wallet/bulk-payouts/:batchId/submit"},
	"approveBulkPayout":     {Method: "POST", Path: "/api/v1/wallet/bulk-payouts/:batchId/approve", Required: []string{"approve"}, Optional: []string{"reason"}},
	"rejectBulkPayout":      {Method: "POST", Path: "/api/v1/wallet/bulk-payouts/:batchId/reject", Optional: []string{"reason"}},
	"processBulkPayout":     {Method: "POST", Path: "/api/v1/wallet/bulk-payouts/:batchId/process"},
	"retryFailedBulkPayout": {Method: "POST", Path: "/api/v1/wallet/bulk-payouts/:batchId/retry-failed"},
}

// walletNestedFields are the fields of the rows / items the gateway
// validates element by element.
var walletNestedFields = map[string]NestedFields{
	"importCorporateEmployees":   {Required: []string{"phone"}, Optional: []string{"displayName", "externalCustomerId", "kycRef"}},
	"offboardCorporateEmployees": {Required: []string{}, Optional: []string{"endUserId", "externalCustomerId"}},
	"createBulkPayout":           {Required: []string{"targetType", "targetRef", "amount"}, Optional: []string{"beneficiaryName"}},
}

// WalletRoutes returns a copy of the wallet route table (route name →
// Route), the same table as the other Lunixi SDKs.
func WalletRoutes() map[string]Route { return cloneRoutes(walletRoutes) }

// WalletNestedFields returns a copy of the row fields of the list bodies
// (importCorporateEmployees and offboardCorporateEmployees rows,
// createBulkPayout items).
func WalletNestedFields() map[string]NestedFields { return cloneNested(walletNestedFields) }

// WalletAccountTypes lists the account types in wire order.
func WalletAccountTypes() []string {
	return []string{WalletAccountTypePersonal, WalletAccountTypeBusiness}
}

// WalletEndUserStatuses lists the end-user statuses in wire order.
func WalletEndUserStatuses() []string {
	return []string{WalletEndUserStatusActive, WalletEndUserStatusFrozen, WalletEndUserStatusClosed}
}

// WalletKycLevels lists the KYC levels in wire order.
func WalletKycLevels() []string {
	return []string{WalletKycLevelNone, WalletKycLevelLow, WalletKycLevelSubstantial, WalletKycLevelHigh}
}

// WalletOtpChannels lists the one-time code channels in wire order.
func WalletOtpChannels() []string {
	return []string{WalletOtpChannelEmail, WalletOtpChannelSMS, WalletOtpChannelPush}
}

// WalletQRTypes lists the QR code types in wire order.
func WalletQRTypes() []string {
	return []string{WalletQRTypeP2PStatic, WalletQRTypeP2PDynamic, WalletQRTypeMerchantStatic, WalletQRTypeMerchantDynamic}
}

// WalletQRImageFormats lists the QR image formats in wire order.
func WalletQRImageFormats() []string { return []string{WalletQRImageFormatSVG, WalletQRImageFormatPNG} }

// WalletBulkTargetTypes lists the bulk payout target types in wire order.
func WalletBulkTargetTypes() []string {
	return []string{WalletBulkTargetTypeWallet, WalletBulkTargetTypeIBAN}
}

// WalletCorporateAccountStatuses lists the corporate account statuses in wire order.
func WalletCorporateAccountStatuses() []string {
	return []string{WalletCorporateAccountStatusActive, WalletCorporateAccountStatusArchived}
}

const walletBase = "/api/v1/wallet"

func walletRoute(path string) bool {
	return path == walletBase || strings.HasPrefix(path, walletBase+"/")
}

// validWalletIdempotency: 1–128 printable ASCII characters without spaces.
func validWalletIdempotency(key string) bool { return safeHeader(key, WalletIdempotencyKeyMax) }

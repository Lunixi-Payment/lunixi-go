package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"maps"
	"net/url"
	"slices"
	"strconv"
)

// WalletClient — the merchant server-to-server surface of the closed-loop
// wallet (/api/v1/wallet/*, product WALLET_SERVICE).
//
// Every call is signed; the merchant, its programs and its end users come
// from the signing key. The key also needs the wallet permission behind the
// route (otherwise 403 WALLET_INSUFFICIENT_SCOPE): wallet:*:read for reads,
// wallet:program|enduser|limit|fee:manage for set-up,
// wallet:operation:intervene for moving money inside the wallet,
// wallet:bank-payout:operate for IBAN transfers and bank withdrawals,
// wallet:qr:manage, wallet:bulk:create, wallet:bulk:approve and
// wallet:enduser:kyc-override for the KYC level.
//
// Requests and filters are maps with the gateway's field names; they are
// checked against WalletRoutes before anything is sent: an unknown field, a
// missing required field, an amount that is not minor-unit digit text, or a
// missing Idempotency-Key returns ErrConfiguration. Amounts are minor units
// as text ("12550" is 125.50 TRY). A call that moves money for an end user
// names that end user (endUserId, buyerEndUserId); the source account must
// belong to them or the answer is 404 WALLET_NOT_FOUND.
//
// Single records are returned as the envelope's data; lists as CursorPage.
type WalletClient struct {
	client      *Client
	Programs    *WalletPrograms
	EndUsers    *WalletEndUsers
	Wallets     *WalletAccounts
	Deposits    *WalletDeposits
	Corporate   *WalletCorporate
	Fees        *WalletFees
	Transfers   *WalletTransfers
	Withdrawals *WalletWithdrawals
	Operations  *WalletOperations
	Topups      *WalletTopups
	Payments    *WalletPayments
	QR          *WalletQR
	BulkPayouts *WalletBulkPayouts
}

func newWalletClient(c *Client) *WalletClient {
	w := &WalletClient{client: c}
	w.Programs = &WalletPrograms{wallet: w}
	w.EndUsers = &WalletEndUsers{wallet: w}
	w.Wallets = &WalletAccounts{wallet: w}
	w.Deposits = &WalletDeposits{wallet: w}
	w.Corporate = &WalletCorporate{wallet: w}
	w.Fees = &WalletFees{wallet: w}
	w.Transfers = &WalletTransfers{wallet: w}
	w.Withdrawals = &WalletWithdrawals{wallet: w}
	w.Operations = &WalletOperations{wallet: w}
	w.Topups = &WalletTopups{wallet: w}
	w.Payments = &WalletPayments{wallet: w}
	w.QR = &WalletQR{wallet: w}
	w.BulkPayouts = &WalletBulkPayouts{wallet: w}
	return w
}

// Ping checks that the key reaches the wallet (signature, product and a
// wallet:*:manage permission).
func (w *WalletClient) Ping(ctx context.Context) (json.RawMessage, error) {
	return w.data(ctx, "ping", nil, nil, nil, "")
}

// WalletPrograms — the wallet programs of the signing merchant.
type WalletPrograms struct{ wallet *WalletClient }

// ListMine lists the wallet programs of the signing merchant.
func (p *WalletPrograms) ListMine(ctx context.Context) (*CursorPage, error) {
	return p.wallet.page(ctx, "listMyPrograms", nil, nil)
}

// SetCollectionAnchor sets the end user whose wallet collects the program's
// merchant payments (collectionEndUserId, reason, expectedVersion). Leaving
// collectionEndUserId out clears the anchor.
func (p *WalletPrograms) SetCollectionAnchor(ctx context.Context, programID string, request map[string]any) (json.RawMessage, error) {
	return p.wallet.data(ctx, "setCollectionAnchor", map[string]string{"programId": programID}, nil, orEmpty(request), "")
}

// WalletEndUsers — end users and their wallets, balances, limits and KYC
// level.
type WalletEndUsers struct{ wallet *WalletClient }

// Create creates an end user and their default wallet (programId,
// accountType, phone, optional externalCustomerId, kycRef, kycLevelCache,
// displayName, corporateAccountId, defaultCurrency). The key is required.
func (e *WalletEndUsers) Create(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "createEndUser", nil, nil, orEmpty(request), idempotencyKey)
}

// List lists end users. Filters: programId, accountType, status,
// corporateAccountId, merchantCategoryState, limit, cursor.
func (e *WalletEndUsers) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return e.wallet.page(ctx, "listEndUsers", nil, orEmpty(filters))
}

// Iterate yields every end user matching the filters, page by page.
func (e *WalletEndUsers) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, e.List)
}

// Get reads one end user.
func (e *WalletEndUsers) Get(ctx context.Context, endUserID string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "getEndUser", map[string]string{"endUserId": endUserID}, nil, nil, "")
}

// GetByExternalID finds an end user by your own customer id; programID ""
// searches every program.
func (e *WalletEndUsers) GetByExternalID(ctx context.Context, externalCustomerID, programID string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "getEndUserByExternalId", map[string]string{"externalCustomerId": externalCustomerID}, map[string]any{"programId": programID}, nil, "")
}

// Freeze freezes an end user; reason "" sends none. The key is required.
func (e *WalletEndUsers) Freeze(ctx context.Context, endUserID, reason, idempotencyKey string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "freezeEndUser", map[string]string{"endUserId": endUserID}, nil, optionalText(map[string]any{}, "reason", reason), idempotencyKey)
}

// GetBalances reads the balances of an end user's accounts.
func (e *WalletEndUsers) GetBalances(ctx context.Context, endUserID string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "getBalances", map[string]string{"endUserId": endUserID}, nil, nil, "")
}

// GetKycLevel reads the end user's KYC level.
func (e *WalletEndUsers) GetKycLevel(ctx context.Context, endUserID string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "getKycLevel", map[string]string{"endUserId": endUserID}, nil, nil, "")
}

// UpdateKycLevel records the KYC level from your own verification (kycLevel,
// sourceRef = your KYC session id, optional externalCustomerId, programId,
// reason, effectiveAt, expectedCurrentLevel). The key is required.
func (e *WalletEndUsers) UpdateKycLevel(ctx context.Context, endUserID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "updateKycLevel", map[string]string{"endUserId": endUserID}, nil, orEmpty(request), idempotencyKey)
}

// GetLimits reads limit usage per operation type; the operationType and
// currency filters narrow the answer.
func (e *WalletEndUsers) GetLimits(ctx context.Context, endUserID string, filters map[string]any) (json.RawMessage, error) {
	return e.wallet.data(ctx, "getLimits", map[string]string{"endUserId": endUserID}, orEmpty(filters), nil, "")
}

// CreateWallet adds a wallet to an end user (optional name, currencies). The
// key is required.
func (e *WalletEndUsers) CreateWallet(ctx context.Context, endUserID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return e.wallet.data(ctx, "createWallet", map[string]string{"endUserId": endUserID}, nil, orEmpty(request), idempotencyKey)
}

// WalletAccounts — wallets and their currency accounts.
type WalletAccounts struct{ wallet *WalletClient }

// CreateAccount adds a currency account to a wallet. The key is required.
func (a *WalletAccounts) CreateAccount(ctx context.Context, walletID, currency, idempotencyKey string) (json.RawMessage, error) {
	return a.wallet.data(ctx, "createWalletAccount", map[string]string{"walletId": walletID}, nil, map[string]any{"currency": currency}, idempotencyKey)
}

// Lookup resolves a wallet number or phone (programId, phone, walletNo) to
// {exists, walletNo, maskedName} before a transfer.
func (a *WalletAccounts) Lookup(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return a.wallet.data(ctx, "lookupWallet", nil, nil, orEmpty(request), "")
}

// WalletDeposits — bank transfer details the end user pays into to top up.
type WalletDeposits struct{ wallet *WalletClient }

// ListInstructions lists the end user's deposit instructions.
func (d *WalletDeposits) ListInstructions(ctx context.Context, endUserID string) (json.RawMessage, error) {
	return d.wallet.data(ctx, "listDepositInstructions", map[string]string{"endUserId": endUserID}, nil, nil, "")
}

// CreateInstruction creates a deposit instruction (walletAccountId, optional
// currency).
func (d *WalletDeposits) CreateInstruction(ctx context.Context, endUserID string, request map[string]any) (json.RawMessage, error) {
	return d.wallet.data(ctx, "createDepositInstruction", map[string]string{"endUserId": endUserID}, nil, orEmpty(request), "")
}

// SimulateInboundCredit books an incoming bank transfer as the bank rail
// would. Test environment only.
func (d *WalletDeposits) SimulateInboundCredit(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return d.wallet.data(ctx, "simulateInboundCredit", nil, nil, orEmpty(request), "")
}

// WalletCorporate — corporate accounts and their employees.
type WalletCorporate struct{ wallet *WalletClient }

// Create creates a corporate account (programId, code, name,
// ownerEndUserId, optional fundingCurrency). The key is required.
func (c *WalletCorporate) Create(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return c.wallet.data(ctx, "createCorporateAccount", nil, nil, orEmpty(request), idempotencyKey)
}

// List lists corporate accounts. Filters: programId, status, limit, cursor.
func (c *WalletCorporate) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return c.wallet.page(ctx, "listCorporateAccounts", nil, orEmpty(filters))
}

// Iterate yields every corporate account matching the filters.
func (c *WalletCorporate) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, c.List)
}

// ImportEmployees imports up to 1000 employees at once ({phone, displayName,
// externalCustomerId, kycRef} rows); all or nothing (a 422 writes nothing).
// The key is required.
func (c *WalletCorporate) ImportEmployees(ctx context.Context, corporateAccountID string, rows []map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return c.wallet.data(ctx, "importCorporateEmployees", map[string]string{"corporateAccountId": corporateAccountID}, nil, map[string]any{"rows": rows}, idempotencyKey)
}

// OffboardEmployees offboards employees ({endUserId} or
// {externalCustomerId} rows); reason "" sends none. The key is required.
func (c *WalletCorporate) OffboardEmployees(ctx context.Context, corporateAccountID string, rows []map[string]any, reason, idempotencyKey string) (json.RawMessage, error) {
	return c.wallet.data(ctx, "offboardCorporateEmployees", map[string]string{"corporateAccountId": corporateAccountID}, nil, optionalText(map[string]any{"rows": rows}, "reason", reason), idempotencyKey)
}

// WalletFees — fee quotes.
type WalletFees struct{ wallet *WalletClient }

// Quote returns the fee, tax and total charge an operation would cost
// (programId, operationType, currency, amount, optional at).
func (f *WalletFees) Quote(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return f.wallet.data(ctx, "quoteFees", nil, nil, orEmpty(request), "")
}

// WalletTransfers — wallet-to-wallet transfers and transfers to an IBAN.
//
// Money moves in two steps: Initiate… reserves the limit and returns
// {operationId, integrityHash, nonce, otpRequired, …}; Complete… with the
// same integrityHash and nonce (and otpCode when otpRequired) commits it.
// Quote first to show the fee; pass its scheduleVersionId to Initiate… to
// hold that fee. Complete… puts operationId into the body itself.
type WalletTransfers struct{ wallet *WalletClient }

// QuoteW2W quotes a wallet-to-wallet transfer.
func (t *WalletTransfers) QuoteW2W(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return t.wallet.data(ctx, "quoteW2W", nil, nil, orEmpty(request), "")
}

// InitiateW2W starts a wallet-to-wallet transfer. The key is required.
func (t *WalletTransfers) InitiateW2W(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return t.wallet.data(ctx, "initiateW2W", nil, nil, orEmpty(request), idempotencyKey)
}

// CompleteW2W commits a wallet-to-wallet transfer (endUserId,
// integrityHash, nonce, optional otpCode, trustedActionToken). The key is
// required.
func (t *WalletTransfers) CompleteW2W(ctx context.Context, operationID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return t.wallet.complete(ctx, "completeW2W", operationID, request, idempotencyKey)
}

// QuoteW2Iban quotes a transfer to an IBAN.
func (t *WalletTransfers) QuoteW2Iban(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return t.wallet.data(ctx, "quoteW2Iban", nil, nil, orEmpty(request), "")
}

// InitiateW2Iban starts a transfer to an IBAN. The key is required.
func (t *WalletTransfers) InitiateW2Iban(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return t.wallet.data(ctx, "initiateW2Iban", nil, nil, orEmpty(request), idempotencyKey)
}

// CompleteW2Iban commits a transfer to an IBAN. The key is required.
func (t *WalletTransfers) CompleteW2Iban(ctx context.Context, operationID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return t.wallet.complete(ctx, "completeW2Iban", operationID, request, idempotencyKey)
}

// WalletWithdrawals — withdrawals to the end user's own bank account (the
// same two steps as transfers).
type WalletWithdrawals struct{ wallet *WalletClient }

// Quote quotes a bank withdrawal.
func (w *WalletWithdrawals) Quote(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return w.wallet.data(ctx, "quoteBankWithdrawal", nil, nil, orEmpty(request), "")
}

// Initiate starts a bank withdrawal. The key is required.
func (w *WalletWithdrawals) Initiate(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return w.wallet.data(ctx, "initiateBankWithdrawal", nil, nil, orEmpty(request), idempotencyKey)
}

// Complete commits a bank withdrawal. The key is required.
func (w *WalletWithdrawals) Complete(ctx context.Context, operationID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return w.wallet.complete(ctx, "completeBankWithdrawal", operationID, request, idempotencyKey)
}

// Get reads a withdrawal: {operation, history}.
func (w *WalletWithdrawals) Get(ctx context.Context, operationID string) (json.RawMessage, error) {
	return w.wallet.data(ctx, "getWithdrawal", map[string]string{"operationId": operationID}, nil, nil, "")
}

// WalletOperations — every money movement of the wallet.
type WalletOperations struct{ wallet *WalletClient }

// List lists operations. Filters: endUserId, walletAccountId, status,
// operationType, destinationWalletAccountId, limit, cursor.
func (o *WalletOperations) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return o.wallet.page(ctx, "listOperations", nil, orEmpty(filters))
}

// Iterate yields every operation matching the filters.
func (o *WalletOperations) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, o.List)
}

// Get reads an operation: {operation, history}.
func (o *WalletOperations) Get(ctx context.Context, operationID string) (json.RawMessage, error) {
	return o.wallet.data(ctx, "getOperation", map[string]string{"operationId": operationID}, nil, nil, "")
}

// GetReceipt reads the receipt of an operation.
func (o *WalletOperations) GetReceipt(ctx context.Context, operationID string) (json.RawMessage, error) {
	return o.wallet.data(ctx, "getOperationReceipt", map[string]string{"operationId": operationID}, nil, nil, "")
}

// WalletTopups — card and agent top-ups.
type WalletTopups struct{ wallet *WalletClient }

// StartCard starts a card top-up. Render checkoutFormContent (or follow
// redirectUrl / threeDsHtml); the wallet is credited when the payment
// completes (wallet.topup.completed). The key is required.
//
// The payment provider requires the buyer and the billing address (buyer
// with BuyerRequiredFields, ip being the user's browser IP; billingAddress
// with AddressRequiredFields); shippingAddress is optional. Missing paths
// fail with ErrConfiguration before sending; the gateway answers them with
// 400 OrderFieldErrorCodeFieldRequired and the paths in details.fields.
func (t *WalletTopups) StartCard(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return t.wallet.data(ctx, "startCardTopup", nil, nil, orEmpty(request), idempotencyKey)
}

// Get reads a top-up: {operation, history}.
func (t *WalletTopups) Get(ctx context.Context, operationID string) (json.RawMessage, error) {
	return t.wallet.data(ctx, "getTopup", map[string]string{"operationId": operationID}, nil, nil, "")
}

// Refund refunds a card top-up to the card (reason, requesterPrincipal,
// approverPrincipal). Four eyes: approverPrincipal is required and must
// differ from requesterPrincipal; binding them to two real people is your
// approval flow's job.
func (t *WalletTopups) Refund(ctx context.Context, operationID string, request map[string]any) (json.RawMessage, error) {
	return t.wallet.data(ctx, "refundTopup", map[string]string{"operationId": operationID}, nil, orEmpty(request), "")
}

// Agent books a cash top-up through an agent of the program (agentId,
// creditWalletAccountId, amount, currency). The key is required.
func (t *WalletTopups) Agent(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return t.wallet.data(ctx, "agentTopup", nil, nil, orEmpty(request), idempotencyKey)
}

// WalletPayments — the end user pays the merchant. The creditor is the
// wallet terminal bound to the signing key (WALLET_TERMINAL_* 403 without
// one); a businessAccountId in the body is refused.
type WalletPayments struct{ wallet *WalletClient }

// Request asks the buyer (by wallet number or phone) to pay; approve it with
// Approve. The key is required.
func (p *WalletPayments) Request(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return p.wallet.data(ctx, "requestPayment", nil, nil, orEmpty(request), idempotencyKey)
}

// Approve approves a payment request (buyerEndUserId, integrityHash, nonce,
// optional otpCode, trustedActionToken). The key is required.
func (p *WalletPayments) Approve(ctx context.Context, operationID string, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return p.wallet.data(ctx, "approvePayment", map[string]string{"operationId": operationID}, nil, orEmpty(request), idempotencyKey)
}

// Collect collects with the payment code the buyer shows (code, amount,
// currency, optional orderRef, paymentPurpose, locale). The key is required.
func (p *WalletPayments) Collect(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return p.wallet.data(ctx, "collectByCode", nil, nil, orEmpty(request), idempotencyKey)
}

// Get reads a payment: {operation, history}.
func (p *WalletPayments) Get(ctx context.Context, operationID string) (json.RawMessage, error) {
	return p.wallet.data(ctx, "getPayment", map[string]string{"operationId": operationID}, nil, nil, "")
}

// GetCommission reads the commission of a business account.
func (p *WalletPayments) GetCommission(ctx context.Context, businessAccountID string) (json.RawMessage, error) {
	return p.wallet.data(ctx, "getBusinessCommission", map[string]string{"businessAccountId": businessAccountID}, nil, nil, "")
}

// WalletQR — wallet QR codes.
type WalletQR struct{ wallet *WalletClient }

// Create creates a QR code (qrType, currency, optional targetEndUserId,
// targetExternalCustomerId, programId, amount, expiresInSeconds, metadata).
// Static codes carry no amount; dynamic ones require it. The key is
// required.
func (q *WalletQR) Create(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return q.wallet.data(ctx, "createQr", nil, nil, orEmpty(request), idempotencyKey)
}

// List lists QR codes. Filters: programId, qrType, status, targetEndUserId,
// limit, cursor.
func (q *WalletQR) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return q.wallet.page(ctx, "listQr", nil, orEmpty(filters))
}

// Iterate yields every QR code matching the filters.
func (q *WalletQR) Iterate(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, q.List)
}

// Get reads one QR code.
func (q *WalletQR) Get(ctx context.Context, qrID string) (json.RawMessage, error) {
	return q.wallet.data(ctx, "getQr", map[string]string{"qrId": qrID}, nil, nil, "")
}

// Image returns the QR code as an image in Response.Bytes
// (Response.ContentType says which). format is WalletQRImageFormatSVG (the
// gateway's default) or WalletQRImageFormatPNG, "" for the default; scale is
// 1–20, 0 for the default. Error answers are still read as the JSON envelope.
func (q *WalletQR) Image(ctx context.Context, qrID, format string, scale int) (*Response, error) {
	query := map[string]any{}
	if format != "" {
		if err := oneOf(format, WalletQRImageFormats(), "QR image format"); err != nil {
			return nil, err
		}
		query["format"] = format
	}
	if scale != 0 {
		if scale < 1 || scale > 20 {
			return nil, invalid("QR image scale must be an integer 1-20")
		}
		query["scale"] = strconv.Itoa(scale)
	}
	return q.wallet.call(ctx, "getQrImage", map[string]string{"qrId": qrID}, query, nil, "")
}

// Revoke revokes a QR code.
func (q *WalletQR) Revoke(ctx context.Context, qrID string) (json.RawMessage, error) {
	return q.wallet.data(ctx, "revokeQr", map[string]string{"qrId": qrID}, nil, nil, "")
}

// Parse decodes a scanned QR payload.
func (q *WalletQR) Parse(ctx context.Context, payload string) (json.RawMessage, error) {
	return q.wallet.data(ctx, "parseQr", nil, nil, map[string]any{"payload": payload}, "")
}

// WalletBulkPayouts — bulk payouts, maker-checker: the approver must be a
// DIFFERENT key than the creator (the principal is the signing key id).
// Creating needs wallet:bulk:create; submit, approve, process and
// retry-failed need wallet:bulk:approve — so the maker key holds both, the
// checker key holds wallet:bulk:approve.
type WalletBulkPayouts struct{ wallet *WalletClient }

// Create creates a bulk payout (programId, sourceWalletAccountId, currency,
// items [{targetType, targetRef, amount, beneficiaryName?}], optional
// reason). The key is required.
func (b *WalletBulkPayouts) Create(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "createBulkPayout", nil, nil, orEmpty(request), idempotencyKey)
}

// List lists bulk payouts. Filters: programId, status, limit.
func (b *WalletBulkPayouts) List(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return b.wallet.page(ctx, "listBulkPayouts", nil, orEmpty(filters))
}

// Get reads one bulk payout.
func (b *WalletBulkPayouts) Get(ctx context.Context, batchID string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "getBulkPayout", map[string]string{"batchId": batchID}, nil, nil, "")
}

// ListItems lists the items of a bulk payout. Filters: status, limit.
func (b *WalletBulkPayouts) ListItems(ctx context.Context, batchID string, filters map[string]any) (*CursorPage, error) {
	return b.wallet.page(ctx, "listBulkPayoutItems", map[string]string{"batchId": batchID}, orEmpty(filters))
}

// Submit submits a bulk payout for approval.
func (b *WalletBulkPayouts) Submit(ctx context.Context, batchID string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "submitBulkPayout", map[string]string{"batchId": batchID}, nil, nil, "")
}

// Approve approves a bulk payout; approve false declines it with reason.
// reason "" sends none.
func (b *WalletBulkPayouts) Approve(ctx context.Context, batchID string, approve bool, reason string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "approveBulkPayout", map[string]string{"batchId": batchID}, nil, optionalText(map[string]any{"approve": approve}, "reason", reason), "")
}

// Reject rejects a bulk payout; reason "" sends none.
func (b *WalletBulkPayouts) Reject(ctx context.Context, batchID, reason string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "rejectBulkPayout", map[string]string{"batchId": batchID}, nil, optionalText(map[string]any{}, "reason", reason), "")
}

// Process starts paying out an approved bulk payout.
func (b *WalletBulkPayouts) Process(ctx context.Context, batchID string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "processBulkPayout", map[string]string{"batchId": batchID}, nil, nil, "")
}

// RetryFailed retries the failed items of a bulk payout.
func (b *WalletBulkPayouts) RetryFailed(ctx context.Context, batchID string) (json.RawMessage, error) {
	return b.wallet.data(ctx, "retryFailedBulkPayout", map[string]string{"batchId": batchID}, nil, nil, "")
}

// ── Route invoker ────────────────────────────────────────────────────────

func (w *WalletClient) data(ctx context.Context, name string, params map[string]string, query, body map[string]any, key string) (json.RawMessage, error) {
	r, err := w.call(ctx, name, params, query, body, key)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (w *WalletClient) page(ctx context.Context, name string, params map[string]string, query map[string]any) (*CursorPage, error) {
	r, err := w.call(ctx, name, params, query, nil, "")
	if err != nil {
		return nil, err
	}
	return dataCursorPage(r, true), nil
}

// complete sends a Complete… route: the gateway's complete DTO requires
// operationId in the body as well (the path wins on the server), so it is
// added here; a different operationId in the body is refused.
func (w *WalletClient) complete(ctx context.Context, name, operationID string, request map[string]any, key string) (json.RawMessage, error) {
	body := maps.Clone(orEmpty(request))
	if given, ok := body["operationId"]; ok && given != nil && given != operationID {
		return nil, invalid("operationId in the body must match the operation being completed")
	}
	body["operationId"] = operationID
	return w.data(ctx, name, map[string]string{"operationId": operationID}, nil, body, key)
}

// call sends one route of WalletRoutes. The body and query are checked
// against the route before anything leaves the process.
func (w *WalletClient) call(ctx context.Context, name string, params map[string]string, query, body map[string]any, key string) (*Response, error) {
	route := walletRoutes[name]
	path, err := fillRoutePath(route.Path, params)
	if err != nil {
		return nil, err
	}
	var values url.Values
	if query != nil {
		if values, err = routeQuery(name, route.Query, query); err != nil {
			return nil, err
		}
	}
	request := Request{Method: route.Method, Path: path, Query: values}
	if route.Method != "GET" && (len(route.Required) > 0 || len(route.Optional) > 0) {
		payload, err := walletBody(name, route, body)
		if err != nil {
			return nil, err
		}
		request.Body = payload
	}
	if route.IdempotencyKey {
		if key == "" {
			return nil, invalid("a stable Idempotency-Key is required for " + name + ". Reuse the same key when retrying the same operation")
		}
		if !validWalletIdempotency(key) {
			return nil, invalid("the wallet Idempotency-Key must be 1–128 printable ASCII characters without spaces")
		}
		request.IdempotencyKey = key
	} else if key != "" {
		return nil, invalid(name + " does not take an Idempotency-Key")
	}
	if route.Binary {
		return w.client.do(ctx, request, walletQRImageAccept)
	}
	return w.client.Do(ctx, request)
}

// walletBody checks a body against its route: nil members are dropped; an
// unknown field, a missing required field (nil or ""), an amount that is not
// minor-unit digit text and the row rules of the list bodies fail. The body
// is sent in the route's field order.
func walletBody(name string, route Route, body map[string]any) (orderedObject, error) {
	object, err := plainObject(body, name+" body")
	if err != nil {
		return nil, err
	}
	plain := compactNil(object)
	if err = checkFieldNames(name, plain, route.Required, route.Optional, presentValue); err != nil {
		return nil, err
	}
	for _, field := range walletAmountFields {
		if value, ok := plain[field]; ok {
			if plain[field], err = walletAmount(name+"."+field, value); err != nil {
				return nil, err
			}
		}
	}
	if spec, ok := walletNestedFields[name]; ok {
		field := "rows"
		if name == "createBulkPayout" {
			field = "items"
		}
		if plain[field], err = walletRows(name+"."+field, spec, plain[field]); err != nil {
			return nil, err
		}
	}
	return orderedFields(plain, route.Fields()), nil
}

func walletRows(label string, spec NestedFields, value any) ([]any, error) {
	rows, ok := value.([]any)
	if !ok || len(rows) == 0 {
		return nil, invalid(label + " must be a non-empty array")
	}
	fields := append(slices.Clone(spec.Required), spec.Optional...)
	out := make([]any, 0, len(rows))
	for i, entry := range rows {
		rowLabel := label + "[" + strconv.Itoa(i) + "]"
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, invalid(rowLabel + " must be an object")
		}
		plain := compactNil(row)
		if err := checkFieldNames(rowLabel, plain, spec.Required, spec.Optional, presentValue); err != nil {
			return nil, err
		}
		if amount, ok := plain["amount"]; ok {
			text, err := walletAmount(rowLabel+".amount", amount)
			if err != nil {
				return nil, err
			}
			plain["amount"] = text
		}
		out = append(out, orderedFields(plain, fields))
	}
	return out, nil
}

// walletAmount: canonical positive minor units as digit text (at most 18
// digits); a number is refused rather than rounded.
func walletAmount(label string, value any) (string, error) {
	text, ok := value.(string)
	if !ok || !validMinor(text, true, false) || !walletAmountPattern.MatchString(text) {
		return "", invalid(label + ` must be minor units as digit text (e.g. "12550" for 125.50)`)
	}
	return text, nil
}

func orEmpty(object map[string]any) map[string]any {
	if object == nil {
		return map[string]any{}
	}
	return object
}

// optionalText sets field to value when value is not "".
func optionalText(object map[string]any, field, value string) map[string]any {
	if value != "" {
		object[field] = value
	}
	return object
}

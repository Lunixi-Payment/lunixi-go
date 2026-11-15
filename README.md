<p align="center">
  <a href="https://lunixi.com"><picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/lunixi-logo-dark.svg">
    <img src=".github/assets/lunixi-logo.svg" alt="Lunixi" height="64">
  </picture></a>
</p>

# Lunixi Go SDK

**Version 1.0.0 uses canonical minor-unit strings for money.** This is a breaking change from 0.x; update request and response handling together. See [migration](MIGRATION-1.0.md).

Version 1.0.0 includes a concrete merchant-site presentation example. **Embedded checkout is the default**: load only the official Lunixi bundle and mount the existing form; methods and multi-card choices stay in the Lunixi panel. Optional hosted mode uses the backend `paymentPageUrl` after exact HTTPS `pay.lunixi.com/payment/` validation. Its token may differ from the embedded token. Missing URLs keep embedded available and hosted on hold/reconciliation with the original attempt/key. Never derive a URL from the browser token. Authorize the customer/order and native browser CSRF before rendering, send `Cache-Control: private, no-store`, preserve the merchant CSP nonce and keep tokens/HTML/redirect URLs out of logs. Generic response data preserves `paymentPageUrl` (every product now has a typed client).

[![CI](https://github.com/Lunixi-Payment/lunixi-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Lunixi-Payment/lunixi-go/actions/workflows/ci.yml)
[![Go reference](https://pkg.go.dev/badge/github.com/Lunixi-Payment/lunixi-go.svg)](https://pkg.go.dev/github.com/Lunixi-Payment/lunixi-go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**[Documentation](https://docs.lunixi.com/sdks/go) · [Türkçe](docs/README.tr.md) · [Changelog](CHANGELOG.md) · [Security](SECURITY.md)**

Server-side Go client for Lunixi payments. Works with `net/http`, Gin, Echo, Fiber and other Go frameworks without a framework dependency. The SDK uses the Go standard library only. Go 1.26 or newer is required.

## Installation

Install the published immutable [v1.0.0 module](https://github.com/Lunixi-Payment/lunixi-go/tree/v1.0.0):

```sh
go get github.com/Lunixi-Payment/lunixi-go@v1.0.0
```

Keep `go.sum` and verify the module through the Go proxy and checksum database. The 0.x versions use the previous money contract.

## Create one client per merchant

```go
import (
    "os"
    "time"
    lunixi "github.com/Lunixi-Payment/lunixi-go"
)

client, err := lunixi.New(lunixi.Config{
    BaseURL:    "https://api-gateway.lunixi.com",
    KeyID:      os.Getenv("LUNIXI_KEY_ID"),
    PrivateKey: os.Getenv("LUNIXI_PRIVATE_KEY_PEM"),
    Timeout:    30 * time.Second,
    MaxRetries: 2,
})
if err != nil { return err }
```

Register the public key in your Lunixi dashboard. Keep its Ed25519 private key on your server or secret manager. PKCS8 PEM, base64 PKCS8 DER, base64 32-byte seed and a validated base64 64-byte Ed25519 private key are accepted. The merchant key selects TEST/LIVE; there is no client-side environment switch. Never put a LIVE key in a sample application.

The client is safe to share between goroutines. Token acquisition is lazy, cached with an expiry margin and coalesced during concurrent refresh. Merchant requests are signed, including GET requests. The subscription, KYC, identity and marketplace dealer routes are sent with the access token only, as the gateway authenticates them; the marketplace split charges stay signed. Canonicalization matches the gateway's JSON.stringify numeric-key order, compact UTF-8 and number format; unsafe JSON integers and duplicate keys are rejected.

| Setting | Default | Bounds / behavior |
|---|---|---|
| `Timeout` | 30 seconds | Up to 2 minutes per HTTP request; pass an overall context deadline |
| `MaxRetries` | 0 | 0–5; GET or a supplied stable idempotency key only |
| `MaxResponseBytes` | 8 MiB | Up to 32 MiB; request JSON is limited to 8 MiB |
| `HTTPClient` | Standard client | Copied; redirects always disabled; never use insecure TLS in production |
| `AllowInsecureLocalhost` | false | HTTP allowed only for explicit localhost / literal loopback testing |

`BaseURL` must be a trusted HTTPS origin, without credentials, path or query. Merchant API paths cannot change origin, traverse directories or call the token endpoint through `Do`.

## Start checkout

Load the basket and order from your server-side records. Persist the attempt key before calling, then reuse that key and body after a timeout.

```go
ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
defer cancel()

response, err := client.Payments.CreateIntent(ctx, lunixi.IntentRequest{
    Amount:      "10050", // 100.50 TRY, canonical minor-unit strings
    Currency:    "TRY",
    OrderID:     "ORD-1001",
    CallbackURL: "https://merchant.example/payment/return",
    BasketItems: []lunixi.BasketItem{
        {ID: "line-1", Name: "Example product", ItemType: "PHYSICAL", Price: "10050"},
    },
}, persistedAttemptKey)
if err != nil { return err }

var checkout struct { Token string `json:"token"` }
if err := response.DecodeData(&checkout); err != nil { return err }
if checkout.Token == "" { return errors.New("missing checkout token") }
// Send Token only to this order's authenticated customer, with
// Cache-Control: no-store. Initialize the existing Lunixi checkout form.
```

Amounts are `string` minor units, positive and no greater than `9223372036854775807`. Basket `Price` is the line total; line totals must equal `Amount`. Do not send the removed root fields `price`, `paidPrice`, `autoCapture`. Installment selection and fees belong to the Lunixi form; `EnabledInstallments` can only narrow the counts the form offers for this payment (for example `[]int{1, 2, 3}` for at most 3, `[]int{1}` for a single payment only). The form shows only the counts your panel settings, the card and the platform maximum also allow; nil means no restriction. Counts are 1–36, distinct and include 1, otherwise `CreateIntent` returns `ErrConfiguration` before any request; the list is sent in ascending order. Buyer/address and documented insurance basket details can be supplied on `IntentRequest`; optional method restrictions can narrow the methods already enabled in the Lunixi panel. `BasketItem.ProductCategory` (`productCategory`) is the optional Lunixi product category, used by payment methods with legal installment limits (e.g. shopping loan) and other platform features; use a `ProductCategory*` constant (`ProductCategories()` lists the codes known to this version). Empty omits the field and keeps today's behaviour; an unknown code is rejected by the API with HTTP 400. The dictionary only grows, so the field stays a plain string.

The existing Lunixi form handles cards and enabled alternative methods; this SDK does not build another form or collect PAN/CVV. **A checkout token, a browser return, HTTP 200 or an authorization is not proof of capture.** Finalize only from an authenticated gateway result after checking your stored order ID, merchant, payment ID, expected amount/currency and final capture/reconciliation state. Apply corresponding refund, void and dispute transitions separately.

## Payment lifecycle

```go
payment, err := client.Payments.Get(ctx, paymentID)
partial := int64(2500)
refund, err := client.Payments.Refund(ctx, paymentID, &partial, "Customer return", persistedRefundKey)
// nil amount requests the full capture/refund; Void releases an authorization.
```

| Helper | Coverage |
|---|---|
| `Payments` | `CreateIntent`, `Get`, `List`, `Methods`, `ListPaymentMethods`, `Capture`, `Refund`, `Void` |
| `Payments` methods | `DirectMethodPayment`, `ConfirmPaymentAction`, `ResendPaymentAction` |
| `Payments` promissory notes | `GetPromissoryNotes`, `CancelPromissoryNotes`, `DownloadPromissoryNoteDocument` |
| `Payments` bank transfer | `ListBankTransferExceptions`, `AcceptBankTransferException`, `MarkBankTransferExceptionRefunded` |
| `Payments` split | `Split`, `GetSplit`, `ListSplits`, `RefundSplit` |
| `Payments` cards | `BinInfo`, `InstallmentOptions`, `CardTaxonomy`, `StoreCard`, `ListStoredCards`, `GetStoredCard`, `DeactivateStoredCard` |
| `Payments` marketplace and reporting | `CreateMarketplaceIntent`, `FailoverRecoveryAnalytics`, `ProviderCredentialAnalytics` |
| `PaymentLinks` | `Create`, `List`, `Get`, `Update`, `Pause`, `Resume`, `Deactivate`, `Action`, `Send`, `QR`, `ListAttempts`, `ListPayments`, `CreateImageUpload`, `ConfirmImageUpload`, `UploadImage` |
| `Reference` | `ListProvinces`, `ListDistricts`, `ListNeighborhoods` |
| `Fraud` | `Evaluate`, `RecordEvent`, `RecordBatch`, `Decisions`, `Decision`, `LatestForPayment` |
| `Client.Do` | Signed requests to other documented `/api/v1/` merchant endpoints |
| `WebhookVerifier`, `SQLInbox` | Raw v2 verification and durable transactional deduplication |

`Split` accepts the documented gateway map and locally checks safe total/leg amounts, 1–5 legs and the leg sum. A card leg (`paymentMethod` empty or `CARD`) carries no method block; a non-card leg is a `MOBILE_BILLING`, `WALLET` or `PROMISSORY_NOTE` method with exactly its own block (`SplitLegBlockFields()`), including `providerCode`, and no card fields; a promissory-note leg gives `noteCount`, not a schedule. `MEAL_CARD`, `OPEN_BANKING`, `SHOPPING_LOAN` and `BANK_TRANSFER` cannot be legs. Other method-specific fields remain governed by the API schema. `ListSplits` takes `SplitListFilters` (status and currency lists, created range, order, owner, limit 1–100, `After` cursor). A pending split may return a `Response` with HTTP 400; inspect `StatusCode` and signed gateway status. `AUTHORIZED`, `AWAITING_*`, `COMPENSATING`, partial refunds and reconciliation-required states do not imply a fulfilled order. Root SDK responses preserve the original envelope, `Data`, `PageInfo` and binary `Bytes`; `DecodeData` uses `json.Number` for interface values.

Every product has a typed client (see the sections below). `Client.Do` remains for any other documented `/api/v1/` endpoint, with the same signed transport. For example:

```go
response, err := client.Do(ctx, lunixi.Request{
    Method: "GET",
    Path: "/api/v1/payments",
    Query: url.Values{"limit": {"20"}},
})
```

Retries keep the exact body and idempotency key and use a new signature nonce. Mutation helpers require 16–128 non-space printable ASCII characters as the key (8–200 for payment links). GET and keyed calls can retry transient network failures / 429 / 500 / 502 / 503 / 504 within the configured bound, with capped exponential delay; safe operations can refresh a rejected token once. Unkeyed mutations never automatically retry. Reconcile an unknown payment outcome before creating another attempt.

`APIError` provides `StatusCode`, `Code` and `RequestID`. Error strings omit upstream messages and request/response bodies. Responses can contain customer data or tokens; do not log `Response`, headers, raw webhook bytes or credentials.

## Cards, installments and marketplace intents

```go
quote, err := client.Payments.InstallmentOptions(ctx, lunixi.InstallmentOptionsRequest{
    Amount: "120000", Currency: "TRY", BinOrPan: bin, Installment: 3,
    EnabledInstallments: []int{1, 2, 3},
})
stored, err := client.Payments.StoreCard(ctx, lunixi.StoreCardRequest{
    Card: cardFields, CardUserKey: buyerKey, CallbackURL: "https://merchant.example/cards/return",
}, persistedStoreKey)
```

`BinInfo` takes a 6 or 8 digit BIN or a card number prefix (digits only); the SDK never puts card numbers or security codes in error messages, and you must not log card fields either. `InstallmentOptions` needs `Amount` (with `Currency`) or `IntentID`; `EnabledInstallments` follows the `IntentRequest` rule and is sent in ascending order. With `Installment` greater than 1 and a BIN the answer carries the `quoteToken` for `DirectMethodPaymentRequest.WithInstallment`. `StoreCard` and `DeactivateStoredCard` require an idempotency key; `ListStoredCards(cardUserKey)` and `GetStoredCard(token)` read stored cards.

`CreateMarketplaceIntent` takes a `MarketplaceIntentRequest`: the `IntentRequest` fields plus `Sellers` (`SubDealerID`, `AmountMinor` as positive minor-unit text, optional `Scenario` object) and `MultiSellerPolicy`. The sellers' amounts must add up to `Amount`; any other total is refused before sending. The key is optional; pass a stable one per attempt to make retries safe. `FailoverRecoveryAnalytics` and `ProviderCredentialAnalytics(credentialID, filters)` read merchant reporting.

## Payment links

```go
link, err := client.PaymentLinks.Create(ctx, lunixi.PaymentLinkRequest{
    Usage: lunixi.PaymentLinkUsageSingleUse, AmountMode: lunixi.PaymentLinkAmountModeFixed,
    Currency: "TRY", AmountMinor: "150000", Title: "Invoice 42",
    Recipient: &lunixi.PaymentLinkRecipient{Email: "buyer@example.com"},
}, persistedLinkKey)
image, err := client.PaymentLinks.UploadImage(ctx, "logo.png", lunixi.PaymentLinkImageContentTypePNG, pngBytes)
```

Every payment-link call is signed, reads included; the signing key decides the environment, so a TEST key sees only TEST links. `Create` and `Send` require an idempotency key (8–200 printable ASCII characters); `Update`, `Pause`, `Resume` and `Deactivate` accept one. Usage, amount mode and currency are fixed at creation: `Update(ctx, linkID, expectedRowVersion, changes, key)` takes only `PaymentLinkSpecFields()` names, refuses `PaymentLinkImmutableFields()`, clears a field given `nil` and sends a `time.Time` as RFC 3339 UTC. Value sets are the `PaymentLink*`, `BuyerFieldRequirement*` and `CustomFieldType*` constants; unknown values are refused before sending, and business rules are left to the API (422 `payment_link.validation.invalid`). `List`, `ListAttempts` and `ListPayments` return `Data.items` with `PageInfo`; pass `pageInfo.nextCursor` as `Cursor`. `QR` returns the SVG or PNG image in `Response.Bytes`.

A link image takes three steps: `CreateImageUpload` returns the upload target, the file is PUT to `UploadURL` with exactly `UploadHeaders`, and `ConfirmImageUpload` publishes it (PNG, JPEG or WebP, at most 2 MiB). `UploadImage` runs all three. The PUT goes to the storage host through your configured HTTP client but not through the signed transport: no access token, signature, digest or idempotency header is sent there. A failed PUT returns an `APIError` with the storage status and code (for example `AccessDenied` after the URL expires) and is not retried; call `UploadImage` again.

## Payment methods from the server

`ListPaymentMethods` lists the methods open for your account with each method's `blockName`, `providerSelection`, `providerSelectionMode` and the providers' `inputFields`; pass a code (`lunixi.PaymentMethodMobileBilling`) to list one method, or `""` for all. Always read provider codes from this answer.

```go
request := lunixi.NewDirectMethodPaymentRequest("10000", "TRY", "ORD-1001").
    WithMethodFields(lunixi.PaymentMethodMobileBilling, providerCode, map[string]any{
        "gsmNumber": "+905321234567",
    })
response, err := client.Payments.DirectMethodPayment(ctx, request, persistedAttemptKey)
if err != nil { return err }
// HTTP 202 with data.action when the buyer must act (PaymentActionType*).
accepted := true
_, err = client.Payments.ConfirmPaymentAction(ctx, paymentID, actionID, lunixi.PaymentActionConfirmRequest{
    OTP: "123456", ConsentAccepted: &accepted,
})
```

`DirectMethodPayment` requires the idempotency key. Before sending, it checks (also for raw `Client.Do` bodies to `/api/v1/payments/direct/payment`): known fields only; `paymentMethod`, `amount`, `currency`, `orderId`; card fields (`CardOnlyFields()`) only with `CARD`; exactly the block of the chosen method (`BlockNameOf`, `MethodBlockFields()`); a block of at most 32 fields whose values are text (at most 1024 characters), numbers, booleans, `ADDRESS` objects or `GROUP` lists of at most 10 objects; 1–100 basket items for `SHOPPING_LOAN`. A method without a block cannot be paid this way yet. Cards use `WithCard` (and `WithForce3D(true)` for 3D Secure). `ConfirmPaymentAction` sends only `otp` (digits), `consentAccepted` and `fields`; `ResendPaymentAction` asks for a new code and closes the previous action.

### Promissory notes

Give the plan either as a schedule or as a note count, never both:

```go
request := lunixi.NewDirectMethodPaymentRequest("120000", "TRY", "ORD-1002").
    WithMethodFields(lunixi.PaymentMethodPromissoryNote, providerCode, debtorFields).
    WithPromissoryNoteSchedule([]lunixi.PromissoryNoteScheduleRow{
        {Amount: "60000", DueDate: "2027-01-15"},
        {Amount: "60000", DueDate: "2027-02-15"},
    })
// or .WithPromissoryNoteCount(3)
```

A schedule has 1–24 rows with positive minor-unit amounts that add up to the payment amount and `YYYY-MM-DD` calendar due dates in ascending order; that a date is in the future is checked by the API. `GetPromissoryNotes` returns the notes and their status, `CancelPromissoryNotes` cancels the listed serial numbers (nil cancels every open note) and `DownloadPromissoryNoteDocument` returns the printable PDF in `Response.Bytes` (locale `PromissoryNoteDocumentLocaleTR` or `PromissoryNoteDocumentLocaleEN`). The document is not stored; download it when needed. Note changes arrive as `PromissoryNoteEvent*` webhooks. `ADDRESS` fields take codes from `client.Reference.ListProvinces`, `ListDistricts(provinceCode)` and `ListNeighborhoods(districtCode)`.

### Para Transferi (bank transfer)

```go
request := lunixi.NewDirectMethodPaymentRequest("125000", "TRY", "ORD-1003").
    WithMethodFields(lunixi.PaymentMethodBankTransfer, "", map[string]any{"bankCode": buyerBankCode})
response, err := client.Payments.DirectMethodPayment(ctx, request, persistedAttemptKey)
if err != nil { return err }
if instruction := response.BankTransferInstruction(); instruction != nil {
    // Show IBAN, HolderName, AmountMinor, PaymentCode and ExpiresAt to the buyer.
}
```

The buyer sends the money from their own bank to your bank account; it does not pass through Lunixi. `bankCode` comes from the method's options in `ListPaymentMethods`; `BankTransferOtherBanksCode` (`"9999"`) means a bank that is not listed. The block takes no provider code. The instruction is display data; the payment is paid only when the gateway reports it captured. Late, short and excess transfers are listed by `ListBankTransferExceptions` (`BankTransferExceptionFilters`: `TEST` / `LIVE`, `BankTransferExceptionStatus*`, limit 1–200) and decided with `AcceptBankTransferException` or `MarkBankTransferExceptionRefunded` (after you refund the sender yourself). Related webhooks are `PaymentBankTransferEvent*`.

## Physical POS

`client.PhysicalPos` starts card payments on your own devices, reverses or cancels them, starts end-of-day and reads payments, transaction trees, sales and tax summaries and end-of-day results. `client.PhysicalPosManagement` manages stores, registers, logical terminals (bank rows, payment-provider rows, device links), devices and their identity keys, and the end-of-day policy at account, store and device level.

```go
key := lunixi.NewPhysicalPosIdempotencyKey()
// Store key with your order BEFORE the call.
result, err := client.PhysicalPos.CreatePayment(ctx, map[string]any{
    "operation":         lunixi.PhysicalPosOperationSale,
    "target":            map[string]any{"deviceId": deviceID},
    "amountMinor":       "4500",
    "currency":          "TRY",
    "merchantReference": orderID,
}, key)
var unknown *lunixi.PhysicalPosOutcomeUnknownError
if errors.As(err, &unknown) {
    // The payment may exist: call again later with the SAME key.
}
// result.StatusCode is 202 for a new payment, 200 with result.Replayed for a repeat.
```

Requests and filters are maps with the gateway's field names (`PhysicalPosRequestFields`, `PhysicalPosFilters`); an unknown field, a missing required field, a wrong type, a numeric amount or a page `limit` outside 1–100 returns `ErrConfiguration` before sending. Amounts are canonical minor-unit text; counts, line numbers, quantities and `rowVersion` are integers. `nil` clears a nullable field (for example `city` in `UpdateStore`); `""` leaves an optional field out. A `time.Time` filter is sent as UTC ISO-8601, and `dateFrom` / `dateTo` as its own calendar day.

Card data never goes to this API: the card is read on the device. A card-data field name (`pan`, `cardNumber`, `cvv`, `expiry`, `track2` …) anywhere in a request, or a card number in `merchantReference`, an item name or tax group code, or the `merchantReference` / `q` filters, returns `*PhysicalPosCardDataRejectedError` with the field path, never the value, and nothing is sent.

A payment write is accepted, not paid: follow it with the `PhysicalPosEvent*` webhooks or `GetPayment`. `CreatePayment` and `CreateReversal` require the key and send it exactly as given (8–200 printable ASCII characters); `CancelPayment`, `StartSettlement` and the management creates accept `""` and then send a new UUID, returned as `IdempotencyKey`. Payment writes retry themselves (`MaxRetries`) with the same key and body. When any attempt got no answer, a 2xx without the payment, or a 5xx that does not say nothing started, the call returns `*PhysicalPosOutcomeUnknownError`; never use a new key for that payment. A void after the device's end-of-day is `*PhysicalPosVoidAfterSettlementError` (refund instead), a 429 is `*PhysicalPosRateLimitError` with `RetryAfter`, and other errors of a keyed write are `*PhysicalPosWriteError` carrying the key. Lists return `PhysicalPosPage` (`Items`, `HasMore`, `NextCursor`, `TotalCount` with `includeTotal`); every list also has an `Iterate…` form that follows the cursor.

`verifier.VerifyPhysicalPos(rawBody, headers)` verifies a webhook and reads it as `PhysicalPosWebhookEvent`: `Category` (`payment`, `fiscal_document`, `settlement`, `other`), `HistorySeq` (nil for end-of-day events; deduplicate those by `ID`), `Original` for reversals, `Cashier` for payments started in a device cashier session, and `IsDeviceSale` for a sale made directly on the device. Amounts in the payload are checked after the signature.

## Wallet

`client.Wallet` is the merchant surface of the closed-loop wallet, grouped by area: `Programs`, `EndUsers`, `Wallets`, `Deposits`, `Corporate`, `Fees`, `Transfers` (wallet to wallet and to an IBAN), `Withdrawals`, `Operations`, `Topups`, `Payments`, `QR` and `BulkPayouts`. Every call is signed; the key needs the `WALLET_SERVICE` product and the wallet permission behind the route.

```go
quote, err := client.Wallet.Transfers.QuoteW2W(ctx, map[string]any{
    "endUserId": endUserID, "sourceWalletAccountId": accountID,
    "destinationWalletNo": walletNo, "amount": "12550", "currency": "TRY",
})
started, err := client.Wallet.Transfers.InitiateW2W(ctx, request, key)
// Commit with the integrityHash and nonce of the answer (and otpCode when otpRequired):
done, err := client.Wallet.Transfers.CompleteW2W(ctx, operationID, map[string]any{
    "endUserId": endUserID, "integrityHash": hash, "nonce": nonce, "otpCode": otp,
}, completeKey)
```

Requests and filters are maps checked against `WalletRoutes()` (the same route table as the other Lunixi SDKs) before sending: an unknown field, a missing required field, an amount that is not minor-unit digit text (`"12550"`), a bad row in `ImportEmployees` / `OffboardEmployees` / bulk payout `items`, or a missing `Idempotency-Key` on a route that requires it returns `ErrConfiguration`. Wallet keys are 1–128 printable ASCII characters. `Complete…` calls put `operationId` into the body themselves. `QR.Image` returns the SVG or PNG in `Response.Bytes`. Lists return `CursorPage`; `Iterate` follows the cursor.

## Bank accounts, transfers and currency exchange

`client.BankConnect` reads your connected bank accounts: `ListBanks`, `ListConnections` / `GetConnection`, `ListAccounts` / `GetAccount`, `ListBalances`, `ListTransactions` / `GetTransaction` (each list with an `Iterate…` form) and `RequestSync`. Connecting a bank happens in the Lunixi dashboard only. Filters are checked against `BankConnectFilters()` before sending: known filters only, values from their sets, `limit` 1–100, at most 100 account ids, minor-unit text amounts with `minAmountMinor` ≤ `maxAmountMinor`, `bookedTo` only with `bookedFrom`, and at most 92 days between them.

`client.BankConnect.Transfers` (`ListPurposes`, `Create`, `List` / `Iterate`, `Get`) and `client.BankConnect.FX` (`ListRates`, `CreateQuote`, `CreateOrder`, `ListOrders` / `IterateOrders`, `GetOrder`) move money on connections that offer it; they need the `bank-connect:transfer` scope and a key with an IP allowlist. Bodies are checked against `BankTransferRoutes()`.

```go
result, err := client.BankConnect.Transfers.Create(ctx, map[string]any{
    "accountId": accountID, "receiverIban": iban, "receiverName": "Ada Lovelace",
    "amountMinor": "150075", "paymentPurpose": purposeCode,
}, payoutID) // "" generates a UUID, returned as result.IdempotencyKey
```

`Create` and `CreateOrder` check your key (8–200 printable ASCII characters) or generate one, and retry a network error or a 5xx (`MaxRetries`) with the same key and body; the same key with the same body answers `Replayed`. A quote takes exactly one of `sourceAmountMinor` / `targetAmountMinor` and two different accounts. Webhooks: `ParseBankAccountTransactionReceived` reads `BankConnectEventTransactionReceived`, `ParseBankTransferWebhook` reads the `BankTransferEvent*` events (`bank_transfer.*`, not the `payment.bank_transfer.*` events of a bank transfer payment).

## Sanctions screening

`client.Sanction` screens names, wallets, accounts, transactions, public procurement bans and adverse media, evaluates behavioural alerts, registers ongoing monitoring and reads alerts, matches, monitored entities, dataset freshness, entity intelligence and your deny list. Bodies and filters are checked against `SanctionRoutes()`, `SanctionNestedFields()` and `SanctionValueSets()` before sending.

```go
result, err := client.Sanction.ScreenName(ctx, map[string]any{
    "name": "Ada Lovelace", "subjectType": lunixi.SanctionSubjectTypePerson,
    "riskDomains": []string{lunixi.SanctionRiskDomainSanctions, lunixi.SanctionRiskDomainPep},
})
if err == nil && lunixi.IsClearScreening(result) {
    // nothing found in a COMPLETE screening
}
```

Screening returns matches, not verdicts. `IsClearScreening` is true only without any match, hit or blocking flag and, where coverage is reported, with `coverageStatus` `COMPLETE`; `PARTIAL`, `DEGRADED` or missing coverage is never clear. A screening that could not run (HTTP 503, or a 2xx whose status is not `success`) is an `*APIError`, never an empty result.

## Insurance

`client.Insurance` lists and searches insurance products for a basket, prices them (`CreateQuote`), turns a quote into an order (`CreateOrder`), binds or confirms the premium payment, issues and cancels policies, refunds the premium, submits leads for types sold by request only, downloads policy documents, receipts and certificates (`Response.Bytes`) and reads reconciliation (`ExportReconciliation` returns the CSV with `Truncated`, `NextCursor` and `Aborted`).

```go
quote, err := client.Insurance.CreateQuote(ctx, map[string]any{
    "insuranceType": "TICKET_CANCELLATION", "currency": "TRY", "paymentIntentId": intentID,
}, "")
consent, err := client.Insurance.ConsentFromQuote(option, acceptedAt)
order, err := client.Insurance.CreateOrder(ctx, map[string]any{"quoteId": quoteID, "consent": consent}, orderKey)
item, err := client.Insurance.PremiumBasketItem(order, "Ticket insurance", "Insurance", "")
```

Fields and filters are checked against `InsuranceQuoteFields()`, `InsuranceLeadFields()`, `InsuranceFilters()` and the other field lists before sending; the rules inside an insurance type's own object stay with the gateway (422 with every field error at once). A quote takes its basket from `paymentIntentId` or `context.items`, not both. `CreateOrder`, `ConfirmPayment`, `IssuePolicy`, `CancelOrder`, `RefundPremium` and `CreateLead` require an `Idempotency-Key` (8–200 printable ASCII characters); `CreateQuote` and `BindPayment` accept `""`. Webhooks: `ParseInsuranceWebhook` reads the order events, `ParseInsuranceLeadWebhook` the `InsuranceEventLeadSubmitted` event.

## Subscriptions

`client.Subscriptions` creates and manages subscriptions (`Create`, `Get`, `List` / `Iterate`, `Cancel`, `Pause`, `Resume`, `ChangePlan`, `AuditLogs`) and hangs the related resources off `Plans`, `Customers`, `Cards`, `Invoices`, `Portal`, `Products`, `Features`, `Addons` and `Usage`.

```go
customer, err := client.Subscriptions.Customers.Upsert(ctx, map[string]any{"externalId": userID, "email": email}, "")
plan, err := client.Subscriptions.Plans.Create(ctx, map[string]any{
    "productId": productID, "code": "pro", "name": "Pro", "price": "9900", "currency": "TRY",
    "billing": map[string]any{"frequency": lunixi.SubscriptionBillingFrequencyMonthly},
}, "")
sub, err := client.Subscriptions.Create(ctx, map[string]any{
    "customerId": customerID, "planId": planID, "paymentCardId": cardID,
}, subscriptionKey) // required; sent in the header and the body
```

Requests and filters are maps checked against `SubscriptionRoutes()` (the same route table as the other Lunixi SDKs) before sending: an unknown field or filter, a missing required field, a value outside its set (collection method, billing frequency, card verification mode, plan-change behavior, portal SDK mode) or a missing key returns `ErrConfiguration`, naming the field and never its value. Money members are canonical minor-unit strings in bodies, filters and answers (`"0"` is a free plan). `Create` requires an `Idempotency-Key` (1–200 printable ASCII characters); plan, customer, card, plan-change and charge-retry writes accept one. `Cards.Add` takes the checkout SDK's `paymentMethodToken`; a card number is refused. The subscription, plan, customer, card, invoice and audit-log lists return `CursorPage` with the `nextPageToken` in `NextCursor` (sent back as `pageToken` by `Iterate`) and `PageSize`. `Portal.CreateSdkSession` returns the session proof for the browser portal SDK; `Portal.CreatePortalLink` returns a hosted link (`SubscriptionPortalURL`). `IsSubscriptionActive`, `IsSubscriptionTerminal` and the `SubscriptionEvent*` webhook names come with it.

## KYC and identity signals

`client.Kyc` creates KYC, KYB and evidence sessions (`CreateSession`, `EvidenceRequest`), reads and lists them (`GetSession`, `ListSessions` / `IterateSessions`, `Timeline`), sets the jurisdiction and primary document, uploads a document from your server, submits, sends to manual review, and reads or updates your KYC policy, webhook deliveries and erasure requests. `person` or `company` is sent as `customer` with its `customerType`, `metadata` as `evidenceContextJson`; `ttlMinutes` is 1–1440. The `kyc.*` webhook (`KycEvent*`) is the authority for the result; `IsKycSessionTerminal` and `KycAssuranceMeets` read the decision.

`client.Identity` reads the device and account-linking signals of the browser fingerprint SDK: `AnalyzeSync` / `Analyze`, `AccountLinks`, `GetSession`, `RelatedSessions`, `Timeline`, `TopSources` and `ListSessions`. `IsIdentityDeviceShared(links, 2)` reports a device shared by two or more accounts; `IdentityLinkReasons` lists why. These are signals, not an allow / block verdict.

## Marketplace sub-dealers

`client.Marketplace` splits charges across sub-dealers and runs the sub-dealer lifecycle, commissions, payouts and the audit log.

```go
legs := []lunixi.MarketplaceSeller{{SubDealerID: "d_alice", AmountMinor: "6050"}, {SubDealerID: "d_bob", AmountMinor: "4000"}}
intent, err := client.Marketplace.CreateCheckoutIntent(ctx, intentRequest, legs, lunixi.MarketplaceMultiSellerPolicyAllOrNothing, attemptKey)
dealer, err := client.Marketplace.CreateDealer(ctx, map[string]any{"legalName": "Acme Ltd", "dealerType": lunixi.MarketplaceDealerTypeLegalEntity})
link, err := client.Marketplace.OnboardingToken(ctx, dealerID)
onboardingURL := lunixi.MarketplaceOnboardingURL(link, "")
```

The split charges (`CreateCheckoutIntent`, `ChargeMarketplaceCard2d`, `ChargeMarketplaceCard3d`, `ChargeMarketplaceCard`) are signed and take an optional payment key (16–128 printable ASCII characters). The seller legs must add up to `amount` with positive minor-unit shares; a card charge also needs `currency`, `orderId`, `card`, `buyer`, `billingAddress` and at least one basket item, and refuses `price` / `paidPrice` and a non-boolean `preauth`. Card data is sent as given and never appears in an error message; most marketplaces use the hosted `CreateCheckoutIntent`, which returns an `*APIError` when the answer carries no token. Dealer, commission, payout and audit routes are bearer-only and checked against `MarketplaceRoutes()`; money members are canonical minor-unit strings in bodies, filters and answers.

## Verify webhooks and consume them transactionally

```go
verifier, err := lunixi.NewWebhookVerifier(lunixi.WebhookConfig{
    Secrets: []string{os.Getenv("LUNIXI_WEBHOOK_SECRET")},
})
if err != nil { return err }
event, err := verifier.VerifyRequest(ctx, request)
if err != nil { return err } // return a non-2xx response

inbox, err := lunixi.NewSQLInbox(db, "lunixi_webhook_inbox", merchantScope)
if err != nil { return err }
// Apply inbox.EnsureSchema(ctx) during migrations / startup, not per request.
applied, err := inbox.Process(ctx, event, func(tx *sql.Tx) error {
    // Validate the authenticated event against your stored order and payment.
    // Apply the order transition AND any outbox rows using this same tx.
    return applyVerifiedOrderTransition(ctx, tx, event)
})
```

The signature is HMAC-SHA256 over `eventId + "." + exactTimestamp + "." + rawBody`. Verification uses constant-time comparison, checks timestamp freshness (default 5 minutes, configurable up to 15 minutes), rejects duplicate headers/JSON keys and matches the signed body ID/type against the headers. The event body must contain `type`; an unsigned type header cannot authorize a financial action. Raw bytes must reach the verifier before any JSON middleware. `Secrets` can include up to three active/previous secrets during rotation. Use a fresh delivery/replay from the dashboard for an old event; do not disable timestamp checks.

`SQLInbox` supports PostgreSQL and SQLite through `database/sql`; supply your own driver. Each merchant/integration gets a separate scope. The callback and event marker commit together. A duplicate skips the callback; a different payload reusing the ID fails closed. Callback/database failure rolls back the marker, allowing safe provider retry. Retain markers for your event replay policy; do not prune within the provider's delivery/replay horizon. External effects require a transactional outbox.

The [runnable SQLite receipt server](examples/webhook) includes actual rollback, crash/restart, duplicate, collision, concurrent replay and HTTP tests. It records verified notifications only; it does not fulfill orders. Add your order state adapter after matching all financial fields. The database contains no webhook body, card data or customer details. Place the loopback HTTP listener behind your trusted HTTPS reverse proxy.

## Examples and checks

```sh
# Main SDK and TEST checkout example
go test -race ./...
go vet ./...
go build ./...
go run ./examples/checkout

# Standalone durable receipt server; the SQLite example requires CGO/C compiler.
cd examples/webhook
go test -race ./...
go vet ./...
go run .
```

The checkout example requires `LUNIXI_BASE_URL`, `LUNIXI_KEY_ID` (TEST only), `LUNIXI_PRIVATE_KEY_PEM`, and persisted `LUNIXI_ORDER_ID` / `LUNIXI_ATTEMPT_KEY`. The receipt server requires `LUNIXI_WEBHOOK_SECRET`. Supply secrets from your shell/secret manager without committing them. `.env` files are ignored and are not loaded automatically.

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Run that audit in both modules. CI checks Go 1.26/1.27, formatting, vet, the race detector, builds and vulnerability reachability. Tests use real TLS test servers and independently produced Node.js Ed25519/JSON/HMAC fixtures. All fixture keys/secrets are synthetic and unusable for merchant access. Tests require no merchant credentials or network payment calls.

[Security policy](SECURITY.md) · [Contributing](CONTRIBUTING.md) · [MIT license](LICENSE)

## Merchant-page integration example

The source example defines `RenderCheckoutHTML(token, nonce)` and `HostedCheckoutURL(paymentPageUrl)` in [presentation.go](https://github.com/Lunixi-Payment/lunixi-go/blob/main/examples/checkout/presentation.go). Copy/adapt these helpers inside your existing authorized, CSRF-protected merchant controller. The CLI defaults to `LUNIXI_CHECKOUT_MODE=embedded`; `hosted` validates the backend URL. It renders/validates internally and intentionally prints neither HTML, token nor URL. These example functions are not new SDK client methods.

Before upgrading from 0.x, follow [the migration guide](MIGRATION-1.0.md).

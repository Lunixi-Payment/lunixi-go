<p align="center">
  <a href="https://lunixi.com"><picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/lunixi-logo-dark.svg">
    <img src=".github/assets/lunixi-logo.svg" alt="Lunixi" height="64">
  </picture></a>
</p>

# Lunixi Go SDK

[![CI](https://github.com/Lunixi-Payment/lunixi-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Lunixi-Payment/lunixi-go/actions/workflows/ci.yml)
[![Go reference](https://pkg.go.dev/badge/github.com/Lunixi-Payment/lunixi-go.svg)](https://pkg.go.dev/github.com/Lunixi-Payment/lunixi-go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**[Documentation](https://docs.lunixi.com/sdks/go) · [Türkçe](docs/README.tr.md) · [Changelog](CHANGELOG.md) · [Security](SECURITY.md)**

Server-side Go client for Lunixi payments. Works with `net/http`, Gin, Echo, Fiber and other Go frameworks without a framework dependency. The SDK uses the Go standard library only. Go 1.26 or newer is required.

## Install

```sh
go get github.com/Lunixi-Payment/lunixi-go@v0.1.0
```

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

The client is safe to share between goroutines. Token acquisition is lazy, cached with an expiry margin and coalesced during concurrent refresh. All merchant requests are signed, including GET requests. Canonicalization matches the gateway's JSON.stringify numeric-key order, compact UTF-8 and number format; unsafe JSON integers and duplicate keys are rejected.

| Setting | Default | Bounds / behavior |
|---|---|---|
| `Timeout` | 30 seconds | Up to 2 minutes per HTTP request; pass an overall context deadline |
| `MaxRetries` | 0 | 0–5; GET or a supplied stable idempotency key only |
| `MaxResponseBytes` | 8 MiB | Up to 32 MiB; request JSON is limited to 8 MiB |
| `HTTPClient` | Standard client | Copied; redirects always disabled; never use insecure TLS in production |
| `AllowInsecureLocalhost` | false | HTTP allowed only for explicit localhost / literal loopback testing |

`BaseURL` must be a trusted HTTPS origin, without credentials, path or query. Merchant API paths cannot change origin, traverse directories or call the token endpoint through `Do`.

## Start hosted checkout

Load the basket and order from your server-side records. Persist the attempt key before calling, then reuse that key and body after a timeout.

```go
ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
defer cancel()

response, err := client.Payments.CreateIntent(ctx, lunixi.IntentRequest{
    Amount:      10050, // 100.50 TRY, integer minor units
    Currency:    "TRY",
    OrderID:     "ORD-1001",
    CallbackURL: "https://merchant.example/payment/return",
    BasketItems: []lunixi.BasketItem{
        {ID: "line-1", Name: "Example product", ItemType: "PHYSICAL", Price: 10050},
    },
}, persistedAttemptKey)
if err != nil { return err }

var checkout struct { Token string `json:"token"` }
if err := response.DecodeData(&checkout); err != nil { return err }
if checkout.Token == "" { return errors.New("missing checkout token") }
// Send Token only to this order's authenticated customer, with
// Cache-Control: no-store. Initialize the existing Lunixi checkout form.
```

Amounts are `int64` minor units, positive and no greater than `9007199254740991`. Basket `Price` is the line total; line totals must equal `Amount`. Do not send the removed root fields `price`, `paidPrice`, `autoCapture`. Installment selection and fees belong to the Lunixi form. Buyer/address and documented insurance basket details can be supplied on `IntentRequest`; optional method restrictions can narrow the methods already enabled in the Lunixi panel.

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
| `Payments` | `CreateIntent`, `Get`, `List`, `Methods`, `Capture`, `Refund`, `Void` |
| `Payments` split | `Split`, `GetSplit`, `RefundSplit` |
| `Fraud` | `Evaluate`, `RecordEvent`, `Decisions`, `Decision` |
| `Client.Do` | Signed requests to other documented `/api/v1/` merchant endpoints |
| `WebhookVerifier`, `SQLInbox` | Raw v2 verification and durable transactional deduplication |

`Split` accepts the documented gateway map and locally checks safe total/leg amounts, 1–5 legs and the leg sum. Method-specific fields remain governed by the API schema. A pending split may return a `Response` with HTTP 400; inspect `StatusCode` and signed gateway status. `AUTHORIZED`, `AWAITING_*`, `COMPENSATING`, partial refunds and reconciliation-required states do not imply a fulfilled order. Root SDK responses preserve the original envelope, `Data`, `PageInfo` and binary `Bytes`; `DecodeData` uses `json.Number` for interface values.

Wallet, bank-connect, payment links, insurance, marketplace, subscription, identity, reference and other services currently use `Do` with their documented schema. Version 0.1.0 does not claim typed helper parity for these services. For example:

```go
response, err := client.Do(ctx, lunixi.Request{
    Method: "GET",
    Path: "/api/v1/payments",
    Query: url.Values{"limit": {"20"}},
})
```

Retries keep the exact body and idempotency key and use a new signature nonce. Mutation helpers require 16–128 non-space printable ASCII characters as the key. GET and keyed calls can retry transient network failures / 429 / 500 / 502 / 503 / 504 within the configured bound, with capped exponential delay; safe operations can refresh a rejected token once. Unkeyed mutations never automatically retry. Reconcile an unknown payment outcome before creating another attempt.

`APIError` provides `StatusCode`, `Code` and `RequestID`. Error strings omit upstream messages and request/response bodies. Responses can contain customer data or tokens; do not log `Response`, headers, raw webhook bytes or credentials.

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

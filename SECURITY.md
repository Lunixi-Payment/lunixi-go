# Security policy

Report suspected vulnerabilities privately to **developer@lunixi.com**. Include the affected SDK version, a minimal reproduction using synthetic data, the impact and any mitigation. Do not open a public issue containing credentials, customer data or an exploit against a live merchant.

The latest published 0.1.x release receives security fixes. Upgrade to the newest patch and a supported patched Go toolchain. We investigate reports and coordinate disclosure after a fix is available; this policy does not promise a response deadline.

## Integration boundaries

Keep private keys, webhook secrets and access tokens on the server. Use an independently registered public key and separate TEST/LIVE credentials. Configure BaseURL from trusted deployment configuration, not an incoming request. Redirects are disabled; custom HTTP transports must preserve TLS certificate/hostname verification. The loopback HTTP option is for tests only.

The SDK rejects unsafe integer JSON numbers, removed payment amount fields, untrusted API paths and unsafe configuration before network access. Automatic mutation retries require a stable caller-provided key. Unknown outcomes require reconciliation.

Verify exact webhook bytes before parsing. Apply order state and the replay marker in the same durable transaction, matched to stored merchant/order/payment/totals/currency. Timestamp freshness alone does not prevent replay. Retain deduplication records for your replay policy. Use an outbox for external side effects. Never finalize from browser parameters, a token, authorization or HTTP success alone.

Error strings omit upstream messages and payloads. Returned response objects and webhook accessors can contain sensitive data: do not log them. The durable receipt example stores IDs/types only and does not implement fulfillment. The SDK does not collect raw card data or remove your payment/privacy obligations.

## Verification

Run `go test -race ./...`, `go vet ./...` and `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` in the root and `examples/webhook` modules. CI runs the same checks. Cross-language fixture credentials are synthetic, deliberately public test data.

References: [Go security best practices](https://go.dev/doc/security/best-practices), [standard-library Ed25519](https://pkg.go.dev/crypto/ed25519).

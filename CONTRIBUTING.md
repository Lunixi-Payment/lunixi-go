# Contributing

Open an issue describing the behavior or proposed change before a large feature. Security reports belong in the private channel in [SECURITY.md](SECURITY.md).

Use Go 1.26+ and keep the root SDK free of runtime dependencies. Keep shared signing, transport, money and webhook rules centralized. Add only publicly documented API endpoints; preserve response envelopes and expose pending/reconciliation states without interpreting them as success. Framework examples must use the same client.

Run formatting, `go test -race ./...`, `go vet ./...`, `go build ./...` and govulncheck in the root module, then the same checks in `examples/webhook` (CGO required). Tests must run with no real merchant credentials or payment calls. Add regression coverage for meaningful security/financial changes; generated fixtures must be synthetic and independently reproducible.

Update the English README, Turkish guide, changelog and examples when the public API changes. Do not commit secrets, local database files or private/customer data. Pull requests should explain the trigger, resulting behavior and actual validation. Keep diffs focused and preserve the original Lunixi logo.

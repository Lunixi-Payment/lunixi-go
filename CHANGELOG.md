# Changelog

## 0.1.1 - 2026-10-31

- Provide a concrete merchant-site renderer/example using the existing official Lunixi checkout form by default, with an explicit optional hosted presentation.
- Use only the validated backend paymentPageUrl for redirects; its opaque token is independent of the embedded token. Missing URL keeps embedded available and requires reconciliation for hosted mode without inventing another key.
- Preserve authentication, customer ownership/CSRF integration boundaries, no-store/token-redaction rules and raw response compatibility. Existing published tags are unchanged.

## 0.1.0 — 2026-10-31

- Initial server-side Go SDK with no runtime dependencies.
- Ed25519 PKCS8/seed authentication, JavaScript-compatible canonical JSON, concurrent token cache and bounded signed HTTPS transport.
- Payment lifecycle, split payments, fraud evaluation/events and generic documented merchant API access.
- Integer-money validation, stable keyed retries, redacted errors and preserved response envelopes.
- Raw v2 webhook verification, rotation and transactional PostgreSQL/SQLite replay inbox.
- Runnable TEST checkout and durable SQLite webhook examples, English/Turkish guides, race/security tests and CI.

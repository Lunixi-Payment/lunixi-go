// Package lunixi is a server-side client for Lunixi's signed merchant APIs.
//
// The client signs requests with Ed25519, caches short-lived access tokens,
// preserves integer minor-unit money, and verifies raw-body v2 webhooks.
// Secret keys belong on the merchant server; browser checkout uses the
// short-lived token returned by Payments.CreateIntent.
package lunixi

// Version is the SDK semantic version.
const Version = "0.1.1"

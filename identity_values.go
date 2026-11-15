package lunixi

import (
	"encoding/json"
	"slices"
)

// Identity (device and account-linking signals) — /api/v1/identity/*. The
// browser fingerprint SDK uses the publishable key (/identity/sdk/init),
// which is not in IdentityRoutes.

const identityLiveSessions = "/api/v1/identity/live-sessions"

// identityAnalyzeFields are the body fields of analyze and analyzeSync; the
// nested objects are forwarded as given.
var identityAnalyzeFields = []string{"subjectType", "subjectId", "productContext", "integrationMode", "customer", "device", "session", "source", "account", "metadata", "paymentId"}

// IdentityAnalyzeFields returns the body fields of Analyze and AnalyzeSync.
func IdentityAnalyzeFields() []string { return slices.Clone(identityAnalyzeFields) }

// identityRoutes is the single description of every identity route the SDK
// calls (the same table as the other Lunixi SDKs). The surface is
// bearer-only.
var identityRoutes = map[string]Route{
	"analyzeSync":     {Method: "POST", Path: identityLiveSessions + "/analyze-sync", Optional: identityAnalyzeFields},
	"analyze":         {Method: "POST", Path: identityLiveSessions + "/analyze", Optional: identityAnalyzeFields},
	"accountLinks":    {Method: "GET", Path: identityLiveSessions + "/:traceId/account-links"},
	"getSession":      {Method: "GET", Path: identityLiveSessions + "/:traceId"},
	"relatedSessions": {Method: "GET", Path: identityLiveSessions + "/:traceId/related-sessions", Query: []string{"limit"}},
	"timeline":        {Method: "GET", Path: identityLiveSessions + "/:traceId/timeline"},
	"topSources":      {Method: "GET", Path: identityLiveSessions + "/top-sources", Query: []string{"days", "limit"}},
	"listSessions":    {Method: "GET", Path: identityLiveSessions, Query: []string{"fingerprintId", "deviceId", "paymentId", "limit"}},
}

// IdentityRoutes returns a copy of the identity route table, keyed by
// operation name.
func IdentityRoutes() map[string]Route { return cloneRoutes(identityRoutes) }

// identityAccountLinks is the part of an AccountLinks answer the helpers
// read.
type identityAccountLinks struct {
	LinkedAccounts []json.RawMessage `json:"linkedAccounts"`
	Sessions       []struct {
		LinkReasons []any `json:"linkReasons"`
	} `json:"sessions"`
}

// IsIdentityDeviceShared reports whether the device of an AccountLinks
// answer is shared by at least threshold accounts, counting the current one
// (linkedAccounts excludes it). A threshold below 2 is 2: a second account
// on the device trips it.
func IsIdentityDeviceShared(accountLinks json.RawMessage, threshold int) bool {
	var links identityAccountLinks
	_ = json.Unmarshal(accountLinks, &links)
	return len(links.LinkedAccounts)+1 >= max(2, threshold)
}

// IdentityLinkReasons returns the distinct linkReasons of an AccountLinks
// answer's sessions (same_device_cluster, same_fingerprint, …), in the
// order they first appear.
func IdentityLinkReasons(accountLinks json.RawMessage) []string {
	var links identityAccountLinks
	_ = json.Unmarshal(accountLinks, &links)
	reasons := []string{}
	for _, session := range links.Sessions {
		for _, reason := range session.LinkReasons {
			text, ok := reason.(string)
			if !ok {
				raw, _ := json.Marshal(reason)
				text = string(raw)
			}
			if !slices.Contains(reasons, text) {
				reasons = append(reasons, text)
			}
		}
	}
	return reasons
}

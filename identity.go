package lunixi

import (
	"context"
	"encoding/json"
)

// IdentityClient — identity signals (/api/v1/identity/*), reached as
// Client.Identity.
//
// The calls carry the access token, without the per-request signature. The
// browser fingerprint SDK produces a traceId; your server reads the linked
// accounts of that trace and flags or blocks when the device is shared ("one
// person, many accounts", IsIdentityDeviceShared).
//
// This is a signal surface: it carries risk flags, reason codes, similarity
// probabilities and the identity snapshot, not an allow / review / block
// verdict (that is the fraud decision). Calls return the envelope's data;
// ListSessions returns CursorPage.
type IdentityClient struct{ client *Client }

// AnalyzeSync analyses a session synchronously: subjectType, subjectId,
// productContext, integrationMode, customer, device, session, source,
// account, metadata, paymentId (all optional; subjectType and subjectId go
// together). Returns {traceId, analysisStatus, riskFlags, reasonCodes,
// identitySnapshot, …}.
func (i *IdentityClient) AnalyzeSync(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return i.analyze(ctx, "analyzeSync", request)
}

// Analyze queues the analysis and returns the traceId at once; the same
// body as AnalyzeSync.
func (i *IdentityClient) Analyze(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return i.analyze(ctx, "analyze", request)
}

// AccountLinks reads the other accounts seen on this device, fingerprint or
// IP (account keys hashed).
func (i *IdentityClient) AccountLinks(ctx context.Context, traceID string) (json.RawMessage, error) {
	return i.data(ctx, "accountLinks", traceID, nil)
}

// GetSession reads one identity session.
func (i *IdentityClient) GetSession(ctx context.Context, traceID string) (json.RawMessage, error) {
	return i.data(ctx, "getSession", traceID, nil)
}

// RelatedSessions reads the recent sessions of the same device-identity
// cluster; limit 0 sends none.
func (i *IdentityClient) RelatedSessions(ctx context.Context, traceID string, limit int) (json.RawMessage, error) {
	query := map[string]any{}
	if limit != 0 {
		query["limit"] = limit
	}
	return i.data(ctx, "relatedSessions", traceID, query)
}

// Timeline reads the session summary and its recorded lifecycle events.
func (i *IdentityClient) Timeline(ctx context.Context, traceID string) (json.RawMessage, error) {
	return i.data(ctx, "timeline", traceID, nil)
}

// TopSources ranks source domains by repeated risky activity. Filters:
// days, limit.
func (i *IdentityClient) TopSources(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	r, err := i.call(ctx, "topSources", nil, orEmpty(filters), nil)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// ListSessions lists identity sessions. Filters: fingerprintId, deviceId,
// paymentId, limit.
func (i *IdentityClient) ListSessions(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	r, err := i.call(ctx, "listSessions", nil, orEmpty(filters), nil)
	if err != nil {
		return nil, err
	}
	return tablePage(r), nil
}

func (i *IdentityClient) analyze(ctx context.Context, name string, request map[string]any) (json.RawMessage, error) {
	body, err := requestObject(name+" request", request)
	if err != nil {
		return nil, err
	}
	for _, field := range []string{"customer", "device", "session", "source", "account", "metadata"} {
		if err = objectMember(body, field); err != nil {
			return nil, err
		}
	}
	if (body["subjectType"] == nil) != (body["subjectId"] == nil) {
		return nil, invalid("subjectType and subjectId go together")
	}
	r, err := i.call(ctx, name, nil, nil, body)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (i *IdentityClient) data(ctx context.Context, name, traceID string, query map[string]any) (json.RawMessage, error) {
	r, err := i.call(ctx, name, map[string]string{"traceId": traceID}, query, nil)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (i *IdentityClient) call(ctx context.Context, name string, params map[string]string, query, body map[string]any) (*Response, error) {
	return sendTableRoute(ctx, i.client, identityRoutes, name, tableCall{params: params, query: query, body: body})
}

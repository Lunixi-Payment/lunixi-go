package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"maps"
	"slices"
	"strings"
)

// KycClient — KYC, KYB and evidence sessions (/api/v1/kyc/*), reached as
// Client.Kyc.
//
// The calls carry the access token, without the per-request signature. Your
// server creates the session (externalCustomerId = your own user id, echoed
// on every webhook); the browser SDK then runs the capture with the
// publishable key. The kyc.* webhook is the authority for the result.
//
// Requests and filters are maps checked against KycRoutes before anything is
// sent; the error names the field, never its value (document images
// included). Session calls return the envelope's data ({session,
// capturePolicy, …}: no personal data comes back, only status, decision,
// risk and the achieved assurance). Lists return CursorPage.
type KycClient struct{ client *Client }

// CreateSession creates a session: subjectId, sessionType
// (KycSessionType*, case-insensitive), optional externalCustomerId,
// customerMode, person | company | customer, jurisdiction,
// requiredDocumentTypes, primaryDocumentType, evidencePurpose, ttlMinutes
// (1–KycSessionTTLMaxMinutes), metadata. person and company are sent as
// customer with customerType PERSON or COMPANY; metadata is sent as
// evidenceContextJson. An Idempotency-Key (the argument or the request's
// idempotencyKey) goes in the header and the body; "" sends none.
func (k *KycClient) CreateSession(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, key, err := kycSessionBody("createSession", request, idempotencyKey)
	if err != nil {
		return nil, err
	}
	return k.data(ctx, "createSession", nil, body, key)
}

// GetSession reads one session.
func (k *KycClient) GetSession(ctx context.Context, sessionID string) (json.RawMessage, error) {
	return k.data(ctx, "getSession", map[string]string{"sessionId": sessionID}, nil, "")
}

// ListSessions lists sessions. Filters: limit, cursor (pageSize and
// pageToken are read as the same). Anything else is refused: the merchant
// list has no status or customer filter, and dropping one would return
// every session.
func (k *KycClient) ListSessions(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	given := compactNil(orEmpty(filters))
	var unknown []string
	for field := range given {
		if !slices.Contains([]string{"limit", "cursor", "pageSize", "pageToken"}, field) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return nil, invalid("listSessions does not accept the filter(s) " + strings.Join(unknown, ", ") + "; it takes limit, cursor")
	}
	query := map[string]any{}
	for wire, alias := range map[string]string{"limit": "pageSize", "cursor": "pageToken"} {
		if value, ok := given[wire]; ok {
			query[wire] = value
		} else if value, ok := given[alias]; ok {
			query[wire] = value
		}
	}
	r, err := k.call(ctx, "listSessions", nil, query, nil, "")
	if err != nil {
		return nil, err
	}
	return tablePage(r), nil
}

// IterateSessions yields every session, page by page.
func (k *KycClient) IterateSessions(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	current := maps.Clone(filters)
	if current != nil {
		if token, ok := current["pageToken"]; ok && current["cursor"] == nil {
			current["cursor"] = token
		}
		delete(current, "pageToken")
	}
	return iterateCursor(ctx, current, k.ListSessions)
}

// SetJurisdiction sets the applicant jurisdiction (ISO country) on a
// server-created session.
func (k *KycClient) SetJurisdiction(ctx context.Context, sessionID, jurisdiction string) (json.RawMessage, error) {
	text, err := requiredText(jurisdiction, "jurisdiction")
	if err != nil {
		return nil, err
	}
	return k.data(ctx, "setJurisdiction", map[string]string{"sessionId": sessionID}, map[string]any{"jurisdiction": text}, "")
}

// SetPrimaryDocument sets the primary document type: PASSPORT,
// NATIONAL_ID, DRIVERS_LICENSE or RESIDENCE_PERMIT.
func (k *KycClient) SetPrimaryDocument(ctx context.Context, sessionID, documentType string) (json.RawMessage, error) {
	text, err := requiredText(documentType, "documentType")
	if err != nil {
		return nil, err
	}
	return k.data(ctx, "setPrimaryDocument", map[string]string{"sessionId": sessionID}, map[string]any{"documentType": text}, "")
}

// UploadDocument uploads a document from your server (most flows use the
// browser SDK): documentType, fileBase64, mimeType, optional side,
// jurisdiction, metadataJson, idempotencyKey. The image never appears in an
// error message.
func (k *KycClient) UploadDocument(ctx context.Context, sessionID string, document map[string]any) (json.RawMessage, error) {
	if document == nil {
		return nil, invalid("uploadDocument document must be an object")
	}
	return k.data(ctx, "uploadDocument", map[string]string{"sessionId": sessionID}, document, "")
}

// Timeline reads the event timeline of a session (audit).
func (k *KycClient) Timeline(ctx context.Context, sessionID string) (json.RawMessage, error) {
	return k.data(ctx, "timeline", map[string]string{"sessionId": sessionID}, nil, "")
}

// Submit submits a session for processing.
func (k *KycClient) Submit(ctx context.Context, sessionID string) (json.RawMessage, error) {
	return k.data(ctx, "submit", map[string]string{"sessionId": sessionID}, nil, "")
}

// ManualReview sends a session to manual review; reasonCode and
// reasonDetail "" are left out (no body when both are).
func (k *KycClient) ManualReview(ctx context.Context, sessionID, reasonCode, reasonDetail string) (json.RawMessage, error) {
	body := optionalText(optionalText(map[string]any{}, "reasonCode", reasonCode), "reasonDetail", reasonDetail)
	return k.data(ctx, "manualReview", map[string]string{"sessionId": sessionID}, body, "")
}

// EvidenceRequest creates an EVIDENCE session (proof-of-X), optionally
// linked to a payment or order; the same body and key rule as
// CreateSession.
func (k *KycClient) EvidenceRequest(ctx context.Context, request map[string]any, idempotencyKey string) (json.RawMessage, error) {
	body, key, err := kycSessionBody("evidenceRequest", request, idempotencyKey)
	if err != nil {
		return nil, err
	}
	return k.data(ctx, "evidenceRequest", nil, body, key)
}

// MerchantPolicy reads your KYC policy (required assurance, providers …).
func (k *KycClient) MerchantPolicy(ctx context.Context) (json.RawMessage, error) {
	return k.data(ctx, "merchantPolicy", nil, nil, "")
}

// UpdateMerchantPolicy writes your KYC policy.
func (k *KycClient) UpdateMerchantPolicy(ctx context.Context, policy map[string]any) (json.RawMessage, error) {
	if policy == nil {
		return nil, invalid("updateMerchantPolicy policy must be an object")
	}
	return k.data(ctx, "updateMerchantPolicy", nil, policy, "")
}

// RotateWebhookSecret rotates the KYC webhook signing secret.
func (k *KycClient) RotateWebhookSecret(ctx context.Context) (json.RawMessage, error) {
	return k.data(ctx, "rotateWebhookSecret", nil, nil, "")
}

// WebhookOutbox lists the KYC webhook deliveries. Filters: limit, status.
func (k *KycClient) WebhookOutbox(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return k.page(ctx, "webhookOutbox", filters)
}

// RetryWebhookEvent retries one KYC webhook delivery.
func (k *KycClient) RetryWebhookEvent(ctx context.Context, eventID string) (json.RawMessage, error) {
	return k.data(ctx, "retryWebhookEvent", map[string]string{"eventId": eventID}, nil, "")
}

// ErasureRequests lists the data erasure requests. Filters: limit, status.
func (k *KycClient) ErasureRequests(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return k.page(ctx, "erasureRequests", filters)
}

func (k *KycClient) call(ctx context.Context, name string, params map[string]string, query, body map[string]any, key string) (*Response, error) {
	return sendTableRoute(ctx, k.client, kycRoutes, name, tableCall{params: params, query: query, body: body, key: key})
}

func (k *KycClient) data(ctx context.Context, name string, params map[string]string, body map[string]any, key string) (json.RawMessage, error) {
	r, err := k.call(ctx, name, params, nil, body, key)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (k *KycClient) page(ctx context.Context, name string, filters map[string]any) (*CursorPage, error) {
	r, err := k.call(ctx, name, nil, orEmpty(filters), nil, "")
	if err != nil {
		return nil, err
	}
	return tablePage(r), nil
}

// kycSessionBody builds the body of createSession / evidenceRequest.
func kycSessionBody(name string, request map[string]any, idempotencyKey string) (map[string]any, string, error) {
	object, err := requestObject(name+" request", request)
	if err != nil {
		return nil, "", err
	}
	person, company, metadata := object["person"], object["company"], object["metadata"]
	delete(object, "person")
	delete(object, "company")
	delete(object, "metadata")
	body := compactNil(object)
	key, err := bodyKey(name, body, idempotencyKey)
	if err != nil {
		return nil, "", err
	}
	if key != "" {
		body["idempotencyKey"] = key
	}
	if _, ok := body["sessionType"]; ok {
		if err = upperChoice(body, "sessionType", KycSessionTypes()); err != nil {
			return nil, "", err
		}
	}
	identities := 0
	for _, value := range []any{person, company, body["customer"]} {
		if value != nil {
			identities++
		}
	}
	if identities > 1 {
		return nil, "", invalid(name + " takes one of person, company or customer")
	}
	for kind, value := range map[string]any{"PERSON": person, "COMPANY": company} {
		if value == nil {
			continue
		}
		fields, ok := value.(map[string]any)
		if !ok {
			return nil, "", invalid(strings.ToLower(kind) + " must be an object")
		}
		customer := maps.Clone(fields)
		customer["customerType"] = kind
		body["customer"] = customer
	}
	if metadata != nil {
		if _, ok := body["evidenceContextJson"]; ok {
			return nil, "", invalid(name + " takes metadata or evidenceContextJson, not both")
		}
		if _, ok := metadata.(map[string]any); !ok {
			return nil, "", invalid("metadata must be an object")
		}
		text, err := CanonicalJSON(metadata)
		if err != nil {
			return nil, "", err
		}
		body["evidenceContextJson"] = string(text)
	}
	if ttl, ok := body["ttlMinutes"]; ok {
		if n, isInt := integerOf(ttl); !isInt || n < 1 || n > KycSessionTTLMaxMinutes {
			return nil, "", invalid("ttlMinutes must be an integer from 1 to " + formatInt(KycSessionTTLMaxMinutes))
		}
	}
	if types, ok := body["requiredDocumentTypes"]; ok {
		if _, isList := stringsOf(types); !isList {
			return nil, "", invalid("requiredDocumentTypes must be an array of document types")
		}
		if _, isText := types.(string); isText {
			return nil, "", invalid("requiredDocumentTypes must be an array of document types")
		}
	}
	return body, key, nil
}

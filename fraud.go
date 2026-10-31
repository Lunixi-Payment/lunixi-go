package lunixi

import (
	"context"
	"net/url"
)

// FraudClient exposes transaction evaluation and event ingestion. Results
// remain gateway decisions; callers must enforce their own business policy.
type FraudClient struct{ client *Client }

var fraudFields = map[string]bool{
	"integrationMode": true, "productContext": true, "transactionRef": true, "channel": true, "surface": true, "stage": true, "operationKind": true, "authModeRequested": true, "authModeApplied": true, "subMerchantRef": true, "subjectType": true, "subjectId": true, "paymentRail": true, "transferDirection": true, "merchantGroupIds": true, "paymentId": true, "payment": true, "bankTransfer": true, "cryptoTransfer": true, "walletOperation": true, "billPayment": true, "loanApplication": true, "payout": true, "customer": true, "device": true, "session": true, "merchant": true, "mobileApp": true, "telemetry": true, "metadata": true, "identitySnapshot": true, "identityAssertion": true,
}

// Evaluate rejects unknown top-level fields, including tenantId, before
// authentication. Tenant identity always comes from the merchant credential.
func (f *FraudClient) Evaluate(ctx context.Context, body map[string]any) (*Response, error) {
	if body == nil {
		return nil, invalid("fraud request")
	}
	for k := range body {
		if !fraudFields[k] {
			return nil, invalid("unknown fraud field")
		}
	}
	return f.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/fraud/decisions/evaluate", Body: body})
}

// RecordEvent ingests one transaction event. A stable eventId and caller key
// are required by this helper for durable application-level idempotency.
func (f *FraudClient) RecordEvent(ctx context.Context, event map[string]any, key string) (*Response, error) {
	if e := requireKey(key); e != nil {
		return nil, e
	}
	for _, field := range []string{"eventId", "subjectType", "eventType", "occurredAt"} {
		if s, ok := event[field].(string); !ok || s == "" {
			return nil, invalid("fraud event required fields")
		}
	}
	return f.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/fraud/events/transactions", Body: event, IdempotencyKey: key})
}

// Decisions lists fraud decisions without discarding sibling pageInfo.
func (f *FraudClient) Decisions(ctx context.Context, filters url.Values) (*Response, error) {
	return f.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/fraud/decision-logs", Query: filters})
}

// Decision reads one fraud decision by trace ID.
func (f *FraudClient) Decision(ctx context.Context, traceID string) (*Response, error) {
	id, e := idPath(traceID)
	if e != nil {
		return nil, e
	}
	return f.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/fraud/decision-logs/" + id})
}

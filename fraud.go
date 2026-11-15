package lunixi

import (
	"context"
	"encoding/json"
	"net/url"
)

// FraudEventBatchLimit is the largest number of events RecordBatch sends in
// one request; the gateway rejects a larger batch as a whole.
const FraudEventBatchLimit = 500

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
	if e := validateFraudEvent(event); e != nil {
		return nil, e
	}
	return f.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/fraud/events/transactions", Body: event, IdempotencyKey: key})
}

func validateFraudEvent(event map[string]any) error {
	for _, field := range []string{"eventId", "subjectType", "eventType", "occurredAt"} {
		if s, ok := event[field].(string); !ok || s == "" {
			return invalid("fraud event required fields")
		}
	}
	return nil
}

// RecordBatch ingests 1–FraudEventBatchLimit transaction events in one request
// (POST /fraud/events/transactions/batch, sent as a JSON array). Every event
// needs eventId, subjectType, eventType and occurredAt; a stable caller key is
// required, as for RecordEvent. Data reports the accepted and rejected events.
func (f *FraudClient) RecordBatch(ctx context.Context, events []map[string]any, key string) (*Response, error) {
	if e := requireKey(key); e != nil {
		return nil, e
	}
	if len(events) == 0 || len(events) > FraudEventBatchLimit {
		return nil, invalid("fraud event batch must hold 1 to 500 events")
	}
	for _, event := range events {
		if e := validateFraudEvent(event); e != nil {
			return nil, e
		}
	}
	return f.client.Do(ctx, Request{Method: "POST", Path: "/api/v1/fraud/events/transactions/batch", Body: events, IdempotencyKey: key})
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

// LatestForPayment returns the most recent fraud decision of a payment, or
// nil when the payment has none. It reads the decision list with paymentId
// and limit 1.
func (f *FraudClient) LatestForPayment(ctx context.Context, paymentID string) (json.RawMessage, error) {
	id, e := idPath(paymentID)
	if e != nil {
		return nil, invalid("paymentId")
	}
	response, e := f.Decisions(ctx, url.Values{"paymentId": {id}, "limit": {"1"}})
	if e != nil {
		return nil, e
	}
	var items []json.RawMessage
	if json.Unmarshal(response.Data, &items) != nil || len(items) == 0 || string(items[0]) == "null" {
		return nil, nil
	}
	return items[0], nil
}

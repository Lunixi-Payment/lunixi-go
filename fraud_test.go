package lunixi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func fraudEvent(id string) map[string]any {
	return map[string]any{"eventId": id, "subjectType": "CUSTOMER", "eventType": "LOGIN", "occurredAt": "2026-11-15T10:00:00Z"}
}

func TestFraudRecordBatch(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Fraud.RecordBatch(ctx, []map[string]any{fraudEvent("evt-1"), fraudEvent("evt-2")}, testKey); e != nil {
		t.Fatal(e)
	}
	_, e := client.Fraud.RecordBatch(ctx, []map[string]any{fraudEvent("evt-1")}, "")
	mustReject(t, "missing key", e)
	_, e = client.Fraud.RecordBatch(ctx, nil, testKey)
	mustReject(t, "empty batch", e)
	tooMany := make([]map[string]any, FraudEventBatchLimit+1)
	for i := range tooMany {
		tooMany[i] = fraudEvent(fmt.Sprintf("evt-%d", i))
	}
	_, e = client.Fraud.RecordBatch(ctx, tooMany, testKey)
	mustReject(t, "501 events", e)
	incomplete := fraudEvent("evt-3")
	delete(incomplete, "occurredAt")
	_, e = client.Fraud.RecordBatch(ctx, []map[string]any{fraudEvent("evt-1"), incomplete}, testKey)
	mustReject(t, "incomplete event", e)
	sent := requests()
	if len(sent) != 1 {
		t.Fatalf("invalid batches reached the network: %d", len(sent))
	}
	r := sent[0]
	if r.Method != "POST" || r.URI != "/api/v1/fraud/events/transactions/batch" || !r.Signed || r.IdempotencyKey != testKey || !strings.HasPrefix(string(r.Body), `[{"eventId":"evt-1"`) {
		t.Fatalf("batch request %+v %s", r, r.Body)
	}
}

func TestFraudLatestForPayment(t *testing.T) {
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("paymentId") == "pay_empty" {
			fmt.Fprint(w, `{"success":true,"data":[],"pageInfo":{"hasMore":false}}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":[{"traceId":"trace-2","decision":"allow"},{"traceId":"trace-1"}],"pageInfo":{"hasMore":true}}`)
	})
	ctx := context.Background()
	decision, e := client.Fraud.LatestForPayment(ctx, "pay_1")
	if e != nil || string(decision) != `{"traceId":"trace-2","decision":"allow"}` {
		t.Fatalf("latest decision %s %v", decision, e)
	}
	decision, e = client.Fraud.LatestForPayment(ctx, "pay_empty")
	if e != nil || decision != nil {
		t.Fatalf("no decision should be nil: %s %v", decision, e)
	}
	_, e = client.Fraud.LatestForPayment(ctx, "")
	mustReject(t, "empty payment id", e)
	sent := requests()
	if len(sent) != 2 || sent[0].Method != "GET" || sent[0].URI != "/api/v1/fraud/decision-logs?limit=1&paymentId=pay_1" || !sent[0].Signed {
		t.Fatalf("latest requests %+v", sent)
	}
}

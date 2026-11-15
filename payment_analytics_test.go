package lunixi

import (
	"context"
	"net/url"
	"testing"
)

func TestPaymentAnalytics(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.FailoverRecoveryAnalytics(ctx, url.Values{"from": {"2026-11-01"}, "to": {"2026-11-15"}}); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.FailoverRecoveryAnalytics(ctx, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.ProviderCredentialAnalytics(ctx, "cred_1", url.Values{"range": {"7d"}}); e != nil {
		t.Fatal(e)
	}
	_, e := client.Payments.ProviderCredentialAnalytics(ctx, "", nil)
	mustReject(t, "empty credential", e)
	sent := requests()
	want := []string{
		"/api/v1/payments/analytics/failover-recovery?from=2026-11-01&to=2026-11-15",
		"/api/v1/payments/analytics/failover-recovery",
		"/api/v1/payments/analytics/provider-credentials/cred_1?range=7d",
	}
	if len(sent) != len(want) {
		t.Fatalf("requests %+v", sent)
	}
	for i, r := range sent {
		if r.Method != "GET" || r.URI != want[i] || !r.Signed {
			t.Fatalf("request %d: %+v", i, r)
		}
	}
}

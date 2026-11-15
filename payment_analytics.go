package lunixi

import (
	"context"
	"net/url"
)

// FailoverRecoveryAnalytics reports the volume and count of payments rescued
// by provider failover retries (GET /payments/analytics/failover-recovery).
// filters are the documented reporting filters (for example from, to, range);
// nil sends none.
func (p *PaymentClient) FailoverRecoveryAnalytics(ctx context.Context, filters url.Values) (*Response, error) {
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/analytics/failover-recovery", Query: filters})
}

// ProviderCredentialAnalytics reports the performance of one provider
// credential (GET /payments/analytics/provider-credentials/{credentialId}).
func (p *PaymentClient) ProviderCredentialAnalytics(ctx context.Context, credentialID string, filters url.Values) (*Response, error) {
	id, err := idPath(credentialID)
	if err != nil {
		return nil, invalid("credentialId")
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/analytics/provider-credentials/" + id, Query: filters})
}

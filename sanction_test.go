package lunixi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestSanctionScreenNameBody(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"status":"success","data":{"screeningRequestId":"sr-1","matches":[],"coverageStatus":"COMPLETE"},"requestId":"req-1"}`)
	})
	result, err := client.Sanction.ScreenName(context.Background(), map[string]any{
		"riskDomains":       []string{SanctionRiskDomainSanctions, SanctionRiskDomainPep},
		"name":              "Ada Lovelace",
		"subjectType":       SanctionSubjectTypePerson,
		"identifiers":       []map[string]any{{"value": "12345678901", "type": "TCKN"}},
		"threshold":         0.85,
		"customerReference": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !IsClearScreening(result) {
		t.Fatalf("result %s", result)
	}
	sent := requests()
	if len(sent) != 1 || sent[0].Method != "POST" || sent[0].URI != "/api/v1/sanction/screening/name" || sent[0].Key != "" {
		t.Fatalf("request %+v", sent)
	}
	want := `{"name":"Ada Lovelace","subjectType":"person","identifiers":[{"type":"TCKN","value":"12345678901"}],"riskDomains":["sanctions","pep"],"threshold":0.85}`
	if string(sent[0].Body) != want {
		t.Fatalf("body %s", sent[0].Body)
	}
}

func TestSanctionRefusesBeforeSending(t *testing.T) {
	cases := map[string]func(*Client) error{
		"unknown field": func(c *Client) error {
			_, err := c.Sanction.ScreenName(context.Background(), map[string]any{"name": "Ada", "nationality": "TR"})
			return err
		},
		"missing name": func(c *Client) error {
			_, err := c.Sanction.ScreenName(context.Background(), map[string]any{"country": "TR"})
			return err
		},
		"risk domain": func(c *Client) error {
			_, err := c.Sanction.ScreenName(context.Background(), map[string]any{"name": "Ada", "riskDomains": []string{"sanctions", "crime"}})
			return err
		},
		"mode": func(c *Client) error {
			_, err := c.Sanction.ScreenName(context.Background(), map[string]any{"name": "Ada", "mode": SanctionWalletScreenModeRisk})
			return err
		},
		"wallet mode": func(c *Client) error {
			_, err := c.Sanction.ScreenWallet(context.Background(), map[string]any{"walletAddress": "0xabc", "mode": SanctionScreenModeFullRisk})
			return err
		},
		"identifier field": func(c *Client) error {
			_, err := c.Sanction.ScreenName(context.Background(), map[string]any{"name": "Ada", "identifiers": []map[string]any{{"type": "TCKN", "value": "1", "issuer": "TR"}}})
			return err
		},
		"identifier value missing": func(c *Client) error {
			_, err := c.Sanction.ScreenName(context.Background(), map[string]any{"name": "Ada", "identifiers": []map[string]any{{"type": "TCKN"}}})
			return err
		},
		"transaction without parties or accounts": func(c *Client) error {
			_, err := c.Sanction.ScreenTransaction(context.Background(), map[string]any{"amount": "100", "parties": []any{}})
			return err
		},
		"transaction party name": func(c *Client) error {
			_, err := c.Sanction.ScreenTransaction(context.Background(), map[string]any{"parties": []map[string]any{{"role": "payer"}}})
			return err
		},
		"procurement without subject": func(c *Client) error {
			_, err := c.Sanction.ScreenPublicProcurementBan(context.Background(), map[string]any{"onlyActive": true})
			return err
		},
		"monitoring subject type": func(c *Client) error {
			_, err := c.Sanction.RegisterMonitoring(context.Background(), map[string]any{"externalCustomerId": "c-1", "subjectType": "vessel"})
			return err
		},
		"profile and profileJson": func(c *Client) error {
			_, err := c.Sanction.RegisterMonitoring(context.Background(), map[string]any{"externalCustomerId": "c-1", "profile": map[string]any{"name": "Ada"}, "profileJson": "{}"})
			return err
		},
		"unknown filter": func(c *Client) error {
			_, err := c.Sanction.ListMonitoringAlerts(context.Background(), map[string]any{"severity": "high"})
			return err
		},
		"entity id": func(c *Client) error {
			_, err := c.Sanction.GetEntityGraph(context.Background(), "", nil)
			return err
		},
	}
	for name, call := range cases {
		client, requests := newProductClient(t, 0, nil)
		expectLocalError(t, name, call(client), requests())
	}
}

func TestSanctionScreeningThatDidNotRunIsAnError(t *testing.T) {
	client, _ := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"status":"failure","code":"sanction.screen_name.failed","data":{"matches":[]}}`)
	})
	_, err := client.Sanction.ScreenName(context.Background(), map[string]any{"name": "Ada"})
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Code != "sanction.screen_name.failed" || apiError.StatusCode != 200 {
		t.Fatalf("error %v", err)
	}
	client, _ = newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 503, `{"status":"failure","code":"sanction.screen_wallet.failed"}`)
	})
	_, err = client.Sanction.ScreenWallet(context.Background(), map[string]any{"walletAddress": "0xabc"})
	if !errors.As(err, &apiError) || apiError.StatusCode != 503 {
		t.Fatalf("error %v", err)
	}
}

func TestSanctionIsClearScreening(t *testing.T) {
	cases := map[string]bool{
		`{"matches":[],"coverageStatus":"COMPLETE"}`:                   true,
		`{"matches":[],"coverageStatus":"PARTIAL"}`:                    false,
		`{"matches":[],"coverageStatus":"DEGRADED"}`:                   false,
		`{"matches":[],"coverageStatus":null}`:                         false,
		`{"matches":[{"entityId":"e-1"}],"coverageStatus":"COMPLETE"}`: false,
		`{"hits":[],"publicationHits":[{"listedAddress":"0xabc"}]}`:    false,
		`{"hits":[],"isSanctioned":true}`:                              false,
		`{"matched":true}`:                                             false,
		`{"caseRequired":true,"coverageStatus":"COMPLETE"}`:            false,
		`{"decision":"REVIEW"}`:                                        false,
		`{"decision":"allow","hits":[]}`:                               true,
		`{"hits":[]}`:                                                  true,
		`null`:                                                         false,
		`[]`:                                                           false,
	}
	for raw, want := range cases {
		if got := IsClearScreening(json.RawMessage(raw)); got != want {
			t.Errorf("%s: %v, want %v", raw, got, want)
		}
	}
}

func TestSanctionMonitoringProfileAndQueries(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"status":"success","data":{"items":[{"id":"m-1"}],"nextCursor":"c2"},"pageInfo":{"hasMore":true,"totalCount":7}}`)
	})
	ctx := context.Background()
	if _, err := client.Sanction.RegisterMonitoring(ctx, map[string]any{"externalCustomerId": "c-1", "profile": map[string]any{"name": "Ada", "country": "TR"}, "riskDomainScope": []string{SanctionRiskDomainSanctions}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Sanction.GetIndirectExposure(ctx, "ent-1", map[string]any{"maxDepth": 2, "entityIds": []string{"ent-2", "ent-3"}}); err != nil {
		t.Fatal(err)
	}
	page, err := client.Sanction.ListMonitoredEntities(ctx, map[string]any{"activeOnly": false, "includeTotal": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor != "c2" || !page.HasMore || page.TotalCount == nil || *page.TotalCount != 7 {
		t.Fatalf("page %+v", page)
	}
	sent := requests()
	if want := `{"externalCustomerId":"c-1","profileJson":"{\"country\":\"TR\",\"name\":\"Ada\"}","riskDomainScope":["sanctions"]}`; sent[0].URI != "/api/v1/sanction/monitoring/register" || string(sent[0].Body) != want {
		t.Fatalf("register %s %s", sent[0].URI, sent[0].Body)
	}
	if sent[1].Method != "GET" || sent[1].URI != "/api/v1/sanction/intelligence/indirect-exposure/ent-1?entityIds=ent-2%2Cent-3&maxDepth=2" {
		t.Fatalf("exposure %+v", sent[1])
	}
	if sent[2].URI != "/api/v1/sanction/monitoring/entities?activeOnly=false&includeTotal=true" {
		t.Fatalf("entities %s", sent[2].URI)
	}
}

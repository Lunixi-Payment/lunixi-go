package lunixi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestIdentityRoutesAreBearerOnly(t *testing.T) {
	for name, route := range IdentityRoutes() {
		if route.StepUp || !strings.HasPrefix(route.Path, "/api/v1/identity/live-sessions") {
			t.Errorf("%s: %+v", name, route)
		}
	}
}

func TestIdentityAnalyze(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"traceId":"tr_1","analysisStatus":"completed","riskFlags":["same_device_cluster"]}}`)
	})
	ctx := context.Background()
	analysis, err := client.Identity.AnalyzeSync(ctx, map[string]any{
		"subjectType": "user", "subjectId": "42", "device": map[string]any{"deviceId": "fp_1", "ipAddress": "203.0.113.7"}, "account": map[string]any{"accountId": "42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(analysis), "tr_1") {
		t.Fatalf("analysis %s", analysis)
	}
	if _, err = client.Identity.Analyze(ctx, map[string]any{"paymentId": "pay_1"}); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/identity/live-sessions/analyze-sync" || sent[0].Signed || !strings.Contains(string(sent[0].Body), `"device":{"deviceId":"fp_1","ipAddress":"203.0.113.7"}`) {
		t.Fatalf("request %+v body %s", sent[0], sent[0].Body)
	}
	if sent[1].URI != "/api/v1/identity/live-sessions/analyze" || string(sent[1].Body) != `{"paymentId":"pay_1"}` {
		t.Fatalf("request %+v body %s", sent[1], sent[1].Body)
	}
	for name, request := range map[string]map[string]any{
		"subject pair":  {"subjectId": "42"},
		"unknown field": {"verdict": "allow"},
		"device object": {"device": "fp_1"},
	} {
		fresh, recorded := newProductClient(t, 0, nil)
		_, err := fresh.Identity.AnalyzeSync(ctx, request)
		expectLocalError(t, name, err, recorded())
	}
}

func TestIdentityAccountLinksHelpers(t *testing.T) {
	links := json.RawMessage(`{"totalLinkedSessions":3,"linkedAccounts":["h_a"],"sessions":[{"linkReasons":["same_fingerprint"]},{"linkReasons":["same_fingerprint","same_ip_asn"]}]}`)
	if !IsIdentityDeviceShared(links, 2) || IsIdentityDeviceShared(links, 3) || IsIdentityDeviceShared(json.RawMessage(`{"linkedAccounts":[]}`), 0) {
		t.Fatal("device sharing")
	}
	if reasons := IdentityLinkReasons(links); !slices.Equal(reasons, []string{"same_fingerprint", "same_ip_asn"}) {
		t.Fatalf("reasons %v", reasons)
	}
}

func TestIdentityReadsAndLists(t *testing.T) {
	client, requests := newProductClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		productJSON(w, 200, `{"success":true,"data":{"items":[{"traceId":"tr_1"}]}}`)
	})
	ctx := context.Background()
	steps := []func() error{
		func() error { _, err := client.Identity.GetSession(ctx, "tr_1"); return err },
		func() error { _, err := client.Identity.RelatedSessions(ctx, "tr_1", 5); return err },
		func() error { _, err := client.Identity.Timeline(ctx, "tr_1"); return err },
		func() error { _, err := client.Identity.AccountLinks(ctx, "tr_1"); return err },
		func() error {
			_, err := client.Identity.TopSources(ctx, map[string]any{"days": 7, "limit": 10})
			return err
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	page, err := client.Identity.ListSessions(ctx, map[string]any{"deviceId": "fp_1"})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page %+v %v", page, err)
	}
	sent := requests()
	for i, uri := range []string{
		"/api/v1/identity/live-sessions/tr_1",
		"/api/v1/identity/live-sessions/tr_1/related-sessions?limit=5",
		"/api/v1/identity/live-sessions/tr_1/timeline",
		"/api/v1/identity/live-sessions/tr_1/account-links",
		"/api/v1/identity/live-sessions/top-sources?days=7&limit=10",
		"/api/v1/identity/live-sessions?deviceId=fp_1",
	} {
		if sent[i].Method != "GET" || sent[i].URI != uri || sent[i].Signed {
			t.Fatalf("request %d: %+v", i, sent[i])
		}
	}
	fresh, recorded := newProductClient(t, 0, nil)
	_, err = fresh.Identity.ListSessions(ctx, map[string]any{"email": "ada@example.com"})
	expectLocalError(t, "unknown filter", err, recorded())
}

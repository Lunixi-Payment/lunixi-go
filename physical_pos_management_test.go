package lunixi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestPhysicalPosManagementKeyedWrites(t *testing.T) {
	client, requests := newPosClient(t, 0, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			posJSON(w, 201, `{"success":true,"data":{"store":{"storeId":"st-1","rowVersion":1},"replayed":false}}`)
			return
		}
		posJSON(w, 200, `{"success":true,"data":{"device":{"deviceId":"dv-1"},"closedAssignment":{"assignmentId":"as-1"},"replayed":true}}`)
	})
	ctx := context.Background()
	created, err := client.PhysicalPosManagement.CreateStore(ctx, map[string]any{"code": "IST-01", "name": "Kadıköy", "city": "", "district": nil}, "")
	if err != nil || created.StatusCode != 201 || created.Replayed || !uuidPattern.MatchString(created.IdempotencyKey) || !strings.Contains(string(created.Record("store")), "st-1") {
		t.Fatalf("%+v %v", created, err)
	}
	closed, err := client.PhysicalPosManagement.CloseDeviceAssignment(ctx, "dv-1", map[string]any{"rowVersion": 3, "reasonCode": "STORAGE"}, "close-key-01")
	if err != nil || closed.StatusCode != 200 || !closed.Replayed || closed.IdempotencyKey != "close-key-01" || closed.Record("closedAssignment") == nil {
		t.Fatalf("%+v %v", closed, err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/physical-pos/stores" || sent[0].Key != created.IdempotencyKey || string(sent[0].Body) != `{"code":"IST-01","name":"Kadıköy"}` {
		t.Fatalf("create %+v %s", sent[0], sent[0].Body)
	}
	if sent[1].URI != "/api/v1/physical-pos/devices/dv-1/assignment/close" || string(sent[1].Body) != `{"rowVersion":3,"reasonCode":"STORAGE"}` {
		t.Fatalf("close %+v %s", sent[1], sent[1].Body)
	}
}

func TestPhysicalPosManagementUpdatesAndRecords(t *testing.T) {
	client, requests := newPosClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		switch {
		case r.Method == "PATCH":
			posJSON(w, 200, `{"success":true,"data":{"store":{"storeId":"st-1","rowVersion":2}}}`)
		case strings.Contains(r.URL.Path, "/keys/"):
			posJSON(w, 200, `{"success":true,"data":{"key":{"keyId":"k-1"},"lifecycleStatus":"SUSPENDED","replayed":false}}`)
		default:
			posJSON(w, 200, `{"success":true,"data":{"device":{"deviceId":"dv-1"}}}`)
		}
	})
	ctx := context.Background()
	store, err := client.PhysicalPosManagement.UpdateStore(ctx, "st-1", map[string]any{"rowVersion": 1, "city": nil, "addressLine": "", "name": "Moda"})
	if err != nil || !strings.Contains(string(store), `"rowVersion":2`) || strings.Contains(string(store), `"store"`) {
		t.Fatalf("%s %v", store, err)
	}
	revoked, err := client.PhysicalPosManagement.RevokeDeviceKey(ctx, "dv-1", "k-1", map[string]any{"rowVersion": 4, "reasonCode": "DEVICE_LOST"})
	if err != nil || !strings.Contains(string(revoked), "lifecycleStatus") {
		t.Fatalf("%s %v", revoked, err)
	}
	device, err := client.PhysicalPosManagement.GetDevice(ctx, "dv-1")
	if err != nil || string(device) != `{"deviceId":"dv-1"}` {
		t.Fatalf("%s %v", device, err)
	}
	sent := requests()
	if sent[0].Method != "PATCH" || sent[0].URI != "/api/v1/physical-pos/stores/st-1" || string(sent[0].Body) != `{"rowVersion":1,"name":"Moda","city":null}` {
		t.Fatalf("update %+v %s", sent[0], sent[0].Body)
	}
	if sent[1].URI != "/api/v1/physical-pos/devices/dv-1/keys/k-1/revoke" || sent[1].Key != "" {
		t.Fatalf("revoke %+v", sent[1])
	}
	if sent[2].Method != "GET" || len(sent[2].Body) != 0 {
		t.Fatalf("get %+v", sent[2])
	}
}

func TestPhysicalPosManagementSettlementPolicies(t *testing.T) {
	client, requests := newPosClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 200, `{"success":true,"data":{"policy":{"mode":"SCHEDULED","rowVersion":5}}}`)
	})
	ctx := context.Background()
	policy, err := client.PhysicalPosManagement.SetStoreSettlementPolicy(ctx, "st-1", map[string]any{"mode": "SCHEDULED", "scheduleTime": "23:30", "scheduleDays": []string{"MON", "TUE"}, "expectedRowVersion": 4})
	if err != nil || string(policy) != `{"mode":"SCHEDULED","rowVersion":5}` {
		t.Fatalf("%s %v", policy, err)
	}
	if _, err = client.PhysicalPosManagement.ClearDeviceSettlementPolicy(ctx, "dv-1", 5); err != nil {
		t.Fatal(err)
	}
	if _, err = client.PhysicalPosManagement.ClearAccountSettlementPolicy(ctx, 0); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if sent[0].Method != "PUT" || sent[0].URI != "/api/v1/physical-pos/stores/st-1/settlement-policy" || string(sent[0].Body) != `{"mode":"SCHEDULED","scheduleTime":"23:30","scheduleDays":["MON","TUE"],"expectedRowVersion":4}` {
		t.Fatalf("set %+v %s", sent[0], sent[0].Body)
	}
	if sent[1].Method != "DELETE" || sent[1].URI != "/api/v1/physical-pos/devices/dv-1/settlement-policy?expectedRowVersion=5" || len(sent[1].Body) != 0 {
		t.Fatalf("clear device %+v", sent[1])
	}
	if sent[2].URI != "/api/v1/physical-pos/settlement-policy" {
		t.Fatalf("clear account %+v", sent[2])
	}
}

func TestPhysicalPosManagementChecksBeforeSending(t *testing.T) {
	client, requests := newPosClient(t, 0, nil)
	ctx := context.Background()
	m := client.PhysicalPosManagement
	checks := map[string]error{}
	_, checks["row version zero"] = m.UpdateStore(ctx, "st-1", map[string]any{"rowVersion": 0})
	_, checks["row version too large"] = m.UpdateRegister(ctx, "rg-1", map[string]any{"rowVersion": PhysicalPosRowVersionMax + 1})
	_, checks["row version text"] = m.ArchiveStore(ctx, "st-1", map[string]any{"rowVersion": "1", "reasonCode": "CLOSED"})
	_, checks["missing reason"] = m.RestoreStore(ctx, "st-1", map[string]any{"rowVersion": 1})
	_, checks["unknown field"] = m.CreateTerminal(ctx, map[string]any{"code": "T1", "displayName": "T", "environment": "LIVE"}, "")
	_, checks["empty currencies"] = m.AddTerminalAcquirer(ctx, "tm-1", map[string]any{"vendorAcquirerRef": "a", "merchantNumber": "m", "terminalNumber": "t", "currencies": []string{""}}, "")
	_, checks["bad key"] = m.CreateDevice(ctx, map[string]any{"vendorCode": "v", "deviceModelId": "d", "serialNumber": "s", "integrationChannel": "c"}, "short")
	_, checks["policy row version"] = m.ClearStoreSettlementPolicy(ctx, "st-1", -1)
	_, checks["policy minutes text"] = m.SetAccountSettlementPolicy(ctx, map[string]any{"mode": "SCHEDULED", "retryWindowMinutes": "30"})
	_, checks["unknown filter"] = m.ListDevices(ctx, map[string]any{"serial": "x"})
	_, checks["history filter"] = m.ListDeviceAudit(ctx, "dv-1", map[string]any{"includeTotal": true})
	_, checks["empty id"] = m.GetStore(ctx, "")
	_, checks["slash id"] = m.GetTerminal(ctx, "a/b")
	_, checks["card data key"] = m.CreateStore(ctx, map[string]any{"code": "S", "name": "N", "cardNumber": "x"}, "")
	for name, err := range checks {
		if !errors.Is(err, ErrConfiguration) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	if len(requests()) != 0 {
		t.Fatal("a refused request was sent")
	}
}

func TestPhysicalPosManagementPagesAndCatalog(t *testing.T) {
	client, requests := newPosClient(t, 0, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/physical-pos/catalog/") {
			posJSON(w, 200, `{"success":true,"data":[{"vendorCode":"V1"}]}`)
			return
		}
		posJSON(w, 200, `{"success":true,"data":{"items":[{"registerId":"rg-1"}]},"pageInfo":{"hasMore":true,"nextCursor":"n1","totalCount":9}}`)
	})
	ctx := context.Background()
	page, err := client.PhysicalPosManagement.ListRegisters(ctx, "st-1", map[string]any{"includeTotal": true, "status": "ACTIVE"})
	if err != nil || len(page.Items) != 1 || !page.HasMore || page.NextCursor != "n1" || page.TotalCount == nil || *page.TotalCount != 9 {
		t.Fatalf("%+v %v", page, err)
	}
	vendors, err := client.PhysicalPosManagement.ListVendors(ctx)
	if err != nil || string(vendors) != `[{"vendorCode":"V1"}]` {
		t.Fatalf("%s %v", vendors, err)
	}
	if _, err = client.PhysicalPosManagement.ListDeviceModels(ctx, map[string]any{"vendorCode": "V1"}); err != nil {
		t.Fatal(err)
	}
	sent := requests()
	if sent[0].URI != "/api/v1/physical-pos/stores/st-1/registers?includeTotal=true&status=ACTIVE" || sent[2].URI != "/api/v1/physical-pos/catalog/device-models?vendorCode=V1" {
		t.Fatalf("%+v", sent)
	}
}

func TestPhysicalPosManagementWriteErrorCarriesKey(t *testing.T) {
	client, _ := newPosClient(t, 0, func(w http.ResponseWriter, _ *http.Request, _ int) {
		posJSON(w, 409, `{"success":false,"code":"physical_pos.idempotency_mismatch"}`)
	})
	_, err := client.PhysicalPosManagement.CreateRegister(context.Background(), "st-1", map[string]any{"code": "K1", "name": "Till"}, "")
	var write *PhysicalPosWriteError
	var apiErr *APIError
	if !errors.As(err, &write) || !uuidPattern.MatchString(write.IdempotencyKey) || !errors.As(err, &apiErr) || apiErr.Code != "physical_pos.idempotency_mismatch" {
		t.Fatalf("%v", err)
	}
}

package lunixi

import (
	"errors"
	"testing"
	"time"
)

const testPosWebhookSecret = "whsec_pos_test_secret_0001"

func posWebhook(t *testing.T, body string) (*PhysicalPosWebhookEvent, error) {
	t.Helper()
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	verifier, err := NewWebhookVerifier(WebhookConfig{Secrets: []string{testPosWebhookSecret}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	headers := webhookHeaders([]byte(body), "evt-1", now.Format(time.RFC3339), testPosWebhookSecret)
	return verifier.VerifyPhysicalPos([]byte(body), headers)
}

func TestPhysicalPosWebhookPaymentEvent(t *testing.T) {
	event, err := posWebhook(t, `{"id":"evt-1","type":"physical_pos.payment.approved","environment":"LIVE","createdAt":"2026-11-15T09:59:58Z","data":{"paymentId":"pp-2","operation":"REFUND","historySeq":7,"amountMinor":"1500","original":{"paymentId":"pp-1","origin":"DEVICE_DIRECT"},"cashier":{"cashierId":"c0","cashierCode":"K01"},"items":[{"lineTotalMinor":"1500"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "evt-1" || event.Type != PhysicalPosEventPaymentApproved || event.Category != PhysicalPosWebhookCategoryPayment || event.Environment != "LIVE" || event.CreatedAt != "2026-11-15T09:59:58Z" || event.IsDeviceSale {
		t.Fatalf("%+v", event)
	}
	if event.HistorySeq == nil || *event.HistorySeq != 7 {
		t.Fatal("historySeq")
	}
	if event.Original == nil || *event.Original != (PhysicalPosOriginalPayment{PaymentID: "pp-1", Origin: "DEVICE_DIRECT"}) {
		t.Fatal("original")
	}
	if event.Cashier == nil || *event.Cashier != (PhysicalPosCashier{CashierID: "c0", CashierCode: "K01"}) {
		t.Fatal("cashier")
	}
	var data map[string]any
	if event.DecodeData(&data) != nil || data["amountMinor"] != "1500" {
		t.Fatal("data")
	}
}

func TestPhysicalPosWebhookSettlementAndDeviceSale(t *testing.T) {
	event, err := posWebhook(t, `{"id":"evt-1","type":"physical_pos.settlement.completed","data":{"deviceId":"dv-1","lines":[{"totals":{"saleMinor":"1000"},"difference":{"netMinor":"-200"}}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if event.Category != PhysicalPosWebhookCategorySettlement || event.HistorySeq != nil || event.Cashier != nil || event.Original != nil {
		t.Fatalf("%+v", event)
	}
	sale, err := posWebhook(t, `{"id":"evt-1","type":"physical_pos.payment.device_sale","payloadJson":"{\"paymentId\":\"pp-9\",\"origin\":\"DEVICE_DIRECT\",\"historySeq\":1}"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !sale.IsDeviceSale || string(sale.Data) != `{"paymentId":"pp-9","origin":"DEVICE_DIRECT","historySeq":1}` || sale.HistorySeq == nil || *sale.HistorySeq != 1 {
		t.Fatalf("%+v %s", sale, sale.Data)
	}
}

func TestPhysicalPosWebhookRejections(t *testing.T) {
	for name, body := range map[string]string{
		"other product":   `{"id":"evt-1","type":"payment.succeeded","data":{"paymentId":"p"}}`,
		"numeric amount":  `{"id":"evt-1","type":"physical_pos.payment.approved","data":{"amountMinor":1500}}`,
		"no data":         `{"id":"evt-1","type":"physical_pos.payment.approved","data":{}}`,
		"bad payloadJson": `{"id":"evt-1","type":"physical_pos.payment.approved","payloadJson":"{not json"}`,
	} {
		if _, err := posWebhook(t, body); !errors.Is(err, ErrWebhook) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	body := `{"id":"evt-1","type":"physical_pos.payment.approved","data":{"paymentId":"p"}}`
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	verifier, _ := NewWebhookVerifier(WebhookConfig{Secrets: []string{testPosWebhookSecret}, Now: func() time.Time { return now }})
	headers := webhookHeaders([]byte(body), "evt-1", now.Format(time.RFC3339), "whsec_another_secret_000")
	if _, err := verifier.VerifyPhysicalPos([]byte(body), headers); !errors.Is(err, ErrWebhook) {
		t.Fatal("unsigned body read")
	}
	if _, err := ParsePhysicalPosWebhookEvent(nil); !errors.Is(err, ErrWebhook) {
		t.Fatal("nil event read")
	}
}

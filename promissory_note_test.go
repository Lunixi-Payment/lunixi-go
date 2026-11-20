package lunixi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func noteRequest(amount string) *DirectMethodPaymentRequest {
	return withTestOrder(NewDirectMethodPaymentRequest(amount, "TRY", "ORDER-77").WithMethodFields(PaymentMethodPromissoryNote, "note_provider", map[string]any{"debtorName": "Example Buyer"}), amount)
}

func TestPromissoryNotePlanRules(t *testing.T) {
	rows := []PromissoryNoteScheduleRow{{Amount: "60000", DueDate: "2027-01-15"}, {Amount: "60000", DueDate: "2027-02-15"}}
	tooMany := make([]PromissoryNoteScheduleRow, 25)
	for i := range tooMany {
		tooMany[i] = PromissoryNoteScheduleRow{Amount: "100", DueDate: "2027-03-01"}
	}
	rejected := map[string]*DirectMethodPaymentRequest{
		"schedule and noteCount": NewDirectMethodPaymentRequest("120000", "TRY", "ORDER-77").WithMethodFields(PaymentMethodPromissoryNote, "note_provider", map[string]any{"noteCount": "2"}).WithPromissoryNoteSchedule(rows),
		"no plan":                noteRequest("120000"),
		"total mismatch":         noteRequest("120001").WithPromissoryNoteSchedule(rows),
		"invalid date":           noteRequest("120000").WithPromissoryNoteSchedule([]PromissoryNoteScheduleRow{{Amount: "60000", DueDate: "2027-01-15"}, {Amount: "60000", DueDate: "2027-02-30"}}),
		"short date":             noteRequest("120000").WithPromissoryNoteSchedule([]PromissoryNoteScheduleRow{{Amount: "120000", DueDate: "2027-1-15"}}),
		"descending":             noteRequest("120000").WithPromissoryNoteSchedule([]PromissoryNoteScheduleRow{{Amount: "60000", DueDate: "2027-02-15"}, {Amount: "60000", DueDate: "2027-01-15"}}),
		"25 rows":                noteRequest("2500").WithPromissoryNoteSchedule(tooMany),
		"empty schedule":         noteRequest("120000").WithPromissoryNoteSchedule(nil),
		"zero amount row":        noteRequest("120000").WithPromissoryNoteSchedule([]PromissoryNoteScheduleRow{{Amount: "0", DueDate: "2027-01-15"}, {Amount: "120000", DueDate: "2027-02-15"}}),
		"25 notes":               noteRequest("120000").WithPromissoryNoteCount(25),
		"zero notes":             noteRequest("120000").WithPromissoryNoteCount(0),
		"schedule in fields":     NewDirectMethodPaymentRequest("120000", "TRY", "ORDER-77").WithMethodFields(PaymentMethodPromissoryNote, "note_provider", map[string]any{"schedule": "x"}),
	}
	for name, request := range rejected {
		_, e := request.Body()
		mustReject(t, name, e)
	}

	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	raw := func(block map[string]any) map[string]any {
		return withOrder(map[string]any{"paymentMethod": "PROMISSORY_NOTE", "amount": "120000", "currency": "TRY", "orderId": "ORDER-77", "promissoryNote": block}, "120000")
	}
	schedule := []any{map[string]any{"amount": "60000", "dueDate": "2027-01-15"}, map[string]any{"amount": "60000", "dueDate": "2027-02-15"}}
	for name, body := range map[string]map[string]any{
		"raw both":          raw(map[string]any{"providerCode": "note_provider", "noteCount": "2", "schedule": schedule}),
		"raw none":          raw(map[string]any{"providerCode": "note_provider"}),
		"raw mismatch":      {"paymentMethod": "PROMISSORY_NOTE", "amount": "120001", "currency": "TRY", "orderId": "ORDER-77", "promissoryNote": map[string]any{"schedule": schedule}},
		"raw row field":     raw(map[string]any{"schedule": []any{map[string]any{"amount": "120000", "dueDate": "2027-01-15", "note": "x"}}}),
		"raw numeric money": raw(map[string]any{"schedule": []any{map[string]any{"amount": 120000, "dueDate": "2027-01-15"}}}),
		"raw note count":    raw(map[string]any{"noteCount": "30"}),
	} {
		_, e := client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: body, IdempotencyKey: testKey})
		mustReject(t, name, e)
	}
	if len(requests()) != 0 {
		t.Fatal("invalid plan reached network")
	}

	if _, e := client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/direct/payment", Body: raw(map[string]any{"noteCount": 3}), IdempotencyKey: testKey}); e != nil {
		t.Fatalf("raw numeric noteCount: %v", e)
	}
}

func TestPromissoryNoteValidPlanBodies(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	rows := []PromissoryNoteScheduleRow{{Amount: "40000", DueDate: "2027-01-15"}, {Amount: "40000", DueDate: "2027-01-15"}, {Amount: "40000", DueDate: "2027-03-15"}}
	if _, e := client.Payments.DirectMethodPayment(ctx, noteRequest("120000").WithPromissoryNoteSchedule(rows), testKey); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.DirectMethodPayment(ctx, noteRequest("120000").WithPromissoryNoteCount(3), "attempt-0000000002"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.DirectMethodPayment(ctx, withTestOrder(NewDirectMethodPaymentRequest("120000", "TRY", "ORDER-78").WithMethodFields(PaymentMethodPromissoryNote, "", map[string]any{"noteCount": 6}), "120000"), "attempt-0000000003"); e != nil {
		t.Fatal(e)
	}
	sent := requests()
	if len(sent) != 3 {
		t.Fatalf("requests %+v", sent)
	}
	first := decodeBody(t, sent[0].Body)["promissoryNote"]
	want := map[string]any{"providerCode": "note_provider", "debtorName": "Example Buyer", "schedule": []any{
		map[string]any{"amount": "40000", "dueDate": "2027-01-15"},
		map[string]any{"amount": "40000", "dueDate": "2027-01-15"},
		map[string]any{"amount": "40000", "dueDate": "2027-03-15"},
	}}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("schedule block %v", first)
	}
	if got := decodeBody(t, sent[1].Body)["promissoryNote"].(map[string]any)["noteCount"]; got != "3" {
		t.Fatalf("noteCount %v", got)
	}
	if got := decodeBody(t, sent[2].Body)["promissoryNote"].(map[string]any)["noteCount"]; got != "6" {
		t.Fatalf("noteCount from fields %v", got)
	}
}

func TestPromissoryNoteDocumentIsBinary(t *testing.T) {
	pdf := []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\nexample document\n%%EOF")
	client, requests := newRecordingClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "pay_unavailable") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"success":false,"code":"payment.promissory_note.document_unavailable"}`)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdf)
	})
	ctx := context.Background()
	response, e := client.Payments.DownloadPromissoryNoteDocument(ctx, "pay_1", PromissoryNoteDocumentLocaleEN)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(response.Bytes, pdf) || response.ContentType != "application/pdf" || len(response.Data) != 0 {
		t.Fatalf("document not returned as bytes: %q %q", response.ContentType, response.Bytes)
	}
	if _, e = client.Payments.DownloadPromissoryNoteDocument(ctx, "pay_1", ""); e != nil {
		t.Fatal(e)
	}
	_, e = client.Payments.DownloadPromissoryNoteDocument(ctx, "pay_unavailable", "")
	var api *APIError
	if !errors.As(e, &api) || api.StatusCode != 503 || api.Code != "payment.promissory_note.document_unavailable" {
		t.Fatalf("error answer not read as JSON: %v", e)
	}
	_, e = client.Payments.DownloadPromissoryNoteDocument(ctx, "pay_1", "de-DE")
	mustReject(t, "unsupported locale", e)
	_, e = client.Payments.DownloadPromissoryNoteDocument(ctx, "", "")
	mustReject(t, "empty paymentId", e)
	sent := requests()
	if len(sent) != 3 || sent[0].URI != "/api/v1/payments/pay_1/promissory-notes/document?locale=en-GB" || sent[1].URI != "/api/v1/payments/pay_1/promissory-notes/document" {
		t.Fatalf("requests %+v", sent)
	}
	for _, r := range sent {
		if r.Accept != "application/pdf, application/json" || !r.Signed || r.Method != "GET" {
			t.Fatalf("document request %+v", r)
		}
	}
}

func TestPromissoryNoteListAndCancel(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.GetPromissoryNotes(ctx, "pay_1"); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.CancelPromissoryNotes(ctx, "pay_1", nil, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := client.Payments.CancelPromissoryNotes(ctx, "pay_1", []string{"SN-0001", "A1B2"}, testKey); e != nil {
		t.Fatal(e)
	}
	tooMany := make([]string, 25)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("SN-%04d", i)
	}
	for name, serials := range map[string][]string{"empty list": {}, "short serial": {"SN1"}, "invalid character": {"SN_0001"}, "25 serials": tooMany, "long serial": {strings.Repeat("A", 65)}} {
		_, e := client.Payments.CancelPromissoryNotes(ctx, "pay_1", serials, "")
		mustReject(t, name, e)
	}
	_, e := client.Payments.GetPromissoryNotes(ctx, "../pay")
	mustReject(t, "path traversal", e)
	sent := requests()
	if len(sent) != 3 {
		t.Fatalf("requests %+v", sent)
	}
	if sent[0].Method != "GET" || sent[0].URI != "/api/v1/payments/pay_1/promissory-notes" {
		t.Fatalf("list %+v", sent[0])
	}
	if sent[1].URI != "/api/v1/payments/pay_1/promissory-notes/cancel" || len(sent[1].Body) != 0 {
		t.Fatalf("cancel all %+v", sent[1])
	}
	if !reflect.DeepEqual(decodeBody(t, sent[2].Body), map[string]any{"serialNumbers": []any{"SN-0001", "A1B2"}}) || sent[2].IdempotencyKey != testKey {
		t.Fatalf("cancel some %+v", sent[2])
	}
	for _, r := range sent {
		if !r.Signed {
			t.Fatalf("not signed: %+v", r)
		}
	}
}

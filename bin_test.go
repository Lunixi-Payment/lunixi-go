package lunixi

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestBinInfoIsSignedAndChecksDigits(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	if _, e := client.Payments.BinInfo(ctx, "45436012"); e != nil {
		t.Fatal(e)
	}
	const pan = "4543601234567890"
	for _, bad := range []string{"", "12345", "4543-6012", pan + "1234", pan + "x"} {
		_, e := client.Payments.BinInfo(ctx, bad)
		mustReject(t, "binOrPan", e)
		if bad != "" && strings.Contains(e.Error(), bad) {
			t.Fatal("error message echoed the card number")
		}
	}
	sent := requests()
	if len(sent) != 1 {
		t.Fatalf("invalid values reached the network: %+v", sent)
	}
	if sent[0].Method != "POST" || sent[0].URI != "/api/v1/bin/info" || string(sent[0].Body) != `{"binOrPan":"45436012"}` || !sent[0].Signed || sent[0].IdempotencyKey != "" {
		t.Fatalf("bin info request %+v %s", sent[0], sent[0].Body)
	}
}

func TestInstallmentOptionsBodyAndEnabledInstallmentsRule(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	counts := []int{3, 1, 2}
	request := InstallmentOptionsRequest{Amount: "10000", Currency: "try", BinOrPan: "45436012", Installment: 3, EnabledInstallments: counts}
	if _, e := client.Payments.InstallmentOptions(ctx, request); e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(counts, []int{3, 1, 2}) {
		t.Fatal("caller slice was changed")
	}
	if _, e := client.Payments.InstallmentOptions(ctx, InstallmentOptionsRequest{IntentID: "intent-1"}); e != nil {
		t.Fatal(e)
	}
	for name, bad := range map[string]InstallmentOptionsRequest{
		"no amount or intent": {Currency: "TRY"},
		"decimal amount":      {Amount: "100.50", Currency: "TRY"},
		"currency":            {Amount: "10000", Currency: "TL"},
		"installment 37":      {Amount: "10000", Installment: 37},
		"bin":                 {Amount: "10000", BinOrPan: "4543"},
		"empty list":          {Amount: "10000", EnabledInstallments: []int{}},
		"list without 1":      {Amount: "10000", EnabledInstallments: []int{2, 3}},
		"duplicate":           {Amount: "10000", EnabledInstallments: []int{1, 2, 2}},
		"out of range":        {Amount: "10000", EnabledInstallments: []int{1, 37}},
	} {
		_, e := client.Payments.InstallmentOptions(ctx, bad)
		mustReject(t, name, e)
	}
	sent := requests()
	if len(sent) != 2 {
		t.Fatalf("invalid requests reached the network: %d", len(sent))
	}
	if sent[0].URI != "/api/v1/bin/installments" || !sent[0].Signed || string(sent[0].Body) != `{"amount":"10000","currency":"TRY","binOrPan":"45436012","installment":3,"enabledInstallments":[1,2,3]}` {
		t.Fatalf("installment body %s", sent[0].Body)
	}
	if string(sent[1].Body) != `{"intentId":"intent-1"}` {
		t.Fatalf("intent quote body %s", sent[1].Body)
	}
}

func TestCardTaxonomy(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	if _, e := client.Payments.CardTaxonomy(context.Background()); e != nil {
		t.Fatal(e)
	}
	sent := requests()
	if len(sent) != 1 || sent[0].Method != "GET" || sent[0].URI != "/api/v1/bin/card-taxonomy" || !sent[0].Signed || len(sent[0].Body) != 0 {
		t.Fatalf("card taxonomy request %+v", sent)
	}
}

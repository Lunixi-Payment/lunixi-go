package lunixi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
)

func TestEnabledInstallmentsRejectsBrokenListsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}, nil)
	for name, counts := range map[string][]int{
		"empty":     {},
		"without 1": {2, 3},
		"duplicate": {1, 2, 2},
		"zero":      {0, 1},
		"negative":  {1, -3},
		"above 36":  {1, 37},
	} {
		request := validIntent()
		request.EnabledInstallments = counts
		if _, e := client.Payments.CreateIntent(context.Background(), request, "attempt-0000000001"); !errors.Is(e, ErrConfiguration) {
			t.Fatalf("%s: accepted %v", name, counts)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid installment list reached network")
	}
}

func TestEnabledInstallmentsAcceptsValidLists(t *testing.T) {
	for _, counts := range [][]int{nil, {1}, {3, 1, 2}, {1, 36}} {
		request := validIntent()
		request.EnabledInstallments = counts
		if e := request.validate(); e != nil {
			t.Fatalf("rejected %v: %v", counts, e)
		}
	}
}

func TestEnabledInstallmentsSentSortedWithoutChangingCaller(t *testing.T) {
	var sent []int
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"at_TEST_key","expires_in":1800}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			EnabledInstallments []int `json:"enabledInstallments"`
		}
		if e := json.Unmarshal(raw, &body); e != nil {
			t.Error(e)
		}
		sent = body.EnabledInstallments
		fmt.Fprint(w, `{"success":true,"data":{"token":"ct_example"}}`)
	}, nil)
	request := validIntent()
	request.EnabledInstallments = []int{6, 1, 3}
	if _, e := client.Payments.CreateIntent(context.Background(), request, "attempt-0000000001"); e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(sent, []int{1, 3, 6}) {
		t.Fatalf("sent %v, want ascending order", sent)
	}
	if !slices.Equal(request.EnabledInstallments, []int{6, 1, 3}) {
		t.Fatal("caller slice was reordered")
	}
}

func TestEnabledInstallmentsOmittedWhenNil(t *testing.T) {
	raw, e := json.Marshal(validIntent())
	if e != nil {
		t.Fatal(e)
	}
	var body map[string]any
	if e = json.Unmarshal(raw, &body); e != nil {
		t.Fatal(e)
	}
	if _, present := body["enabledInstallments"]; present {
		t.Fatal("nil list must not be sent")
	}
}

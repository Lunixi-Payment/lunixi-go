package lunixi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
)

// testdata/physical-pos-parity.json is a byte-identical copy of the shared
// physical POS fixture of the Node, PHP, Java and Python SDKs. Its expected
// values come from the gateway rules (card-data key tokens, the guard's field
// paths, the PAN value scan) and the gateway's closed field and filter
// lists; a rule or list that drifts here turns these tests red.

type posParityFixture struct {
	CardDataKeyRule struct {
		NumberTerms, SecretTerms, SuffixTerms, WholeKeyTerms, MaskTokens []string
		DigitRunKeyPattern                                               string
	}
	CardDataKeyCases []struct {
		Key      string
		CardData bool
	}
	CardDataPathCases []struct {
		Body  map[string]any
		Field *string
	}
	PanValueCases []struct {
		Value, Mode string
		Pan         bool
	}
	IdempotencyKeyCases []struct {
		Key   string
		Valid bool
	}
	ForbiddenFields []string
	RequestFields   map[string][]string
	Filters         map[string][]string
	EventTypes      []string
	NotStartedCodes []string
	PathIDCases     []struct {
		ID      string
		Valid   bool
		Encoded string
	} `json:"pathIdCases"`
	WebhookCashierCases []struct {
		Name    string
		Data    map[string]any
		Cashier *PhysicalPosCashier
	}
	WebhookOriginalCases []struct {
		Name     string
		Data     map[string]any
		Original *PhysicalPosOriginalPayment
	}
	WebhookCategoryCases []struct {
		Type, Category string
	}
}

func posFixture(t *testing.T) posParityFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/physical-pos-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture posParityFixture
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestPhysicalPosParityCardDataRule(t *testing.T) {
	rule := posFixture(t).CardDataKeyRule
	for name, pair := range map[string][2][]string{
		"numberTerms":   {posCardNumberTerms, rule.NumberTerms},
		"secretTerms":   {posCardSecretTerms, rule.SecretTerms},
		"suffixTerms":   {posCardSuffixTerms, rule.SuffixTerms},
		"wholeKeyTerms": {posWholeKeyTerms, rule.WholeKeyTerms},
		"maskTokens":    {posMaskTokens, rule.MaskTokens},
	} {
		if !slices.Equal(pair[0], pair[1]) {
			t.Errorf("%s drifted: %v", name, pair[0])
		}
	}
	if posDigitRunKeyPattern != rule.DigitRunKeyPattern {
		t.Error("digit run key pattern drifted")
	}
}

func TestPhysicalPosParityCardDataKeys(t *testing.T) {
	for _, c := range posFixture(t).CardDataKeyCases {
		if got := posIsCardDataKey(c.Key); got != c.CardData {
			t.Errorf("posIsCardDataKey(%q) = %v, want %v", c.Key, got, c.CardData)
		}
	}
}

func TestPhysicalPosParityCardDataPaths(t *testing.T) {
	for _, c := range posFixture(t).CardDataPathCases {
		field, found := posFindCardDataField(c.Body, "(body)")
		if c.Field == nil {
			if found {
				t.Errorf("%v: found %q, want none", c.Body, field)
			}
			continue
		}
		if !found || field != *c.Field {
			t.Errorf("%v: got %q (%v), want %q", c.Body, field, found, *c.Field)
		}
	}
}

func TestPhysicalPosParityPanValues(t *testing.T) {
	for _, c := range posFixture(t).PanValueCases {
		if got := posContainsPanLikeValue(c.Value, c.Mode); got != c.Pan {
			t.Errorf("%s %q: got %v, want %v", c.Mode, c.Value, got, c.Pan)
		}
	}
}

func TestPhysicalPosParityIdempotencyKeys(t *testing.T) {
	for _, c := range posFixture(t).IdempotencyKeyCases {
		key, err := posCheckIdempotencyKey(c.Key)
		if c.Valid && (err != nil || key != c.Key) {
			t.Errorf("key %q refused or changed", c.Key)
		}
		if !c.Valid && !errors.Is(err, ErrConfiguration) {
			t.Errorf("key %q accepted", c.Key)
		}
	}
}

// Go sends path segments without escaping (the transport refuses percent
// signs in paths), so an id that would need escaping is refused locally.
func TestPhysicalPosParityPathIDs(t *testing.T) {
	for _, c := range posFixture(t).PathIDCases {
		id, err := posPathID(c.ID, "paymentId")
		switch {
		case c.Valid && c.Encoded == "":
			if err != nil || id != c.ID {
				t.Errorf("id %q refused", c.ID)
			}
		default:
			if !errors.Is(err, ErrConfiguration) {
				t.Errorf("id %q accepted", c.ID)
			}
		}
	}
}

func TestPhysicalPosParityFieldAndFilterLists(t *testing.T) {
	fixture := posFixture(t)
	for name, want := range fixture.RequestFields {
		var got []string
		switch name {
		case "paymentTarget":
			got = PhysicalPosPaymentTargetFields()
		case "saleItem":
			got = PhysicalPosSaleItemFields()
		case "refundItem":
			got = PhysicalPosRefundItemFields()
		default:
			got = PhysicalPosRequestFields(name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("request fields %s: %v, want %v", name, got, want)
		}
	}
	if len(fixture.RequestFields) != len(physicalPosRequestFields)+3 {
		t.Errorf("request field lists: %d in the SDK, %d in the fixture", len(physicalPosRequestFields)+3, len(fixture.RequestFields))
	}
	for i, kind := range physicalPosSaleItemKinds {
		if kind.name != physicalPosSaleItemFields[i] {
			t.Error("sale item kinds drifted from the sale item fields")
		}
	}
	for i, kind := range physicalPosRefundItemKinds {
		if kind.name != physicalPosRefundItemFields[i] {
			t.Error("refund item kinds drifted from the refund item fields")
		}
	}
	for route, want := range fixture.Filters {
		if got := PhysicalPosFilters(route); !slices.Equal(got, want) {
			t.Errorf("filters %s: %v, want %v", route, got, want)
		}
	}
	if len(fixture.Filters) != len(physicalPosFilters) {
		t.Errorf("filter routes: %d in the SDK, %d in the fixture", len(physicalPosFilters), len(fixture.Filters))
	}
	if !slices.Equal(PhysicalPosEventTypes(), fixture.EventTypes) {
		t.Error("event types drifted")
	}
	if !slices.Equal(PhysicalPosNotStartedCodes(), fixture.NotStartedCodes) {
		t.Error("not-started codes drifted")
	}
}

func TestPhysicalPosParityForbiddenFields(t *testing.T) {
	client, requests := newPosClient(t, 0, nil)
	for _, field := range posFixture(t).ForbiddenFields {
		request := posSale()
		request[field] = "x"
		if _, err := client.PhysicalPos.CreatePayment(context.Background(), request, testPosKey); !errors.Is(err, ErrConfiguration) {
			t.Errorf("field %s: %v", field, err)
		}
	}
	if len(requests()) != 0 {
		t.Fatal("a refused request was sent")
	}
}

func TestPhysicalPosParityWebhookReaders(t *testing.T) {
	fixture := posFixture(t)
	for _, c := range fixture.WebhookCashierCases {
		got := posWebhookCashier(c.Data)
		if (got == nil) != (c.Cashier == nil) || (got != nil && *got != *c.Cashier) {
			t.Errorf("cashier %s: %+v", c.Name, got)
		}
	}
	for _, c := range fixture.WebhookOriginalCases {
		got := posWebhookOriginal(c.Data)
		if (got == nil) != (c.Original == nil) || (got != nil && *got != *c.Original) {
			t.Errorf("original %s: %+v", c.Name, got)
		}
	}
	for _, c := range fixture.WebhookCategoryCases {
		if got := posWebhookCategory(c.Type); got != c.Category {
			t.Errorf("category %s: %s, want %s", c.Type, got, c.Category)
		}
	}
}

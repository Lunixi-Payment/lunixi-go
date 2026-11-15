package lunixi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// parityProducts are the catalog products this SDK covers. Other products of
// the shared catalog are not part of the Go surface yet and are not checked.
var parityProducts = []string{"payments", "fraud", "payment-links", "reference", "physical-pos", "wallet", "bank-connect", "sanction", "insurance", "subscriptions", "kyc", "identity", "marketplace"}

// TestCatalogParity reads the shared operation catalog of the repository
// (contracts/products) and checks that every required operation exists as a
// method on its client. Sub-clients are fields (wallet.endUsers.get is
// Client.Wallet.EndUsers.Get); where Go has no such field the method is on
// the parent (fraud.events.record is a method of Client.Fraud). Go
// initialisms apply (qr → QR, fx → FX, a trailing Id → ID). Names that
// differ on purpose come from contracts/sdk-names.json ("go"). The test is
// skipped outside the monorepo.
func TestCatalogParity(t *testing.T) {
	contracts := filepath.Join("..", "contracts")
	if _, err := os.Stat(filepath.Join(contracts, "products")); errors.Is(err, fs.ErrNotExist) {
		t.Skip("shared catalog not present (standalone SDK checkout)")
	}
	names := map[string]*string{}
	if raw, err := os.ReadFile(filepath.Join(contracts, "sdk-names.json")); err == nil {
		var all map[string]json.RawMessage
		if err = json.Unmarshal(raw, &all); err != nil {
			t.Fatalf("sdk-names.json: %v", err)
		}
		if goNames, ok := all["go"]; ok {
			if err = json.Unmarshal(goNames, &names); err != nil {
				t.Fatalf("sdk-names.json go: %v", err)
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	client, err := New(Config{BaseURL: "https://api.lunixi.example", KeyID: "mk_test_sk_example", PrivateKey: fixture(t).PrivateSeed})
	if err != nil {
		t.Fatal(err)
	}
	root := reflect.ValueOf(client).Elem()
	var missing []string
	for _, product := range parityProducts {
		raw, err := os.ReadFile(filepath.Join(contracts, "products", product+".json"))
		if err != nil {
			t.Fatalf("catalog %s: %v", product, err)
		}
		var catalog struct {
			Operations map[string]struct {
				Status string `json:"status"`
			} `json:"operations"`
		}
		if err = json.Unmarshal(raw, &catalog); err != nil {
			t.Fatalf("catalog %s: %v", product, err)
		}
		for operation, meta := range catalog.Operations {
			if meta.Status != "required" {
				continue
			}
			parts := strings.Split(operation, ".")
			method := goMethodName(parts[len(parts)-1])
			if override, ok := names[operation]; ok {
				if override == nil {
					continue
				}
				method = *override
			}
			path := "Client." + goSegmentName(parts[0])
			accessor := root.FieldByName(goSegmentName(parts[0]))
			if accessor.IsValid() && accessor.Kind() == reflect.Pointer && !accessor.IsNil() {
				for _, segment := range parts[1 : len(parts)-1] {
					next := accessor.Elem().FieldByName(goSegmentName(segment))
					if !next.IsValid() || next.Kind() != reflect.Pointer || next.IsNil() {
						break
					}
					accessor = next
					path += "." + goSegmentName(segment)
				}
			}
			if !accessor.IsValid() || accessor.Kind() != reflect.Pointer || accessor.IsNil() || !accessor.MethodByName(method).IsValid() {
				missing = append(missing, operation+" ("+path+"."+method+")")
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("required catalog operations missing from the Go SDK:\n%s", strings.Join(missing, "\n"))
	}
}

func exportedName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// goSegmentName is the Go field name of a catalog accessor segment.
func goSegmentName(name string) string {
	switch name {
	case "qr":
		return "QR"
	case "fx":
		return "FX"
	}
	return exportedName(name)
}

// goMethodName is the Go method name of a catalog operation leaf.
func goMethodName(name string) string {
	method := exportedName(name)
	if strings.HasSuffix(method, "Id") {
		method = strings.TrimSuffix(method, "Id") + "ID"
	}
	return method
}

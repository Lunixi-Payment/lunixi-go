package lunixi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The route tables, filter lists, field lists, value sets and event names
// of the wallet, bank-connect, sanction, insurance, subscription, KYC,
// identity and marketplace clients are compared
// with the shared catalog (contracts/products, generated from the Node and
// PHP SDKs). The tests are skipped outside the monorepo.

type catalogGroup struct {
	Kind   string          `json:"kind"`
	Values json.RawMessage `json:"values"`
}

type catalogRoute struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	IdempotencyKey *bool    `json:"idempotencyKey"`
	Required       []string `json:"required"`
	Optional       []string `json:"optional"`
	AnyOf          []string `json:"anyOf"`
	Query          []string `json:"query"`
	Binary         *bool    `json:"binary"`
	Screening      *bool    `json:"screening"`
}

func catalogGroups(t *testing.T, product string) map[string]catalogGroup {
	t.Helper()
	path := filepath.Join("..", "contracts", "products", product+".json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("shared catalog not present (standalone SDK checkout)")
	}
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Groups map[string]catalogGroup `json:"groups"`
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("catalog %s: %v", product, err)
	}
	return catalog.Groups
}

func catalogList(t *testing.T, groups map[string]catalogGroup, name string) []string {
	t.Helper()
	group, ok := groups[name]
	if !ok {
		t.Fatalf("catalog group %s missing", name)
	}
	var values []string
	if err := json.Unmarshal(group.Values, &values); err != nil {
		t.Fatalf("catalog group %s: %v", name, err)
	}
	return values
}

func catalogPattern(t *testing.T, groups map[string]catalogGroup, name string) string {
	t.Helper()
	var values struct {
		Pattern string `json:"$pattern"`
	}
	group, ok := groups[name]
	if !ok || json.Unmarshal(group.Values, &values) != nil || values.Pattern == "" {
		t.Fatalf("catalog pattern %s missing", name)
	}
	return values.Pattern
}

func sameList(a, b []string) bool {
	return slices.Equal(append([]string{}, a...), append([]string{}, b...))
}

func compareRouteTable(t *testing.T, product, group string, table map[string]Route) {
	t.Helper()
	groups := catalogGroups(t, product)
	var catalog map[string]catalogRoute
	if err := json.Unmarshal(groups[group].Values, &catalog); err != nil || len(catalog) == 0 {
		t.Fatalf("catalog %s %s: %v", product, group, err)
	}
	var problems []string
	for name, want := range catalog {
		got, ok := table[name]
		if !ok {
			problems = append(problems, name+": missing in Go")
			continue
		}
		flag := func(value *bool) bool { return value != nil && *value }
		switch {
		case got.Method != want.Method:
			problems = append(problems, name+": method "+got.Method+" != "+want.Method)
		case got.Path != want.Path:
			problems = append(problems, name+": path "+got.Path+" != "+want.Path)
		case got.IdempotencyKey != flag(want.IdempotencyKey):
			problems = append(problems, name+": idempotencyKey differs")
		case got.Binary != flag(want.Binary):
			problems = append(problems, name+": binary differs")
		case got.Screening != flag(want.Screening):
			problems = append(problems, name+": screening differs")
		case !sameList(got.Required, want.Required):
			problems = append(problems, name+": required "+strings.Join(got.Required, ",")+" != "+strings.Join(want.Required, ","))
		case !sameList(got.Optional, want.Optional):
			problems = append(problems, name+": optional "+strings.Join(got.Optional, ",")+" != "+strings.Join(want.Optional, ","))
		case !sameList(got.AnyOf, want.AnyOf):
			problems = append(problems, name+": anyOf differs")
		case !sameList(got.Query, want.Query):
			problems = append(problems, name+": query "+strings.Join(got.Query, ",")+" != "+strings.Join(want.Query, ","))
		}
	}
	for name := range table {
		if _, ok := catalog[name]; !ok {
			problems = append(problems, name+": not in the catalog")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("%s %s differs from the catalog:\n%s", product, group, strings.Join(problems, "\n"))
	}
}

func TestCatalogWalletRoutes(t *testing.T) {
	compareRouteTable(t, "wallet", "WALLET_ROUTES", WalletRoutes())
}

func TestCatalogSanctionRoutes(t *testing.T) {
	compareRouteTable(t, "sanction", "SANCTION_ROUTES", SanctionRoutes())
}

func TestCatalogBankTransferRoutes(t *testing.T) {
	compareRouteTable(t, "bank-connect", "BANK_TRANSFER_ROUTES", BankTransferRoutes())
}

func compareLists(t *testing.T, groups map[string]catalogGroup, lists map[string][]string) {
	t.Helper()
	for name, got := range lists {
		if want := catalogList(t, groups, name); !sameList(got, want) {
			t.Errorf("%s: Go %v, catalog %v", name, got, want)
		}
	}
}

func TestCatalogWalletValues(t *testing.T) {
	groups := catalogGroups(t, "wallet")
	compareLists(t, groups, map[string][]string{
		"WALLET_AMOUNT_FIELDS":             walletAmountFields,
		"WalletAccountType.ALL":            WalletAccountTypes(),
		"WalletEndUserStatus.ALL":          WalletEndUserStatuses(),
		"WalletKycLevel.ALL":               WalletKycLevels(),
		"WalletOtpChannel.ALL":             WalletOtpChannels(),
		"WalletQrType.ALL":                 WalletQRTypes(),
		"WalletQrImageFormat.ALL":          WalletQRImageFormats(),
		"WalletBulkTargetType.ALL":         WalletBulkTargetTypes(),
		"WalletCorporateAccountStatus.ALL": WalletCorporateAccountStatuses(),
	})
	if got, want := walletAmountPattern.String(), catalogPattern(t, groups, "AMOUNT_PATTERN"); got != want {
		t.Errorf("amount pattern %s, catalog %s", got, want)
	}
}

func TestCatalogBankConnectValues(t *testing.T) {
	groups := catalogGroups(t, "bank-connect")
	lists := map[string][]string{
		"BANK_CONNECT_EVENT_TYPES":     BankConnectEventTypes(),
		"BANK_TRANSFER_EVENT_TYPES":    BankTransferEventTypes(),
		"BankConnectionStatus.ALL":     BankConnectionStatuses(),
		"BankAccountStatus.ALL":        BankAccountStatuses(),
		"BankTransactionDirection.ALL": BankTransactionDirections(),
		"BankTransactionType.ALL":      BankTransactionTypes(),
		"BankCapability.ALL":           BankCapabilities(),
		"BankTransferStatus.ALL":       BankTransferStatuses(),
		"BankTransferRail.ALL":         BankTransferRails(),
		"BankFxOrderStatus.ALL":        BankFXOrderStatuses(),
	}
	for route, filters := range BankConnectFilters() {
		lists["BANK_CONNECT_FILTERS."+route] = filters
	}
	compareLists(t, groups, lists)
	for name := range groups {
		if route, ok := strings.CutPrefix(name, "BANK_CONNECT_FILTERS."); ok {
			if _, known := BankConnectFilters()[route]; !known {
				t.Errorf("catalog filter list %s has no Go counterpart", name)
			}
		}
	}
	if got, want := bankTransferPaymentPurposePattern.String(), catalogPattern(t, groups, "BANK_TRANSFER_PAYMENT_PURPOSE_PATTERN"); got != want {
		t.Errorf("payment purpose pattern %s, catalog %s", got, want)
	}
	checkKeyPattern(t, catalogPattern(t, groups, "BANK_TRANSFER_IDEMPOTENCY_KEY_PATTERN"))
}

func TestCatalogSanctionValues(t *testing.T) {
	groups := catalogGroups(t, "sanction")
	lists := map[string][]string{
		"ScreenMode.ALL":       SanctionScreenModes(),
		"WalletScreenMode.ALL": SanctionWalletScreenModes(),
		"SubjectType.ALL":      SanctionSubjectTypes(),
		"RiskDomain.ALL":       SanctionRiskDomains(),
		"CoverageStatus.ALL":   SanctionCoverageStatuses(),
		"HitType.ALL":          SanctionHitTypes(),
	}
	for field, values := range SanctionValueSets() {
		lists["SANCTION_VALUE_SETS."+field] = values
	}
	compareLists(t, groups, lists)
	for name := range groups {
		if field, ok := strings.CutPrefix(name, "SANCTION_VALUE_SETS."); ok {
			if _, known := SanctionValueSets()[field]; !known {
				t.Errorf("catalog value set %s has no Go counterpart", name)
			}
		}
	}
}

func TestCatalogInsuranceValues(t *testing.T) {
	groups := catalogGroups(t, "insurance")
	lists := map[string][]string{
		"INSURANCE_EVENT_TYPES":             InsuranceEventTypes(),
		"InsuranceEvents.ORDER_EVENTS":      InsuranceOrderEventTypes(),
		"InsuranceEvents.LEAD_EVENTS":       InsuranceLeadEventTypes(),
		"INSURANCE_QUOTE_FIELDS":            InsuranceQuoteFields(),
		"INSURANCE_LEAD_FIELDS":             InsuranceLeadFields(),
		"INSURANCE_PRODUCT_SEARCH_FIELDS":   InsuranceProductSearchFields(),
		"INSURANCE_ORDER_FIELDS":            InsuranceOrderFields(),
		"INSURANCE_CONSENT_FIELDS":          InsuranceConsentFields(),
		"INSURANCE_EXTERNAL_PAYMENT_FIELDS": InsuranceExternalPaymentFields(),
		"INSURANCE_EXTERNAL_REFUND_FIELDS":  InsuranceExternalRefundFields(),
		"InsuranceReferenceKind.ALL":        InsuranceReferenceKinds(),
		"InsuranceLeadStatus.ALL":           InsuranceLeadStatuses(),
		"InsuranceOrderStatus.ALL":          InsuranceOrderStatuses(),
		"InsurancePolicyStatus.ALL":         InsurancePolicyStatuses(),
	}
	for route, filters := range InsuranceFilters() {
		lists["INSURANCE_FILTERS."+route] = filters
	}
	compareLists(t, groups, lists)
	for name := range groups {
		if route, ok := strings.CutPrefix(name, "INSURANCE_FILTERS."); ok {
			if _, known := InsuranceFilters()[route]; !known {
				t.Errorf("catalog filter list %s has no Go counterpart", name)
			}
		}
	}
	checkKeyPattern(t, catalogPattern(t, groups, "INSURANCE_IDEMPOTENCY_KEY_PATTERN"))
}

// checkKeyPattern compares the catalog's Idempotency-Key pattern with the
// SDK's key rule on boundary keys.
func checkKeyPattern(t *testing.T, pattern string) {
	t.Helper()
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("pattern %s: %v", pattern, err)
	}
	for _, key := range []string{"", "1234567", "12345678", strings.Repeat("k", 200), strings.Repeat("k", 201), "key with space", "key\twith-tab", "ключ-ключ-ключ", "order-2026-000123:try"} {
		if compiled.MatchString(key) != validPrintableKey(key) {
			t.Errorf("key %q: catalog %v, SDK %v", key, compiled.MatchString(key), validPrintableKey(key))
		}
	}
}

// ── Subscriptions, KYC, identity and marketplace ─────────────────────────

// catalogTableRoute is a route of the SUBSCRIPTION_ROUTES, KYC_ROUTES,
// IDENTITY_ROUTES and MARKETPLACE_ROUTES groups (idempotencyKey is
// "required", "optional" or "none").
type catalogTableRoute struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	StepUp         bool     `json:"stepUp"`
	IdempotencyKey string   `json:"idempotencyKey"`
	Required       []string `json:"required"`
	Optional       []string `json:"optional"`
	Query          []string `json:"query"`
	Free           bool     `json:"free"`
	OptionalBody   bool     `json:"optionalBody"`
	AnyQuery       bool     `json:"anyQuery"`
}

func compareTableRoutes(t *testing.T, product, group string, table map[string]Route) {
	t.Helper()
	groups := catalogGroups(t, product)
	var catalog map[string]catalogTableRoute
	if err := json.Unmarshal(groups[group].Values, &catalog); err != nil || len(catalog) == 0 {
		t.Fatalf("catalog %s %s: %v", product, group, err)
	}
	var problems []string
	for name, want := range catalog {
		got, ok := table[name]
		if !ok {
			problems = append(problems, name+": missing in Go")
			continue
		}
		switch {
		case got.Method != want.Method:
			problems = append(problems, name+": method "+got.Method+" != "+want.Method)
		case got.Path != want.Path:
			problems = append(problems, name+": path "+got.Path+" != "+want.Path)
		case got.StepUp != want.StepUp:
			problems = append(problems, name+": stepUp differs")
		case got.KeyRule() != want.IdempotencyKey:
			problems = append(problems, name+": idempotencyKey "+got.KeyRule()+" != "+want.IdempotencyKey)
		case got.Free != want.Free:
			problems = append(problems, name+": free differs")
		case got.OptionalBody != want.OptionalBody:
			problems = append(problems, name+": optionalBody differs")
		case got.AnyQuery != want.AnyQuery:
			problems = append(problems, name+": anyQuery differs")
		case !sameList(got.Required, want.Required):
			problems = append(problems, name+": required "+strings.Join(got.Required, ",")+" != "+strings.Join(want.Required, ","))
		case !sameList(got.Optional, want.Optional):
			problems = append(problems, name+": optional "+strings.Join(got.Optional, ",")+" != "+strings.Join(want.Optional, ","))
		case !sameList(got.Query, want.Query):
			problems = append(problems, name+": query "+strings.Join(got.Query, ",")+" != "+strings.Join(want.Query, ","))
		case got.Binary || got.Screening || len(got.AnyOf) > 0:
			problems = append(problems, name+": binary, screening or anyOf set")
		}
	}
	for name := range table {
		if _, ok := catalog[name]; !ok {
			problems = append(problems, name+": not in the catalog")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("%s %s differs from the catalog:\n%s", product, group, strings.Join(problems, "\n"))
	}
}

// compareEnums checks that each catalog enum group holds exactly the given
// values (the member names differ between the reference SDKs).
func compareEnums(t *testing.T, groups map[string]catalogGroup, enums map[string][]string) {
	t.Helper()
	for name, got := range enums {
		group, ok := groups[name]
		if !ok || group.Kind != "enum" {
			t.Errorf("catalog enum %s missing", name)
			continue
		}
		var values map[string]string
		if err := json.Unmarshal(group.Values, &values); err != nil {
			t.Errorf("catalog enum %s: %v", name, err)
			continue
		}
		want := slices.Sorted(maps.Values(values))
		if have := slices.Sorted(slices.Values(got)); !slices.Equal(have, want) {
			t.Errorf("%s: Go %v, catalog %v", name, have, want)
		}
	}
}

func TestCatalogSubscriptionRoutes(t *testing.T) {
	compareTableRoutes(t, "subscriptions", "SUBSCRIPTION_ROUTES", SubscriptionRoutes())
}

func TestCatalogKycRoutes(t *testing.T) {
	compareTableRoutes(t, "kyc", "KYC_ROUTES", KycRoutes())
}

func TestCatalogIdentityRoutes(t *testing.T) {
	compareTableRoutes(t, "identity", "IDENTITY_ROUTES", IdentityRoutes())
}

func TestCatalogMarketplaceRoutes(t *testing.T) {
	compareTableRoutes(t, "marketplace", "MARKETPLACE_ROUTES", MarketplaceRoutes())
}

func TestCatalogSubscriptionValues(t *testing.T) {
	groups := catalogGroups(t, "subscriptions")
	compareLists(t, groups, map[string][]string{
		"SUBSCRIPTION_ACTIVE_STATES":         SubscriptionActiveStates(),
		"SUBSCRIPTION_TERMINAL_STATES":       SubscriptionTerminalStates(),
		"SubscriptionStatus.ACTIVE_STATES":   SubscriptionActiveStates(),
		"SubscriptionStatus.TERMINAL_STATES": SubscriptionTerminalStates(),
		"SUBSCRIPTION_BILLING_RULE_FIELDS":   SubscriptionBillingRuleFields(),
		"SUBSCRIPTION_EVENT_TYPES":           SubscriptionEventTypes(),
		"CollectionMethod.ALL":               SubscriptionCollectionMethods(),
	})
	compareEnums(t, groups, map[string][]string{
		"SubscriptionStatus":               SubscriptionStatuses(),
		"SubscriptionInvoiceStatus":        SubscriptionInvoiceStatuses(),
		"InvoiceStatus":                    SubscriptionInvoiceStatuses(),
		"SubscriptionCollectionMethod":     SubscriptionCollectionMethods(),
		"CollectionMethod":                 SubscriptionCollectionMethods(),
		"SubscriptionBillingFrequency":     SubscriptionBillingFrequencies(),
		"BillingRule":                      SubscriptionBillingFrequencies(),
		"SubscriptionCardVerificationMode": SubscriptionCardVerificationModes(),
		"AddCardRequest":                   SubscriptionCardVerificationModes(),
		"SubscriptionPlanChangeBehavior":   SubscriptionPlanChangeBehaviors(),
		"ChangePlanRequest":                SubscriptionPlanChangeBehaviors(),
		"SubscriptionPortalSdkMode":        SubscriptionPortalSdkModes(),
		"PortalClient":                     SubscriptionPortalSdkModes(),
		"SubscriptionEvents":               SubscriptionEventTypes(),
		"PortalLink":                       {SubscriptionPortalHostedBase},
	})
}

func TestCatalogKycValues(t *testing.T) {
	groups := catalogGroups(t, "kyc")
	compareLists(t, groups, map[string][]string{
		"KYC_EVENT_TYPES":               KycEventTypes(),
		"KYC_SESSION_TERMINAL_STATES":   KycSessionTerminalStates(),
		"SessionStatus.TERMINAL_STATES": KycSessionTerminalStates(),
		"SessionType.ALL":               KycSessionTypes(),
		"EvidencePurpose.ALL":           KycEvidencePurposes(),
		"AchievedAssurance.DOMAINS":     KycAssuranceDomains(),
	})
	compareEnums(t, groups, map[string][]string{
		"KycSessionType":     KycSessionTypes(),
		"SessionType":        KycSessionTypes(),
		"KycSessionStatus":   KycSessionStatuses(),
		"SessionStatus":      KycSessionStatuses(),
		"KycEvidencePurpose": KycEvidencePurposes(),
		"EvidencePurpose":    KycEvidencePurposes(),
		"KycAssuranceLevel":  KycAssuranceLevels(),
		"AssuranceLevel":     KycAssuranceLevels(),
		"KycAssuranceDomain": KycAssuranceDomains(),
		"AchievedAssurance":  KycAssuranceDomains(),
		"KycEvents":          KycEventTypes(),
	})
}

func TestCatalogIdentityValues(t *testing.T) {
	compareLists(t, catalogGroups(t, "identity"), map[string][]string{
		"IDENTITY_ANALYZE_FIELDS": IdentityAnalyzeFields(),
	})
}

func TestCatalogMarketplaceValues(t *testing.T) {
	groups := catalogGroups(t, "marketplace")
	compareLists(t, groups, map[string][]string{
		"MARKETPLACE_SELLER_FIELDS":          MarketplaceSellerFields(),
		"MARKETPLACE_CHARGE_REQUIRED_FIELDS": MarketplaceChargeRequiredFields(),
	})
	compareEnums(t, groups, map[string][]string{
		"MarketplaceDealerStatus":      MarketplaceDealerStatuses(),
		"DealerStatus":                 MarketplaceDealerStatuses(),
		"MarketplaceDealerType":        MarketplaceDealerTypes(),
		"CreateDealerRequest":          MarketplaceDealerTypes(),
		"MarketplaceMultiSellerPolicy": MarketplaceMultiSellerPolicies(),
		"MarketplaceClient":            MarketplaceMultiSellerPolicies(),
		"DealerOnboardingLink":         {MarketplaceDealerOnboardingHostedBase},
	})
}

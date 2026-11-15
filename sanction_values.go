package lunixi

// Sanctions screening (/api/v1/sanction/*, product SANCTION_SERVICE): value
// sets, the route table and the clear-reading rule.

// Risk domains a screening can be narrowed to (riskDomains). Omitted: every
// domain.
const (
	SanctionRiskDomainSanctions  = "sanctions"
	SanctionRiskDomainPep        = "pep"
	SanctionRiskDomainRca        = "rca"
	SanctionRiskDomainWanted     = "wanted"
	SanctionRiskDomainRegulatory = "regulatory"
	SanctionRiskDomainDebarment  = "debarment"
	SanctionRiskDomainOwnership  = "ownership"
	SanctionRiskDomainVessel     = "vessel"
	SanctionRiskDomainSecurities = "securities"
)

// Name-screening presets (mode). Sent with riskDomains, their intersection is
// screened.
const (
	SanctionScreenModeSanctionsOnly = "sanctions_only"
	SanctionScreenModeSanctionsPep  = "sanctions_pep"
	SanctionScreenModeFullRisk      = "full_risk"
)

// Wallet-screening depth (mode); the wider modes add exposure and
// crypto-intelligence hits.
const (
	SanctionWalletScreenModeSanctions = "wallet_sanctions"
	SanctionWalletScreenModeRisk      = "wallet_risk"
	SanctionWalletScreenModeFullAML   = "wallet_full_aml"
)

// Subject of a name screening or a monitoring registration (subjectType).
const (
	SanctionSubjectTypePerson       = "person"
	SanctionSubjectTypeOrganization = "organization"
)

// How much of the requested scope a screening covered (coverageStatus).
// COMPLETE: every requested risk domain was screened. PARTIAL: some requested
// domain has no list (uncoveredRiskDomains, uncoveredSources); no match is
// clear only for what was screened. DEGRADED: the candidate search was
// limited; run the screening again.
const (
	SanctionCoverageStatusComplete = "COMPLETE"
	SanctionCoverageStatusPartial  = "PARTIAL"
	SanctionCoverageStatusDegraded = "DEGRADED"
)

// Strength of a screening hit (hitType).
const (
	SanctionHitTypeExact    = "exact"
	SanctionHitTypeProbable = "probable"
	SanctionHitTypePossible = "possible"
)

// Sanction webhook events.
const (
	SanctionEventMatchDetected          = "screening.match.detected"
	SanctionEventFalsePositiveCleared   = "screening.false_positive.cleared"
	SanctionEventMonitoringAlertCreated = "ongoing_monitoring.alert.created"
	SanctionEventMonitoringClear        = "ongoing_monitoring.clear"
)

// sanctionRoutes is the single description of every sanction route the SDK
// calls: method, path template, the body fields the gateway forwards to the
// sanction service (it drops any other field silently, so the SDK refuses it
// before sending) and the query parameters it reads. Screening routes are
// read only when the answer is status "success".
//
// Only routes an API key can reach are listed (all need
// sanction:view_results): the screening report PDF is a panel route, and
// adding deny-list entries, analyst decisions, case assignment, entity
// resolution, list-source syncs and the adverse-media policy need
// permissions an API key does not carry.
var sanctionRoutes = map[string]Route{
	"screenName":                 {Method: "POST", Path: "/api/v1/sanction/screening/name", Screening: true, Required: []string{"name"}, Optional: []string{"subjectType", "country", "dateOfBirth", "identifiers", "riskDomains", "mode", "threshold", "customerReference"}},
	"screenWallet":               {Method: "POST", Path: "/api/v1/sanction/screening/wallet", Screening: true, Required: []string{"walletAddress"}, Optional: []string{"chain", "mode", "customerReference"}},
	"screenAccount":              {Method: "POST", Path: "/api/v1/sanction/screening/account", Screening: true, Required: []string{"accountValue", "accountScheme"}, Optional: []string{"providerCode", "productChannel", "customerReference"}},
	"screenTransaction":          {Method: "POST", Path: "/api/v1/sanction/screening/transaction", Screening: true, Required: []string{}, Optional: []string{"transactionType", "direction", "amount", "currency", "channel", "occurredAt", "parties", "accounts", "metadataJson"}, AnyOf: []string{"parties", "accounts"}},
	"screenPublicProcurementBan": {Method: "POST", Path: "/api/v1/sanction/intelligence/public-procurement-ban", Screening: true, Required: []string{}, Optional: []string{"entityName", "tckn", "vkn", "entityType", "onlyActive", "limit", "customerReference", "productChannel"}, AnyOf: []string{"entityName", "tckn", "vkn"}},
	"screenAdverseMedia":         {Method: "POST", Path: "/api/v1/sanction/intelligence/adverse-media", Screening: true, Required: []string{}, Optional: []string{"entityName", "entityType", "tckn", "vkn", "countryCode", "limit", "onlyActive", "customerReference", "productChannel"}},
	"evaluateBehavioralAlert":    {Method: "POST", Path: "/api/v1/sanction/intelligence/behavioral-alert", Screening: true, Required: []string{}, Optional: []string{"subjectReference", "customerReference", "productChannel", "velocityScore", "unusualPattern", "countryRiskLevel", "metadataJson"}},
	"listScreeningHistory":       {Method: "GET", Path: "/api/v1/sanction/screening/history", Query: []string{"limit", "cursor"}},
	"registerMonitoring":         {Method: "POST", Path: "/api/v1/sanction/monitoring/register", Required: []string{"externalCustomerId"}, Optional: []string{"subjectType", "profileJson", "frequency", "riskDomainScope"}},
	"listMonitoringAlerts":       {Method: "GET", Path: "/api/v1/sanction/monitoring/alerts", Query: []string{"status", "limit", "cursor", "includeTotal"}},
	"listMonitoredEntities":      {Method: "GET", Path: "/api/v1/sanction/monitoring/entities", Query: []string{"activeOnly", "limit", "cursor", "includeTotal"}},
	"listMonitoringMatches":      {Method: "GET", Path: "/api/v1/sanction/monitoring/matches", Query: []string{"status", "limit", "cursor"}},
	"getDatasetFreshness":        {Method: "GET", Path: "/api/v1/sanction/monitoring/dataset-freshness"},
	"searchEntities":             {Method: "GET", Path: "/api/v1/sanction/intelligence/entity-search", Query: []string{"q", "limit"}},
	"getEntityProfile":           {Method: "GET", Path: "/api/v1/sanction/intelligence/entity-profiles/:entityId", Query: []string{"screeningRequestId"}},
	"getEntityGraph":             {Method: "GET", Path: "/api/v1/sanction/intelligence/entity-graph/:entityId", Query: []string{"depth"}},
	"getIndirectExposure":        {Method: "GET", Path: "/api/v1/sanction/intelligence/indirect-exposure/:entityId", Query: []string{"maxDepth", "entityIds"}},
	"getOwnershipAggregation":    {Method: "GET", Path: "/api/v1/sanction/intelligence/ownership-aggregation/:entityId", Query: []string{"depth", "entityIds"}},
	"listBlacklistEntries":       {Method: "GET", Path: "/api/v1/sanction/blacklist", Query: []string{"limit", "cursor"}},
}

// sanctionNestedFields are the fields of the nested lists, checked element
// by element (the service drops anything else).
var sanctionNestedFields = map[string]NestedFields{
	"screenName.identifiers":     {Required: []string{"type", "value"}, Optional: []string{}},
	"screenTransaction.parties":  {Required: []string{"name"}, Optional: []string{"role", "subjectType", "countryCode", "customerId", "dateOfBirth", "nationalId"}},
	"screenTransaction.accounts": {Required: []string{"accountType", "value"}, Optional: []string{"role", "providerCode", "chain"}},
}

// sanctionValueSets are checked before sending; the sanction service refuses
// anything else with 400.
var sanctionValueSets = map[string][]string{
	"screenName.riskDomains":             SanctionRiskDomains(),
	"screenName.mode":                    SanctionScreenModes(),
	"screenName.subjectType":             SanctionSubjectTypes(),
	"screenWallet.mode":                  SanctionWalletScreenModes(),
	"registerMonitoring.subjectType":     SanctionSubjectTypes(),
	"registerMonitoring.riskDomainScope": SanctionRiskDomains(),
}

// SanctionRoutes returns a copy of the sanction route table (route name →
// Route), the same table as the other Lunixi SDKs.
func SanctionRoutes() map[string]Route { return cloneRoutes(sanctionRoutes) }

// SanctionNestedFields returns a copy of the fields of the nested lists
// ("route.field" → fields).
func SanctionNestedFields() map[string]NestedFields { return cloneNested(sanctionNestedFields) }

// SanctionValueSets returns a copy of the value sets checked before sending
// ("route.field" → values).
func SanctionValueSets() map[string][]string { return cloneLists(sanctionValueSets) }

// SanctionRiskDomains lists the risk domains in wire order.
func SanctionRiskDomains() []string {
	return []string{SanctionRiskDomainSanctions, SanctionRiskDomainPep, SanctionRiskDomainRca, SanctionRiskDomainWanted, SanctionRiskDomainRegulatory, SanctionRiskDomainDebarment, SanctionRiskDomainOwnership, SanctionRiskDomainVessel, SanctionRiskDomainSecurities}
}

// SanctionScreenModes lists the name-screening modes in wire order.
func SanctionScreenModes() []string {
	return []string{SanctionScreenModeSanctionsOnly, SanctionScreenModeSanctionsPep, SanctionScreenModeFullRisk}
}

// SanctionWalletScreenModes lists the wallet-screening modes in wire order.
func SanctionWalletScreenModes() []string {
	return []string{SanctionWalletScreenModeSanctions, SanctionWalletScreenModeRisk, SanctionWalletScreenModeFullAML}
}

// SanctionSubjectTypes lists the subject types in wire order.
func SanctionSubjectTypes() []string {
	return []string{SanctionSubjectTypePerson, SanctionSubjectTypeOrganization}
}

// SanctionCoverageStatuses lists the coverage statuses in wire order.
func SanctionCoverageStatuses() []string {
	return []string{SanctionCoverageStatusComplete, SanctionCoverageStatusPartial, SanctionCoverageStatusDegraded}
}

// SanctionHitTypes lists the hit types in wire order.
func SanctionHitTypes() []string {
	return []string{SanctionHitTypeExact, SanctionHitTypeProbable, SanctionHitTypePossible}
}

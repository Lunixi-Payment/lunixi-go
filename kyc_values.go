package lunixi

import (
	"slices"
	"strings"
)

// KYC / KYB / evidence — /api/v1/kyc/*. The values are the same codes as the
// other Lunixi SDKs. The browser bootstraps its own SDK token with the
// publishable key (/kyc/sdk/init); that route is not reachable from a server
// key and is not in KycRoutes.

// Session types. EVIDENCE is proof-of-X (address, funds, card ownership …),
// not only onboarding.
const (
	// KycSessionTypeKYC verifies a person's identity.
	KycSessionTypeKYC = "KYC"
	// KycSessionTypeKYB verifies a company and its beneficial owners.
	KycSessionTypeKYB      = "KYB"
	KycSessionTypeEvidence = "EVIDENCE"
)

// Status of a KYC session.
const (
	KycSessionStatusCreated                    = "CREATED"
	KycSessionStatusInitiated                  = "INITIATED"
	KycSessionStatusDocumentSubmitted          = "DOCUMENT_SUBMITTED"
	KycSessionStatusProcessing                 = "PROCESSING"
	KycSessionStatusAwaitingProviderCallback   = "AWAITING_PROVIDER_CALLBACK"
	KycSessionStatusAwaitingAdditionalDocument = "AWAITING_ADDITIONAL_DOCUMENT"
	KycSessionStatusAwaitingUserAction         = "AWAITING_USER_ACTION"
	KycSessionStatusUnderReview                = "UNDER_REVIEW"
	KycSessionStatusPendingAdditionalInfo      = "PENDING_ADDITIONAL_INFO"
	KycSessionStatusRescreenRequired           = "RESCREEN_REQUIRED"
	KycSessionStatusReopened                   = "REOPENED"
	KycSessionStatusApproved                   = "APPROVED"
	KycSessionStatusRejected                   = "REJECTED"
	KycSessionStatusManualReview               = "MANUAL_REVIEW"
	KycSessionStatusExpired                    = "EXPIRED"
	KycSessionStatusAbandoned                  = "ABANDONED"
	KycSessionStatusCancelled                  = "CANCELLED"
)

// evidencePurpose of an EVIDENCE session.
const (
	KycEvidencePurposeIdentityVerification   = "IDENTITY_VERIFICATION"
	KycEvidencePurposeBusinessVerification   = "BUSINESS_VERIFICATION"
	KycEvidencePurposeProofOfFunds           = "PROOF_OF_FUNDS"
	KycEvidencePurposeProofOfAddress         = "PROOF_OF_ADDRESS"
	KycEvidencePurposeSourceOfFunds          = "SOURCE_OF_FUNDS"
	KycEvidencePurposeSourceOfWealth         = "SOURCE_OF_WEALTH"
	KycEvidencePurposeTransactionEvidence    = "TRANSACTION_EVIDENCE"
	KycEvidencePurposeCardOwnership          = "CARD_OWNERSHIP"
	KycEvidencePurposePaymentMethodOwnership = "PAYMENT_METHOD_OWNERSHIP"
	KycEvidencePurposePayoutEvidence         = "PAYOUT_EVIDENCE"
	KycEvidencePurposeMerchantOnboarding     = "MERCHANT_ONBOARDING"
)

// Assurance ladder (rank-aligned with NIST 800-63 IAL / eIDAS LoA). A gate
// passes when achieved >= required (KycAssuranceMeets).
const (
	KycAssuranceLevelNone         = "NONE"
	KycAssuranceLevelNotPerformed = "NOT_PERFORMED"
	KycAssuranceLevelLow          = "LOW"
	KycAssuranceLevelSubstantial  = "SUBSTANTIAL"
	KycAssuranceLevelHigh         = "HIGH"
)

// Domains of the achieved assurance snapshot
// (session.decision.assuranceSnapshot.achieved), each an assurance level.
const (
	KycAssuranceDomainIdentity      = "identity_evidence_strength"
	KycAssuranceDomainDocument      = "document_authenticity_level"
	KycAssuranceDomainBiometric     = "possession_biometric_binding"
	KycAssuranceDomainAuthoritative = "authoritative_source_level"
	KycAssuranceDomainAddress       = "address_assurance"
	KycAssuranceDomainAML           = "aml_freshness"
	KycAssuranceDomainHumanReview   = "human_review_level"
)

// Webhook event types of KYC (the same signed envelope as payment events).
// data always carries sessionId and externalCustomerId; terminal events
// carry the decision. A rescreen or screening hit on an approved session
// must revoke access.
const (
	KycEventSessionCreated                = "kyc.session.created"
	KycEventSessionInitiated              = "kyc.session.initiated"
	KycEventSessionExpired                = "kyc.session.expired"
	KycEventSessionAbandoned              = "kyc.session.abandoned"
	KycEventSessionCancelled              = "kyc.session.cancelled"
	KycEventDocumentReceived              = "kyc.document.received"
	KycEventDocumentRejected              = "kyc.document.rejected"
	KycEventLivenessCompleted             = "kyc.liveness.completed"
	KycEventProcessingStarted             = "kyc.processing.started"
	KycEventManualReviewRequired          = "kyc.manual_review.required"
	KycEventApproved                      = "kyc.approved"
	KycEventRejected                      = "kyc.rejected"
	KycEventRescreenRequired              = "kyc.rescreen.required"
	KycEventKybSessionCreated             = "kyb.session.created"
	KycEventKybCompanyVerified            = "kyb.company.verified"
	KycEventKybCompanyNotFound            = "kyb.company.not_found"
	KycEventKybUboPending                 = "kyb.ubo.pending"
	KycEventKybRelatedKycRequired         = "kyb.related_kyc.required"
	KycEventKybRelatedKycCompleted        = "kyb.related_kyc.completed"
	KycEventKybManualReviewRequired       = "kyb.manual_review.required"
	KycEventKybApproved                   = "kyb.approved"
	KycEventKybRejected                   = "kyb.rejected"
	KycEventScreeningMatchDetected        = "screening.match.detected"
	KycEventScreeningFalsePositiveCleared = "screening.false_positive.cleared"
	KycEventMonitoringAlertCreated        = "ongoing_monitoring.alert.created"
)

// KycSessionTTLMaxMinutes is the longest session lifetime ttlMinutes
// accepts (one day).
const KycSessionTTLMaxMinutes = 1440

// KycSessionTypes returns every session type.
func KycSessionTypes() []string {
	return []string{KycSessionTypeKYC, KycSessionTypeKYB, KycSessionTypeEvidence}
}

// KycSessionStatuses returns every session status.
func KycSessionStatuses() []string {
	return []string{
		KycSessionStatusCreated, KycSessionStatusInitiated, KycSessionStatusDocumentSubmitted, KycSessionStatusProcessing,
		KycSessionStatusAwaitingProviderCallback, KycSessionStatusAwaitingAdditionalDocument, KycSessionStatusAwaitingUserAction,
		KycSessionStatusUnderReview, KycSessionStatusPendingAdditionalInfo, KycSessionStatusRescreenRequired, KycSessionStatusReopened,
		KycSessionStatusApproved, KycSessionStatusRejected, KycSessionStatusManualReview, KycSessionStatusExpired,
		KycSessionStatusAbandoned, KycSessionStatusCancelled,
	}
}

// KycSessionTerminalStates returns the terminal session states.
func KycSessionTerminalStates() []string {
	return []string{KycSessionStatusApproved, KycSessionStatusRejected, KycSessionStatusManualReview, KycSessionStatusExpired, KycSessionStatusAbandoned, KycSessionStatusCancelled}
}

// KycEvidencePurposes returns every evidence purpose.
func KycEvidencePurposes() []string {
	return []string{
		KycEvidencePurposeIdentityVerification, KycEvidencePurposeBusinessVerification, KycEvidencePurposeProofOfFunds,
		KycEvidencePurposeProofOfAddress, KycEvidencePurposeSourceOfFunds, KycEvidencePurposeSourceOfWealth,
		KycEvidencePurposeTransactionEvidence, KycEvidencePurposeCardOwnership, KycEvidencePurposePaymentMethodOwnership,
		KycEvidencePurposePayoutEvidence, KycEvidencePurposeMerchantOnboarding,
	}
}

// KycAssuranceLevels returns the assurance ladder, lowest first.
func KycAssuranceLevels() []string {
	return []string{KycAssuranceLevelNone, KycAssuranceLevelNotPerformed, KycAssuranceLevelLow, KycAssuranceLevelSubstantial, KycAssuranceLevelHigh}
}

// KycAssuranceDomains returns the domains of the achieved assurance
// snapshot.
func KycAssuranceDomains() []string {
	return []string{KycAssuranceDomainIdentity, KycAssuranceDomainDocument, KycAssuranceDomainBiometric, KycAssuranceDomainAuthoritative, KycAssuranceDomainAddress, KycAssuranceDomainAML, KycAssuranceDomainHumanReview}
}

// KycEventTypes returns every KYC webhook event type.
func KycEventTypes() []string {
	return []string{
		KycEventSessionCreated, KycEventSessionInitiated, KycEventSessionExpired, KycEventSessionAbandoned,
		KycEventSessionCancelled, KycEventDocumentReceived, KycEventDocumentRejected, KycEventLivenessCompleted,
		KycEventProcessingStarted, KycEventManualReviewRequired, KycEventApproved, KycEventRejected,
		KycEventRescreenRequired, KycEventKybSessionCreated, KycEventKybCompanyVerified, KycEventKybCompanyNotFound,
		KycEventKybUboPending, KycEventKybRelatedKycRequired, KycEventKybRelatedKycCompleted,
		KycEventKybManualReviewRequired, KycEventKybApproved, KycEventKybRejected, KycEventScreeningMatchDetected,
		KycEventScreeningFalsePositiveCleared, KycEventMonitoringAlertCreated,
	}
}

const kycBase = "/api/v1/kyc"

// kycSessionFields are the optional body fields of createSession and
// evidenceRequest.
var kycSessionFields = []string{"externalCustomerId", "customerMode", "customer", "jurisdiction", "requiredDocumentTypes", "primaryDocumentType", "evidencePurpose", "ttlMinutes", "idempotencyKey", "evidenceContextJson"}

// kycRoutes is the single description of every KYC route the SDK calls (the
// same table as the other Lunixi SDKs). The surface is bearer-only.
var kycRoutes = map[string]Route{
	"createSession": {Method: "POST", Path: kycBase + "/sessions", OptionalIdempotencyKey: true, Required: []string{"subjectId", "sessionType"}, Optional: kycSessionFields},
	"getSession":    {Method: "GET", Path: kycBase + "/sessions/:sessionId"},
	// The merchant list takes only these two; an unknown filter is a 400 on
	// the gateway.
	"listSessions":       {Method: "GET", Path: kycBase + "/sessions", Query: []string{"limit", "cursor"}},
	"setJurisdiction":    {Method: "POST", Path: kycBase + "/sessions/:sessionId/jurisdiction", Required: []string{"jurisdiction"}},
	"setPrimaryDocument": {Method: "POST", Path: kycBase + "/sessions/:sessionId/primary-document", Required: []string{"documentType"}},
	"uploadDocument": {Method: "POST", Path: kycBase + "/sessions/:sessionId/documents",
		Required: []string{"documentType", "fileBase64", "mimeType"},
		Optional: []string{"side", "jurisdiction", "metadataJson", "idempotencyKey"}},
	"timeline":             {Method: "GET", Path: kycBase + "/sessions/:sessionId/timeline"},
	"submit":               {Method: "POST", Path: kycBase + "/sessions/:sessionId/submit"},
	"manualReview":         {Method: "POST", Path: kycBase + "/sessions/:sessionId/manual-review", Optional: []string{"reasonCode", "reasonDetail"}, OptionalBody: true},
	"evidenceRequest":      {Method: "POST", Path: kycBase + "/evidence-requests", OptionalIdempotencyKey: true, Required: []string{"subjectId", "sessionType"}, Optional: kycSessionFields},
	"merchantPolicy":       {Method: "GET", Path: kycBase + "/admin/merchant-policy"},
	"updateMerchantPolicy": {Method: "POST", Path: kycBase + "/admin/merchant-policy", Free: true},
	"rotateWebhookSecret":  {Method: "POST", Path: kycBase + "/admin/merchant-policy/webhook-secret/rotate"},
	"webhookOutbox":        {Method: "GET", Path: kycBase + "/admin/webhook-outbox", Query: []string{"limit", "status"}},
	"retryWebhookEvent":    {Method: "POST", Path: kycBase + "/admin/webhook-outbox/:eventId/retry"},
	"erasureRequests":      {Method: "GET", Path: kycBase + "/admin/erasure-requests", Query: []string{"limit", "status"}},
}

// KycRoutes returns a copy of the KYC route table, keyed by operation name.
func KycRoutes() map[string]Route { return cloneRoutes(kycRoutes) }

// IsKycSessionTerminal reports APPROVED, REJECTED, MANUAL_REVIEW, EXPIRED,
// ABANDONED and CANCELLED (case-insensitive).
func IsKycSessionTerminal(status string) bool {
	return slices.Contains(KycSessionTerminalStates(), strings.ToUpper(status))
}

// KycAssuranceRank is the rank of a level on the assurance ladder: HIGH 3,
// SUBSTANTIAL 2, LOW 1, anything else 0 (case-insensitive).
func KycAssuranceRank(level string) int {
	switch strings.ToUpper(level) {
	case KycAssuranceLevelHigh:
		return 3
	case KycAssuranceLevelSubstantial:
		return 2
	case KycAssuranceLevelLow:
		return 1
	}
	return 0
}

// KycAssuranceMeets reports whether achieved meets or exceeds required.
func KycAssuranceMeets(achieved, required string) bool {
	return KycAssuranceRank(achieved) >= KycAssuranceRank(required)
}

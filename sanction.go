package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"slices"
	"strconv"
	"strings"
)

// SanctionClient — sanctions screening (/api/v1/sanction/*, product
// SANCTION_SERVICE). The key needs the sanction:view_results permission (403
// otherwise).
//
// Screening returns matches, not verdicts: the decision is yours. Read a
// result with IsClearScreening: an empty match list is clear only when
// coverageStatus is COMPLETE. When the screening cannot run (no list
// publication can be pinned) the gateway answers 503 and the call returns an
// *APIError; a 2xx answer whose status is not "success" is an *APIError too,
// so a screening that did not run never comes back as an empty result.
//
// Requests and filters are maps checked against SanctionRoutes before
// anything is sent: an unknown field, a missing required field, an empty
// AnyOf group, a nested row field outside SanctionNestedFields or a value
// outside SanctionValueSets returns ErrConfiguration (the service would drop
// an unknown field silently). Results are the envelope's data; the
// envelope's requestId is the HTTP request's id, the screening's own id is
// screeningRequestId (name screening). Lists return CursorPage.
type SanctionClient struct{ client *Client }

// ScreenName screens a person or organization: name, optional subjectType,
// country, dateOfBirth, identifiers [{type, value}], riskDomains, mode,
// threshold, customerReference.
func (s *SanctionClient) ScreenName(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "screenName", request)
}

// ScreenWallet screens a crypto wallet address (exact match against the
// listed addresses): walletAddress, optional chain, mode, customerReference.
func (s *SanctionClient) ScreenWallet(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "screenWallet", request)
}

// ScreenAccount screens a bank account / IBAN against your own deny list and
// account risk data (no list publication): accountValue, accountScheme,
// optional providerCode, productChannel, customerReference.
func (s *SanctionClient) ScreenAccount(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "screenAccount", request)
}

// ScreenTransaction screens every party and account of a transaction
// against one list publication; parties [{name, …}] or accounts
// [{accountType, value, …}] (at least one of them).
func (s *SanctionClient) ScreenTransaction(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "screenTransaction", request)
}

// ScreenPublicProcurementBan screens public procurement debarment lists by
// entityName, tckn or vkn (at least one). The Turkish procurement list is not
// published, so coverageStatus is PARTIAL and uncoveredSources names it: no
// match is not a clean result for it.
func (s *SanctionClient) ScreenPublicProcurementBan(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "screenPublicProcurementBan", request)
}

// ScreenAdverseMedia screens adverse media. It runs only when your
// adverse-media policy is enabled.
func (s *SanctionClient) ScreenAdverseMedia(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "screenAdverseMedia", request)
}

// EvaluateBehavioralAlert evaluates a behavioural risk signal.
func (s *SanctionClient) EvaluateBehavioralAlert(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return s.screen(ctx, "evaluateBehavioralAlert", request)
}

// ListScreeningHistory lists earlier screenings (audit trail), newest
// first. Filters: limit, cursor. Raw also carries runsToday,
// reviewDecisions, blockedCount and avgLatencyMs.
func (s *SanctionClient) ListScreeningHistory(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return s.page(ctx, "listScreeningHistory", nil, filters)
}

// IterateScreeningHistory yields every earlier screening, page by page.
func (s *SanctionClient) IterateScreeningHistory(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, s.ListScreeningHistory)
}

// RegisterMonitoring registers a subject for ongoing monitoring and returns
// {monitoredEntityId}: externalCustomerId, optional subjectType,
// profileJson, frequency, riskDomainScope. A "profile" member (an object:
// name, dateOfBirth, country, identifiers …) is sent as profileJson text;
// pass one of the two.
func (s *SanctionClient) RegisterMonitoring(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	body := orEmpty(request)
	if profile, ok := body["profile"]; ok && profile != nil {
		if body["profileJson"] != nil {
			return nil, invalid("registerMonitoring takes profile or profileJson, not both")
		}
		text, err := json.Marshal(profile)
		if err != nil {
			return nil, invalid("registerMonitoring profile must be JSON-encodable")
		}
		body = mapsWithout(body, "profile")
		body["profileJson"] = string(text)
	} else if ok {
		body = mapsWithout(body, "profile")
	}
	r, err := s.call(ctx, "registerMonitoring", nil, nil, body)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// ListMonitoringAlerts lists the new-match alerts raised by monitoring.
// Filters: status (e.g. open), limit, cursor, includeTotal.
func (s *SanctionClient) ListMonitoringAlerts(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return s.page(ctx, "listMonitoringAlerts", nil, filters)
}

// IterateMonitoringAlerts yields every monitoring alert, page by page.
func (s *SanctionClient) IterateMonitoringAlerts(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, s.ListMonitoringAlerts)
}

// ListMonitoredEntities lists the monitored subjects. Filters: activeOnly
// (true by default on the gateway), limit, cursor, includeTotal.
func (s *SanctionClient) ListMonitoredEntities(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return s.page(ctx, "listMonitoredEntities", nil, filters)
}

// IterateMonitoredEntities yields every monitored subject, page by page.
func (s *SanctionClient) IterateMonitoredEntities(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, s.ListMonitoredEntities)
}

// ListMonitoringMatches lists the matches raised by monitoring. Filters:
// status, limit, cursor.
func (s *SanctionClient) ListMonitoringMatches(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return s.page(ctx, "listMonitoringMatches", nil, filters)
}

// IterateMonitoringMatches yields every monitoring match, page by page.
func (s *SanctionClient) IterateMonitoringMatches(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, s.ListMonitoringMatches)
}

// GetDatasetFreshness reads the freshness of the source lists: {manifest}.
// A manifest that could not be produced comes back as status "failure" and
// is an *APIError.
func (s *SanctionClient) GetDatasetFreshness(ctx context.Context) (json.RawMessage, error) {
	return s.screen(ctx, "getDatasetFreshness", nil)
}

// SearchEntities searches entities by name. Filters: q, limit.
func (s *SanctionClient) SearchEntities(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	return s.data(ctx, "searchEntities", nil, filters)
}

// GetEntityProfile reads an entity's risk profile. Filter:
// screeningRequestId (read it on that screening's publication).
func (s *SanctionClient) GetEntityProfile(ctx context.Context, entityID string, filters map[string]any) (json.RawMessage, error) {
	return s.data(ctx, "getEntityProfile", map[string]string{"entityId": entityID}, filters)
}

// GetEntityGraph reads an entity's relationship graph. Filter: depth (1–3).
func (s *SanctionClient) GetEntityGraph(ctx context.Context, entityID string, filters map[string]any) (json.RawMessage, error) {
	return s.data(ctx, "getEntityGraph", map[string]string{"entityId": entityID}, filters)
}

// GetIndirectExposure reads an entity's indirect exposure paths. Filters:
// maxDepth, entityIds (the records of a merged match: a list or one id, sent
// comma-separated).
func (s *SanctionClient) GetIndirectExposure(ctx context.Context, entityID string, filters map[string]any) (json.RawMessage, error) {
	return s.data(ctx, "getIndirectExposure", map[string]string{"entityId": entityID}, filters)
}

// GetOwnershipAggregation reads an entity's ownership aggregation (50%
// rule). Filters: depth, entityIds (as in GetIndirectExposure).
func (s *SanctionClient) GetOwnershipAggregation(ctx context.Context, entityID string, filters map[string]any) (json.RawMessage, error) {
	return s.data(ctx, "getOwnershipAggregation", map[string]string{"entityId": entityID}, filters)
}

// ListBlacklistEntries lists your own deny-list entries (read only with an
// API key). Filters: limit, cursor.
func (s *SanctionClient) ListBlacklistEntries(ctx context.Context, filters map[string]any) (*CursorPage, error) {
	return s.page(ctx, "listBlacklistEntries", nil, filters)
}

// IterateBlacklistEntries yields every deny-list entry, page by page.
func (s *SanctionClient) IterateBlacklistEntries(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return iterateCursor(ctx, filters, s.ListBlacklistEntries)
}

// IsClearScreening reports whether a screening result (the data a
// screening method returned) can be read as "nothing found".
//
// True only when the result has no match or hit of any kind (matches, hits,
// publicationHits), no isSanctioned, isBlacklisted, matched or caseRequired
// flag, no blocking decision (block, reject, review, hit, escalate …) and,
// when it reports coverage, coverageStatus COMPLETE. A PARTIAL or DEGRADED
// screening and a replay without coverage (null) are never clear. Account
// and adverse-media screening do not report coverage; for them the hits and
// flags decide.
func IsClearScreening(result json.RawMessage) bool {
	var fields map[string]any
	if decodeNumbers(result, &fields) != nil || fields == nil {
		return false
	}
	for _, key := range []string{"matches", "hits", "publicationHits"} {
		if list, ok := fields[key].([]any); ok && len(list) > 0 {
			return false
		}
	}
	for _, key := range []string{"isSanctioned", "isBlacklisted", "matched", "caseRequired"} {
		if flag, ok := fields[key].(bool); ok && flag {
			return false
		}
	}
	if decision, ok := fields["decision"].(string); ok {
		switch strings.ToLower(decision) {
		case "block", "blocked", "reject", "rejected", "review", "hit", "escalate":
			return false
		}
	}
	if coverage, ok := fields["coverageStatus"]; ok {
		return coverage == SanctionCoverageStatusComplete
	}
	return true
}

// ── Route invoker ────────────────────────────────────────────────────────

func (s *SanctionClient) screen(ctx context.Context, name string, request map[string]any) (json.RawMessage, error) {
	r, err := s.call(ctx, name, nil, nil, request)
	if err != nil {
		return nil, err
	}
	return screeningData(r)
}

func (s *SanctionClient) data(ctx context.Context, name string, params map[string]string, filters map[string]any) (json.RawMessage, error) {
	r, err := s.call(ctx, name, params, orEmpty(filters), nil)
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

func (s *SanctionClient) page(ctx context.Context, name string, params map[string]string, filters map[string]any) (*CursorPage, error) {
	r, err := s.call(ctx, name, params, orEmpty(filters), nil)
	if err != nil {
		return nil, err
	}
	return dataCursorPage(r, false), nil
}

// call sends one route of SanctionRoutes. The body and query are checked
// against the route before anything leaves the process.
func (s *SanctionClient) call(ctx context.Context, name string, params map[string]string, query, body map[string]any) (*Response, error) {
	route := sanctionRoutes[name]
	path, err := fillRoutePath(route.Path, params)
	if err != nil {
		return nil, err
	}
	request := Request{Method: route.Method, Path: path}
	if query != nil {
		if request.Query, err = routeQuery(name, route.Query, query); err != nil {
			return nil, err
		}
	}
	if route.Method != "GET" {
		payload, err := sanctionBody(name, route, body)
		if err != nil {
			return nil, err
		}
		request.Body = payload
	}
	return s.client.Do(ctx, request)
}

// sanctionBody checks a body against its route: nil members are dropped;
// unknown fields, missing required fields, an empty AnyOf group, the nested
// rows and the value sets fail before sending.
func sanctionBody(name string, route Route, body map[string]any) (orderedObject, error) {
	object, err := plainObject(body, name+" body")
	if err != nil {
		return nil, err
	}
	plain := compactNil(object)
	if err = checkFieldNames(name, plain, route.Required, route.Optional, presentNonEmpty); err != nil {
		return nil, err
	}
	if len(route.AnyOf) > 0 && !slices.ContainsFunc(route.AnyOf, func(key string) bool { return presentNonEmpty(plain[key]) }) {
		return nil, invalid(name + " requires at least one of " + strings.Join(route.AnyOf, ", "))
	}
	for key, value := range plain {
		label := name + "." + key
		if spec, ok := sanctionNestedFields[label]; ok {
			if plain[key], err = sanctionRows(label, spec, value); err != nil {
				return nil, err
			}
		}
		if allowed, ok := sanctionValueSets[label]; ok {
			if err = sanctionValues(label, allowed, value); err != nil {
				return nil, err
			}
		}
	}
	return orderedFields(plain, route.Fields()), nil
}

func sanctionRows(label string, spec NestedFields, value any) ([]any, error) {
	rows, ok := value.([]any)
	if !ok {
		return nil, invalid(label + " must be an array")
	}
	fields := append(slices.Clone(spec.Required), spec.Optional...)
	out := make([]any, 0, len(rows))
	for i, entry := range rows {
		rowLabel := label + "[" + strconv.Itoa(i) + "]"
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, invalid(rowLabel + " must be an object")
		}
		plain := compactNil(row)
		if err := checkFieldNames(rowLabel, plain, spec.Required, spec.Optional, presentNonEmpty); err != nil {
			return nil, err
		}
		out = append(out, orderedFields(plain, fields))
	}
	return out, nil
}

func sanctionValues(label string, allowed []string, value any) error {
	values, ok := value.([]any)
	if !ok {
		values = []any{value}
	}
	for _, entry := range values {
		text, ok := entry.(string)
		if !ok || !slices.Contains(allowed, text) {
			return invalid(label + " must be one of " + strings.Join(allowed, ", "))
		}
	}
	return nil
}

// screeningData refuses a 2xx answer that is not status "success": a
// screening that did not run must never look like an empty result.
func screeningData(r *Response) (json.RawMessage, error) {
	envelope := objectFields(r.Envelope)
	var status string
	if json.Unmarshal(envelope["status"], &status) == nil && status != "success" {
		code := stringField(envelope, "code")
		if !safeCode.MatchString(code) {
			code = "SCREENING_NOT_COMPLETED"
		}
		return nil, &APIError{StatusCode: r.StatusCode, Code: code, RequestID: r.RequestID}
	}
	return r.Data, nil
}

func mapsWithout(object map[string]any, key string) map[string]any {
	out := make(map[string]any, len(object))
	for name, value := range object {
		if name != key {
			out[name] = value
		}
	}
	return out
}

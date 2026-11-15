package lunixi

import (
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"strconv"
)

// PhysicalPosManagementClient — stores, registers, logical terminals with
// their bank rows, payment-provider rows and device links, devices with their
// identity keys, the end-of-day policy, the catalog and the overview
// (/api/v1/physical-pos). Reads need physical_pos:view, writes
// physical_pos:manage, device identity writes (key revocation, enrollment
// revert, activation-code revocation) physical_pos:identity; a TEST key can
// only read. Activation codes are issued in the panel only.
//
// Updates and status changes take the rowVersion you last read (409
// physical_pos.version_conflict when the record changed). Reason codes are
// closed sets checked by the gateway (400
// physical_pos.input.reason_code_invalid). Routes that require an
// Idempotency-Key get a new UUID when you pass ""; a key you pass is sent
// exactly as given (never trimmed). The key sent comes back as
// IdempotencyKey. A key belongs to one operation: reusing it for another body
// or route is 409 physical_pos.idempotency_mismatch.
//
// Records come back as JSON (the record of data, or the whole data object
// where it has several parts); pages as PhysicalPosPage, whose TotalCount is
// set only with includeTotal true on entity lists.
type PhysicalPosManagementClient struct{ client *Client }

// GetOverview returns the store, register, terminal and device counts
// (GET /overview).
func (m *PhysicalPosManagementClient) GetOverview(ctx context.Context) (json.RawMessage, error) {
	return m.read(ctx, "/overview", nil, "overview")
}

// ── Catalog ──────────────────────────────────────────────────────────────

// ListVendors lists the device vendors (GET /catalog/vendors).
func (m *PhysicalPosManagementClient) ListVendors(ctx context.Context) (json.RawMessage, error) {
	return m.read(ctx, "/catalog/vendors", nil, "")
}

// ListDeviceModels lists the device models; filters vendorCode,
// deviceType, status (GET /catalog/device-models).
func (m *PhysicalPosManagementClient) ListDeviceModels(ctx context.Context, filters map[string]any) (json.RawMessage, error) {
	return m.client.posData(ctx, "/catalog/device-models", filters, "listDeviceModels")
}

// ListAcquirers lists the banks a terminal row can belong to
// (GET /catalog/acquirers).
func (m *PhysicalPosManagementClient) ListAcquirers(ctx context.Context) (json.RawMessage, error) {
	return m.read(ctx, "/catalog/acquirers", nil, "")
}

// ── Stores ───────────────────────────────────────────────────────────────

// ListStores lists stores; filters limit, cursor, includeTotal, status, q.
func (m *PhysicalPosManagementClient) ListStores(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return m.client.posList(ctx, "/stores", filters, "listStores")
}

// IterateStores yields every store matching the filters.
func (m *PhysicalPosManagementClient) IterateStores(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, m.ListStores)
}

// CreateStore creates a store: {code, name, city?, district?, addressLine?,
// timezone?, externalRef?}. The result holds data.store, Replayed,
// StatusCode (201 new / 200 replay) and IdempotencyKey.
func (m *PhysicalPosManagementClient) CreateStore(ctx context.Context, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	return m.keyedWrite(ctx, "/stores", request, "createStore", idempotencyKey)
}

// GetStore returns {store, registerSummary: {activeCount, archivedCount},
// assignedDeviceCount}.
func (m *PhysicalPosManagementClient) GetStore(ctx context.Context, storeID string) (json.RawMessage, error) {
	path, err := posPath("/stores/", storeID, "storeId", "")
	if err != nil {
		return nil, err
	}
	return m.read(ctx, path, nil, "")
}

// UpdateStore changes a store: {rowVersion, name?, city?, district?,
// addressLine?, timezone?, externalRef?}. nil clears city, district,
// addressLine and externalRef; a field left out is unchanged.
func (m *PhysicalPosManagementClient) UpdateStore(ctx context.Context, storeID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "PATCH", "/stores/", storeID, "storeId", "", request, "updateStore", "store")
}

// ArchiveStore archives a store with its active registers: {rowVersion,
// reasonCode}. Returns {store, archivedRegisterCount}.
func (m *PhysicalPosManagementClient) ArchiveStore(ctx context.Context, storeID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/stores/", storeID, "storeId", "/archive", request, "statusChange", "")
}

// RestoreStore restores an archived store: {rowVersion, reasonCode}.
func (m *PhysicalPosManagementClient) RestoreStore(ctx context.Context, storeID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/stores/", storeID, "storeId", "/restore", request, "statusChange", "store")
}

// ListStoreAudit lists a store's audit entries; filters limit, cursor.
func (m *PhysicalPosManagementClient) ListStoreAudit(ctx context.Context, storeID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/stores/", storeID, "storeId", "/audit", filters)
}

// IterateStoreAudit yields every audit entry of a store.
func (m *PhysicalPosManagementClient) IterateStoreAudit(ctx context.Context, storeID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListStoreAudit(ctx, storeID, page)
	})
}

// ── Registers ────────────────────────────────────────────────────────────

// ListRegisters lists a store's registers; filters limit, cursor,
// includeTotal, status.
func (m *PhysicalPosManagementClient) ListRegisters(ctx context.Context, storeID string, filters map[string]any) (*PhysicalPosPage, error) {
	path, err := posPath("/stores/", storeID, "storeId", "/registers")
	if err != nil {
		return nil, err
	}
	return m.client.posList(ctx, path, filters, "listRegisters")
}

// IterateRegisters yields every register of a store.
func (m *PhysicalPosManagementClient) IterateRegisters(ctx context.Context, storeID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListRegisters(ctx, storeID, page)
	})
}

// CreateRegister creates a register in a store: {code, name}. The result
// holds data.register, Replayed, StatusCode and IdempotencyKey.
func (m *PhysicalPosManagementClient) CreateRegister(ctx context.Context, storeID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	path, err := posPath("/stores/", storeID, "storeId", "/registers")
	if err != nil {
		return nil, err
	}
	return m.keyedWrite(ctx, path, request, "createRegister", idempotencyKey)
}

// UpdateRegister changes a register: {rowVersion, name?}.
func (m *PhysicalPosManagementClient) UpdateRegister(ctx context.Context, registerID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "PATCH", "/registers/", registerID, "registerId", "", request, "updateRegister", "register")
}

// ArchiveRegister archives a register: {rowVersion, reasonCode}.
func (m *PhysicalPosManagementClient) ArchiveRegister(ctx context.Context, registerID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/registers/", registerID, "registerId", "/archive", request, "statusChange", "register")
}

// RestoreRegister restores an archived register: {rowVersion, reasonCode}.
func (m *PhysicalPosManagementClient) RestoreRegister(ctx context.Context, registerID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/registers/", registerID, "registerId", "/restore", request, "statusChange", "register")
}

// ListRegisterAudit lists a register's audit entries; filters limit, cursor.
func (m *PhysicalPosManagementClient) ListRegisterAudit(ctx context.Context, registerID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/registers/", registerID, "registerId", "/audit", filters)
}

// IterateRegisterAudit yields every audit entry of a register.
func (m *PhysicalPosManagementClient) IterateRegisterAudit(ctx context.Context, registerID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListRegisterAudit(ctx, registerID, page)
	})
}

// ── Logical terminals ────────────────────────────────────────────────────

// ListTerminals lists logical terminals; filters limit, cursor,
// includeTotal, status, q.
func (m *PhysicalPosManagementClient) ListTerminals(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return m.client.posList(ctx, "/terminals", filters, "listTerminals")
}

// IterateTerminals yields every terminal matching the filters.
func (m *PhysicalPosManagementClient) IterateTerminals(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, m.ListTerminals)
}

// CreateTerminal creates a logical terminal: {code, displayName,
// vendorCode?, virtualSerial?, vendorTerminalRef?}. The result holds
// data.terminal, Replayed, StatusCode and IdempotencyKey.
func (m *PhysicalPosManagementClient) CreateTerminal(ctx context.Context, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	return m.keyedWrite(ctx, "/terminals", request, "createTerminal", idempotencyKey)
}

// GetTerminal returns {terminal, acquirers: {items, pageInfo}}.
func (m *PhysicalPosManagementClient) GetTerminal(ctx context.Context, terminalID string) (json.RawMessage, error) {
	path, err := posPath("/terminals/", terminalID, "terminalId", "")
	if err != nil {
		return nil, err
	}
	return m.read(ctx, path, nil, "")
}

// UpdateTerminal changes a terminal: {rowVersion, displayName?, vendorCode?}.
func (m *PhysicalPosManagementClient) UpdateTerminal(ctx context.Context, terminalID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "PATCH", "/terminals/", terminalID, "terminalId", "", request, "updateTerminal", "terminal")
}

// CloseTerminal closes a terminal: {rowVersion, reasonCode}.
func (m *PhysicalPosManagementClient) CloseTerminal(ctx context.Context, terminalID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/terminals/", terminalID, "terminalId", "/close", request, "statusChange", "terminal")
}

// ListTerminalAcquirers lists the bank rows of a terminal; a row's id is the
// payment's terminalAcquirerId. Filters limit, cursor, includeTotal.
func (m *PhysicalPosManagementClient) ListTerminalAcquirers(ctx context.Context, terminalID string, filters map[string]any) (*PhysicalPosPage, error) {
	path, err := posPath("/terminals/", terminalID, "terminalId", "/acquirers")
	if err != nil {
		return nil, err
	}
	return m.client.posList(ctx, path, filters, "listTerminalAcquirers")
}

// IterateTerminalAcquirers yields every bank row of a terminal.
func (m *PhysicalPosManagementClient) IterateTerminalAcquirers(ctx context.Context, terminalID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListTerminalAcquirers(ctx, terminalID, page)
	})
}

// AddTerminalAcquirer adds a bank row: {vendorAcquirerRef, merchantNumber,
// terminalNumber, label?, currencies}. The result holds data.acquirer,
// Replayed, StatusCode and IdempotencyKey.
func (m *PhysicalPosManagementClient) AddTerminalAcquirer(ctx context.Context, terminalID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	path, err := posPath("/terminals/", terminalID, "terminalId", "/acquirers")
	if err != nil {
		return nil, err
	}
	return m.keyedWrite(ctx, path, request, "addTerminalAcquirer", idempotencyKey)
}

// UpdateTerminalAcquirer changes a bank row: {rowVersion, label?,
// currencies?, status?}; nil label clears it.
func (m *PhysicalPosManagementClient) UpdateTerminalAcquirer(ctx context.Context, terminalAcquirerID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "PATCH", "/terminal-acquirers/", terminalAcquirerID, "terminalAcquirerId", "", request, "updateTerminalAcquirer", "acquirer")
}

// ListTerminalProviders lists the payment-provider rows of a terminal (a
// provider's merchant / terminal / product code, for example of a meal card
// provider). A row's id is the payment's terminalProviderId. Filters limit,
// cursor, includeTotal.
func (m *PhysicalPosManagementClient) ListTerminalProviders(ctx context.Context, terminalID string, filters map[string]any) (*PhysicalPosPage, error) {
	path, err := posPath("/terminals/", terminalID, "terminalId", "/providers")
	if err != nil {
		return nil, err
	}
	return m.client.posList(ctx, path, filters, "listTerminalProviders")
}

// IterateTerminalProviders yields every provider row of a terminal.
func (m *PhysicalPosManagementClient) IterateTerminalProviders(ctx context.Context, terminalID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListTerminalProviders(ctx, terminalID, page)
	})
}

// AddTerminalProvider adds a provider row: {vendorAccountId, merchantCode,
// terminalCode, productCode?, label?}; the provider comes from the vendor
// account. The result holds data.provider, Replayed, StatusCode (201 new /
// 200 replay) and IdempotencyKey.
func (m *PhysicalPosManagementClient) AddTerminalProvider(ctx context.Context, terminalID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	path, err := posPath("/terminals/", terminalID, "terminalId", "/providers")
	if err != nil {
		return nil, err
	}
	return m.keyedWrite(ctx, path, request, "addTerminalProvider", idempotencyKey)
}

// UpdateTerminalProvider changes a provider row: {rowVersion, label?,
// status?}; nil label clears it, status ACTIVE or DISABLED.
func (m *PhysicalPosManagementClient) UpdateTerminalProvider(ctx context.Context, terminalProviderID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "PATCH", "/terminal-providers/", terminalProviderID, "terminalProviderId", "", request, "updateTerminalProvider", "provider")
}

// ChangeTerminalDevice links the terminal to a device, closing the previous
// link: {rowVersion, deviceId, reasonCode}. Returns {terminal, link}.
func (m *PhysicalPosManagementClient) ChangeTerminalDevice(ctx context.Context, terminalID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/terminals/", terminalID, "terminalId", "/device-link", request, "changeTerminalDevice", "")
}

// CloseTerminalDeviceLink closes the terminal's device link: {rowVersion,
// reasonCode}. The result holds data.terminal and data.link (the closed
// link or null), Replayed, StatusCode and IdempotencyKey.
func (m *PhysicalPosManagementClient) CloseTerminalDeviceLink(ctx context.Context, terminalID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	path, err := posPath("/terminals/", terminalID, "terminalId", "/device-link/close")
	if err != nil {
		return nil, err
	}
	return m.keyedWrite(ctx, path, request, "statusChange", idempotencyKey)
}

// ListTerminalLinks lists a terminal's device links; filters limit, cursor.
func (m *PhysicalPosManagementClient) ListTerminalLinks(ctx context.Context, terminalID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/terminals/", terminalID, "terminalId", "/links", filters)
}

// IterateTerminalLinks yields every device link of a terminal.
func (m *PhysicalPosManagementClient) IterateTerminalLinks(ctx context.Context, terminalID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListTerminalLinks(ctx, terminalID, page)
	})
}

// ListTerminalAudit lists a terminal's audit entries; filters limit, cursor.
func (m *PhysicalPosManagementClient) ListTerminalAudit(ctx context.Context, terminalID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/terminals/", terminalID, "terminalId", "/audit", filters)
}

// IterateTerminalAudit yields every audit entry of a terminal.
func (m *PhysicalPosManagementClient) IterateTerminalAudit(ctx context.Context, terminalID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListTerminalAudit(ctx, terminalID, page)
	})
}

// ── Devices ──────────────────────────────────────────────────────────────

// ListDevices lists devices; filters limit, cursor, includeTotal, storeId,
// registerId, vendorCode, deviceModelId, deviceType, lifecycleStatus,
// ownershipStatus, integrationChannel, q.
func (m *PhysicalPosManagementClient) ListDevices(ctx context.Context, filters map[string]any) (*PhysicalPosPage, error) {
	return m.client.posList(ctx, "/devices", filters, "listDevices")
}

// IterateDevices yields every device matching the filters.
func (m *PhysicalPosManagementClient) IterateDevices(ctx context.Context, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, m.ListDevices)
}

// CreateDevice registers a device: {vendorCode, deviceModelId,
// serialNumber, displayName?, externalRef?, storeId?, registerId?,
// integrationChannel, channelVariant?, appMode?}. The result holds
// data.device, Replayed, StatusCode and IdempotencyKey.
func (m *PhysicalPosManagementClient) CreateDevice(ctx context.Context, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	return m.keyedWrite(ctx, "/devices", request, "createDevice", idempotencyKey)
}

// GetDevice reads one device (GET /devices/{deviceId}).
func (m *PhysicalPosManagementClient) GetDevice(ctx context.Context, deviceID string) (json.RawMessage, error) {
	path, err := posPath("/devices/", deviceID, "deviceId", "")
	if err != nil {
		return nil, err
	}
	return m.read(ctx, path, nil, "device")
}

// UpdateDevice changes a device: {rowVersion, displayName?, externalRef?,
// integrationChannel?, channelVariant?, appMode?}; nil clears displayName,
// externalRef, channelVariant and appMode.
func (m *PhysicalPosManagementClient) UpdateDevice(ctx context.Context, deviceID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "PATCH", "/devices/", deviceID, "deviceId", "", request, "updateDevice", "device")
}

// AssignDevice assigns the device to a store (and register): {rowVersion,
// storeId, registerId?, reasonCode}. Returns {device, assignment,
// previousAssignmentId}.
func (m *PhysicalPosManagementClient) AssignDevice(ctx context.Context, deviceID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/devices/", deviceID, "deviceId", "/assignment", request, "assignDevice", "")
}

// CloseDeviceAssignment moves the device to storage: {rowVersion,
// reasonCode}. The result holds data.device and data.closedAssignment,
// Replayed and IdempotencyKey.
func (m *PhysicalPosManagementClient) CloseDeviceAssignment(ctx context.Context, deviceID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	path, err := posPath("/devices/", deviceID, "deviceId", "/assignment/close")
	if err != nil {
		return nil, err
	}
	return m.keyedWrite(ctx, path, request, "statusChange", idempotencyKey)
}

// ChangeDeviceLifecycle moves the device to target ACTIVE, SUSPENDED or
// MAINTENANCE: {rowVersion, target, reasonCode}.
func (m *PhysicalPosManagementClient) ChangeDeviceLifecycle(ctx context.Context, deviceID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/devices/", deviceID, "deviceId", "/lifecycle", request, "changeDeviceLifecycle", "device")
}

// RetireDevice retires the device for good: {rowVersion, reasonCode}. The
// result holds data.device, Replayed and IdempotencyKey.
func (m *PhysicalPosManagementClient) RetireDevice(ctx context.Context, deviceID string, request map[string]any, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	path, err := posPath("/devices/", deviceID, "deviceId", "/retire")
	if err != nil {
		return nil, err
	}
	return m.keyedWrite(ctx, path, request, "statusChange", idempotencyKey)
}

// ListDeviceAssignments lists a device's store assignments; filters limit,
// cursor.
func (m *PhysicalPosManagementClient) ListDeviceAssignments(ctx context.Context, deviceID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/devices/", deviceID, "deviceId", "/assignments", filters)
}

// IterateDeviceAssignments yields every assignment of a device.
func (m *PhysicalPosManagementClient) IterateDeviceAssignments(ctx context.Context, deviceID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListDeviceAssignments(ctx, deviceID, page)
	})
}

// ListDeviceAudit lists a device's audit entries; filters limit, cursor.
func (m *PhysicalPosManagementClient) ListDeviceAudit(ctx context.Context, deviceID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/devices/", deviceID, "deviceId", "/audit", filters)
}

// IterateDeviceAudit yields every audit entry of a device.
func (m *PhysicalPosManagementClient) IterateDeviceAudit(ctx context.Context, deviceID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListDeviceAudit(ctx, deviceID, page)
	})
}

// ── Device identity (physical_pos:identity for writes) ──────────────────

// ListDeviceKeys lists the device's identity keys, fingerprints only
// (GET /devices/{deviceId}/keys). Each key: keyId, algorithm, keyStatus
// (ACTIVE, SUPERSEDED, REVOKED), enrolledVia (PAIRING, ACTIVATION_CODE,
// ROTATION), fingerprint, lineageRootKeyId, rotatedFromKeyId, activatedAt,
// supersededAt, revokedAt, revokeReason, rowVersion. Filters limit, cursor.
func (m *PhysicalPosManagementClient) ListDeviceKeys(ctx context.Context, deviceID string, filters map[string]any) (*PhysicalPosPage, error) {
	return m.history(ctx, "/devices/", deviceID, "deviceId", "/keys", filters)
}

// IterateDeviceKeys yields every identity key of a device.
func (m *PhysicalPosManagementClient) IterateDeviceKeys(ctx context.Context, deviceID string, filters map[string]any) iter.Seq2[json.RawMessage, error] {
	return posIterate(ctx, filters, func(ctx context.Context, page map[string]any) (*PhysicalPosPage, error) {
		return m.ListDeviceKeys(ctx, deviceID, page)
	})
}

// RevokeDeviceKey revokes a device key: {rowVersion (the key's),
// reasonCode}; reasonCode DEVICE_LOST, DEVICE_STOLEN, KEY_COMPROMISED,
// DEVICE_REPLACED, MERCHANT_REQUEST or OTHER. Returns {key, lifecycleStatus,
// replayed}.
func (m *PhysicalPosManagementClient) RevokeDeviceKey(ctx context.Context, deviceID, keyID string, request map[string]any) (json.RawMessage, error) {
	return m.keyAction(ctx, deviceID, keyID, "/revoke", request, "revokeDeviceKey")
}

// RevertDeviceEnrollment reverts an activation-code enrollment (revokes the
// key, suspends the device): {reasonCode}; NOT_MY_DEVICE,
// SUSPECTED_PHISHING, WRONG_DEVICE or OTHER. Returns {key, lifecycleStatus,
// replayed}.
func (m *PhysicalPosManagementClient) RevertDeviceEnrollment(ctx context.Context, deviceID, keyID string, request map[string]any) (json.RawMessage, error) {
	return m.keyAction(ctx, deviceID, keyID, "/revert-enrollment", request, "revertDeviceEnrollment")
}

// RevokeActivationCode revokes the device's open activation code:
// {reasonCode}; MERCHANT_REVOKED, DEVICE_LOST, DEVICE_STOLEN or OTHER.
// Returns {deviceId, revokedAt, revokeReason}.
func (m *PhysicalPosManagementClient) RevokeActivationCode(ctx context.Context, deviceID string, request map[string]any) (json.RawMessage, error) {
	return m.record(ctx, "POST", "/devices/", deviceID, "deviceId", "/activation-code/revoke", request, "revokeActivationCode", "")
}

// ── End-of-day policy (account → store → device) ─────────────────────────
//
// A level's policy replaces the level above it entirely. Set… sends the full
// setting: {mode (MANUAL, SCHEDULED, DEVICE_AUTO), scheduleTime?, timeZone?,
// scheduleDays?, retryWindowMinutes?, retryIntervalMinutes?,
// offlineBehavior?, overdueAlertHours?, expectedRowVersion?}. Clear… resets
// the account to MANUAL, or removes a store / device override (it then
// inherits). expectedRowVersion is the policy's rowVersion you read (409
// physical_pos.version_conflict when it changed); leave it out (0 for
// Clear…) for last write wins. Each call returns the policy.

// GetAccountSettlementPolicy reads the account's end-of-day policy.
func (m *PhysicalPosManagementClient) GetAccountSettlementPolicy(ctx context.Context) (json.RawMessage, error) {
	return m.policy(ctx, "GET", "/settlement-policy", nil, 0)
}

// SetAccountSettlementPolicy replaces the account's end-of-day policy.
func (m *PhysicalPosManagementClient) SetAccountSettlementPolicy(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	return m.policy(ctx, "PUT", "/settlement-policy", request, 0)
}

// ClearAccountSettlementPolicy resets the account's policy to MANUAL.
func (m *PhysicalPosManagementClient) ClearAccountSettlementPolicy(ctx context.Context, expectedRowVersion int64) (json.RawMessage, error) {
	return m.policy(ctx, "DELETE", "/settlement-policy", nil, expectedRowVersion)
}

// GetStoreSettlementPolicy reads a store's end-of-day policy override.
func (m *PhysicalPosManagementClient) GetStoreSettlementPolicy(ctx context.Context, storeID string) (json.RawMessage, error) {
	return m.scopedPolicy(ctx, "GET", "/stores/", storeID, "storeId", nil, 0)
}

// SetStoreSettlementPolicy replaces a store's end-of-day policy override.
func (m *PhysicalPosManagementClient) SetStoreSettlementPolicy(ctx context.Context, storeID string, request map[string]any) (json.RawMessage, error) {
	return m.scopedPolicy(ctx, "PUT", "/stores/", storeID, "storeId", request, 0)
}

// ClearStoreSettlementPolicy removes a store's override; the store then
// inherits the account's policy.
func (m *PhysicalPosManagementClient) ClearStoreSettlementPolicy(ctx context.Context, storeID string, expectedRowVersion int64) (json.RawMessage, error) {
	return m.scopedPolicy(ctx, "DELETE", "/stores/", storeID, "storeId", nil, expectedRowVersion)
}

// GetDeviceSettlementPolicy reads a device's end-of-day policy override.
func (m *PhysicalPosManagementClient) GetDeviceSettlementPolicy(ctx context.Context, deviceID string) (json.RawMessage, error) {
	return m.scopedPolicy(ctx, "GET", "/devices/", deviceID, "deviceId", nil, 0)
}

// SetDeviceSettlementPolicy replaces a device's end-of-day policy override.
// DEVICE_AUTO only on devices that report their end-of-day results.
func (m *PhysicalPosManagementClient) SetDeviceSettlementPolicy(ctx context.Context, deviceID string, request map[string]any) (json.RawMessage, error) {
	return m.scopedPolicy(ctx, "PUT", "/devices/", deviceID, "deviceId", request, 0)
}

// ClearDeviceSettlementPolicy removes a device's override; the device then
// inherits its store's or the account's policy.
func (m *PhysicalPosManagementClient) ClearDeviceSettlementPolicy(ctx context.Context, deviceID string, expectedRowVersion int64) (json.RawMessage, error) {
	return m.scopedPolicy(ctx, "DELETE", "/devices/", deviceID, "deviceId", nil, expectedRowVersion)
}

// GetEffectiveSettlementPolicy returns the policy a device follows
// (GET /devices/{deviceId}/settlement-policy/effective): {deviceId,
// sourceScopeType (ACCOUNT, STORE, DEVICE), policy,
// serverSettlementSupported, deviceAutoSupported, nextScheduledAt,
// lastSettlementAt, overdue, scheduleWarning}.
func (m *PhysicalPosManagementClient) GetEffectiveSettlementPolicy(ctx context.Context, deviceID string) (json.RawMessage, error) {
	path, err := posPath("/devices/", deviceID, "deviceId", "/settlement-policy/effective")
	if err != nil {
		return nil, err
	}
	return m.read(ctx, path, nil, "effective")
}

// ── Transport ────────────────────────────────────────────────────────────

func posPath(prefix, id, field, suffix string) (string, error) {
	segment, err := posPathID(id, field)
	if err != nil {
		return "", err
	}
	return prefix + segment + suffix, nil
}

// read GETs a path and returns data.<name> (or data when name is "").
func (m *PhysicalPosManagementClient) read(ctx context.Context, path string, query url.Values, name string) (json.RawMessage, error) {
	r, err := m.client.posSend(ctx, "GET", path, nil, query, "")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return r.Data, nil
	}
	return posRecord(r, name), nil
}

// record sends a checked body to prefix+id+suffix and returns data.<name>
// (or data when name is "").
func (m *PhysicalPosManagementClient) record(ctx context.Context, method, prefix, id, field, suffix string, request map[string]any, operation, name string) (json.RawMessage, error) {
	path, err := posPath(prefix, id, field, suffix)
	if err != nil {
		return nil, err
	}
	body, err := posBuildBody(request, operation)
	if err != nil {
		return nil, err
	}
	r, err := m.client.posSend(ctx, method, path, body, nil, "")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return r.Data, nil
	}
	return posRecord(r, name), nil
}

func (m *PhysicalPosManagementClient) history(ctx context.Context, prefix, id, field, suffix string, filters map[string]any) (*PhysicalPosPage, error) {
	path, err := posPath(prefix, id, field, suffix)
	if err != nil {
		return nil, err
	}
	return m.client.posList(ctx, path, filters, "history")
}

func (m *PhysicalPosManagementClient) keyAction(ctx context.Context, deviceID, keyID, action string, request map[string]any, operation string) (json.RawMessage, error) {
	device, err := posPathID(deviceID, "deviceId")
	if err != nil {
		return nil, err
	}
	return m.record(ctx, "POST", "/devices/"+device+"/keys/", keyID, "keyId", action, request, operation, "")
}

func (m *PhysicalPosManagementClient) keyedWrite(ctx context.Context, path string, request map[string]any, operation, idempotencyKey string) (*PhysicalPosWriteResult, error) {
	body, err := posBuildBody(request, operation)
	if err != nil {
		return nil, err
	}
	key, err := posIdempotencyKeyOf(idempotencyKey)
	if err != nil {
		return nil, err
	}
	r, err := m.client.posSend(ctx, "POST", path, body, nil, key)
	if err != nil {
		return nil, err
	}
	return posWriteResult(r, key), nil
}

func (m *PhysicalPosManagementClient) scopedPolicy(ctx context.Context, method, prefix, id, field string, request map[string]any, expectedRowVersion int64) (json.RawMessage, error) {
	path, err := posPath(prefix, id, field, "/settlement-policy")
	if err != nil {
		return nil, err
	}
	return m.policy(ctx, method, path, request, expectedRowVersion)
}

// policy GETs, PUTs (a checked setSettlementPolicy body) or DELETEs
// (?expectedRowVersion= unless 0) an end-of-day policy and returns
// data.policy.
func (m *PhysicalPosManagementClient) policy(ctx context.Context, method, path string, request map[string]any, expectedRowVersion int64) (json.RawMessage, error) {
	var body any
	if method == "PUT" {
		checked, err := posBuildBody(request, "setSettlementPolicy")
		if err != nil {
			return nil, err
		}
		body = checked
	}
	var query url.Values
	if method == "DELETE" && expectedRowVersion != 0 {
		version, err := posRowVersion(expectedRowVersion, "expectedRowVersion")
		if err != nil {
			return nil, err
		}
		query = url.Values{"expectedRowVersion": {strconv.FormatInt(version, 10)}}
	}
	r, err := m.client.posSend(ctx, method, path, body, query, "")
	if err != nil {
		return nil, err
	}
	return posRecord(r, "policy"), nil
}

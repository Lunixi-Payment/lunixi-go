package lunixi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PhysicalPosWebhookEvent is a verified physical_pos.* webhook.
//
// Data is the envelope's data (or its payloadJson, as an object or JSON
// text). Amounts in it are minor-unit text ("4500"), as in the HTTP API;
// they are checked after the signature. Delivery order is not guaranteed and
// an event can arrive more than once: deduplicate by ID and ignore an event
// whose HistorySeq is not greater than the last one you applied for the same
// data.paymentId. End-of-day events (Category settlement) carry no
// HistorySeq; deduplicate them by ID.
type PhysicalPosWebhookEvent struct {
	ID          string
	Type        string
	Category    string
	Environment string
	CreatedAt   string
	// HistorySeq is data.historySeq, nil when absent (settlement events).
	HistorySeq *int64
	// Original is the reversal's original payment, nil on a root payment.
	Original *PhysicalPosOriginalPayment
	// Cashier is set for a payment started from a device cashier session
	// (register mode), nil otherwise (the payload then has no cashier key).
	Cashier *PhysicalPosCashier
	// IsDeviceSale is true for physical_pos.payment.device_sale, a sale made
	// directly on the device (data.origin, data.deviceOccurredAt).
	IsDeviceSale bool
	Data         json.RawMessage
	// Raw is the signed envelope. It can contain customer data: do not log
	// it or persist it outside your data-retention policy.
	Raw json.RawMessage
}

// PhysicalPosOriginalPayment is data.original of a reversal event; Origin is
// API or DEVICE_DIRECT.
type PhysicalPosOriginalPayment struct {
	PaymentID string
	Origin    string
}

// PhysicalPosCashier is data.cashier of a payment event.
type PhysicalPosCashier struct {
	CashierID   string
	CashierCode string
}

// DecodeData decodes Data using json.Number for interface values.
func (e *PhysicalPosWebhookEvent) DecodeData(target any) error { return decodeNumbers(e.Data, target) }

// VerifyPhysicalPos verifies a webhook (signature, event id, timestamp) with
// the configured secrets and reads it as a physical POS event. There is no
// way in without the signature: pass the exact raw body and the request
// headers. Any failure is ErrWebhook.
func (v *WebhookVerifier) VerifyPhysicalPos(raw []byte, headers http.Header) (*PhysicalPosWebhookEvent, error) {
	event, err := v.Verify(raw, headers)
	if err != nil {
		return nil, err
	}
	return ParsePhysicalPosWebhookEvent(event)
}

// ParsePhysicalPosWebhookEvent reads an event already verified by
// WebhookVerifier as a physical POS event. A WebhookEvent can only be built
// by the verifier, so an unverified body cannot be read here. The event type
// comes from the signed body; a type outside physical_pos.*, an envelope
// without data, or an amount that is not minor-unit text is ErrWebhook.
func ParsePhysicalPosWebhookEvent(event *WebhookEvent) (*PhysicalPosWebhookEvent, error) {
	if event == nil {
		return nil, fmt.Errorf("%w: a verified event is required", ErrWebhook)
	}
	var envelope map[string]json.RawMessage
	if decodeNumbers(event.raw, &envelope) != nil || envelope == nil {
		return nil, fmt.Errorf("%w: the physical POS webhook body is not an event envelope", ErrWebhook)
	}
	eventType := posFirstText(envelope["type"], envelope["eventType"])
	if eventType == "" {
		eventType = event.Type()
	}
	if !strings.HasPrefix(eventType, "physical_pos.") {
		return nil, fmt.Errorf("%w: not a physical POS webhook event", ErrWebhook)
	}
	data, err := posWebhookPayload(envelope)
	if err != nil {
		return nil, err
	}
	if err = posValidateMoney(data); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWebhook, err)
	}
	var fields map[string]any
	_ = decodeNumbers(data, &fields)
	id := posFirstText(envelope["id"], envelope["eventId"])
	if id == "" {
		id = event.ID()
	}
	createdAt := posFirstText(envelope["createdAt"], envelope["occurredAt"], envelope["timestamp"])
	out := &PhysicalPosWebhookEvent{
		ID:           id,
		Type:         eventType,
		Category:     posWebhookCategory(eventType),
		Environment:  posFirstText(envelope["environment"]),
		CreatedAt:    createdAt,
		Original:     posWebhookOriginal(fields),
		Cashier:      posWebhookCashier(fields),
		IsDeviceSale: eventType == PhysicalPosEventPaymentDeviceSale,
		Data:         data,
		Raw:          append(json.RawMessage(nil), event.raw...),
	}
	if raw, ok := posRawField(data, "historySeq"); ok {
		out.HistorySeq = posSafeInteger(raw)
	}
	return out, nil
}

// posWebhookPayload is the envelope's data when it is a non-empty object,
// otherwise its payloadJson (an object or JSON text).
func posWebhookPayload(envelope map[string]json.RawMessage) (json.RawMessage, error) {
	if posNonEmptyObject(envelope["data"]) {
		return append(json.RawMessage(nil), envelope["data"]...), nil
	}
	payload := envelope["payloadJson"]
	var text string
	if json.Unmarshal(payload, &text) == nil {
		if !json.Valid([]byte(text)) {
			return nil, fmt.Errorf("%w: the physical POS webhook payloadJson is not valid JSON", ErrWebhook)
		}
		payload = json.RawMessage(text)
	}
	if posNonEmptyObject(payload) {
		return append(json.RawMessage(nil), payload...), nil
	}
	return nil, fmt.Errorf("%w: the physical POS webhook carries no event data (data / payloadJson)", ErrWebhook)
}

func posNonEmptyObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && len(object) > 0
}

func posRawField(raw json.RawMessage, name string) (json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil, false
	}
	value, ok := object[name]
	return value, ok
}

func posFirstText(values ...json.RawMessage) string {
	for _, raw := range values {
		var text string
		if json.Unmarshal(raw, &text) == nil && text != "" {
			return text
		}
	}
	return ""
}

func posText(value any) string {
	text, _ := value.(string)
	return text
}

// posWebhookOriginal reads data.original: both paymentId and origin as
// non-empty text (other keys are dropped), otherwise nil.
func posWebhookOriginal(data map[string]any) *PhysicalPosOriginalPayment {
	original, ok := data["original"].(map[string]any)
	if !ok {
		return nil
	}
	paymentID, origin := posText(original["paymentId"]), posText(original["origin"])
	if paymentID == "" || origin == "" {
		return nil
	}
	return &PhysicalPosOriginalPayment{PaymentID: paymentID, Origin: origin}
}

// posWebhookCashier reads data.cashier: both cashierId and cashierCode as
// non-empty text (other keys are dropped), otherwise nil.
func posWebhookCashier(data map[string]any) *PhysicalPosCashier {
	cashier, ok := data["cashier"].(map[string]any)
	if !ok {
		return nil
	}
	cashierID, cashierCode := posText(cashier["cashierId"]), posText(cashier["cashierCode"])
	if cashierID == "" || cashierCode == "" {
		return nil
	}
	return &PhysicalPosCashier{CashierID: cashierID, CashierCode: cashierCode}
}

func posWebhookCategory(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "physical_pos.payment."):
		return PhysicalPosWebhookCategoryPayment
	case strings.HasPrefix(eventType, "physical_pos.fiscal_document."):
		return PhysicalPosWebhookCategoryFiscalDocument
	case strings.HasPrefix(eventType, "physical_pos.settlement."):
		return PhysicalPosWebhookCategorySettlement
	}
	return PhysicalPosWebhookCategoryOther
}

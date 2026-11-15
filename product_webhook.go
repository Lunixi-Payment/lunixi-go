package lunixi

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Readers of the bank-account, bank-transfer and insurance webhook data.
// Each takes an event verified by WebhookVerifier (a WebhookEvent can only be
// built by the verifier), checks the event type from the signed body and
// decodes data into a typed value. A type outside the product's events, an
// envelope without data, a value of the wrong JSON type or an amount that is
// not minor-unit text is ErrWebhook. Values without a value are nil.

// BankAccountTransactionReceivedData is the data of a
// bank_account.transaction.received webhook.
type BankAccountTransactionReceivedData struct {
	TransactionID          string  `json:"transactionId"`
	AccountID              string  `json:"accountId"`
	ConnectionID           string  `json:"connectionId"`
	BankEftCode            string  `json:"bankEftCode"`
	BankName               string  `json:"bankName"`
	Direction              string  `json:"direction"`
	AmountMinor            string  `json:"amountMinor"`
	Currency               string  `json:"currency"`
	BookedAt               string  `json:"bookedAt"`
	ValueAt                *string `json:"valueAt"`
	ReceivedAt             string  `json:"receivedAt"`
	CanonicalType          string  `json:"canonicalType"`
	Description            *string `json:"description"`
	CounterpartyNameMasked *string `json:"counterpartyNameMasked"`
	CounterpartyIbanMasked *string `json:"counterpartyIbanMasked"`
	ReceiptNo              *string `json:"receiptNo"`
}

// BankTransferWebhookData is the data of a bank_transfer.* webhook.
type BankTransferWebhookData struct {
	TransferID         string  `json:"transferId"`
	ConnectionID       string  `json:"connectionId"`
	AccountID          string  `json:"accountId"`
	BankEftCode        string  `json:"bankEftCode"`
	BankName           string  `json:"bankName"`
	Status             string  `json:"status"`
	Rail               *string `json:"rail"`
	AmountMinor        string  `json:"amountMinor"`
	Currency           string  `json:"currency"`
	ReceiverIbanMasked string  `json:"receiverIbanMasked"`
	ReceiverNameMasked string  `json:"receiverNameMasked"`
	PaymentPurpose     *string `json:"paymentPurpose"`
	Description        *string `json:"description"`
	FailureCode        *string `json:"failureCode"`
	IdempotencyKey     string  `json:"idempotencyKey"`
	CreatedAt          string  `json:"createdAt"`
	CompletedAt        *string `json:"completedAt"`
	FailedAt           *string `json:"failedAt"`
	ReturnedAt         *string `json:"returnedAt"`
}

// InsuranceProvider is the provider behind an insurance quote or order (for
// information; Code is the Lunixi catalogue code).
type InsuranceProvider struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	LogoURL      *string `json:"logoUrl"`
	ConnectionID string  `json:"connectionId"`
}

// InsuranceWebhookPolicy is a policy in an insurance webhook's data.
type InsuranceWebhookPolicy struct {
	PolicyID       string  `json:"policyId"`
	BasketItemID   string  `json:"basketItemId"`
	PolicyNumber   *string `json:"policyNumber"`
	PolicyStatus   string  `json:"policyStatus"`
	DocumentStatus string  `json:"documentStatus"`
	// DocumentURL is the GetPolicyDocument address while the policy is in
	// force, otherwise "". Downloading it is a signed API call.
	DocumentURL string `json:"documentUrl"`
}

// InsuranceWebhookData is the data of every order insurance.* webhook (all
// InsuranceOrderEventTypes). No other key is delivered; no personal data.
type InsuranceWebhookData struct {
	// EventID is the same as the envelope's id; dedupe on it.
	EventID            string            `json:"eventId"`
	EventType          string            `json:"eventType"`
	MerchantID         string            `json:"merchantId"`
	InsuranceOrderID   string            `json:"insuranceOrderId"`
	PaymentIntentID    *string           `json:"paymentIntentId"`
	MerchantOrderID    *string           `json:"merchantOrderId"`
	InsuranceType      string            `json:"insuranceType"`
	PremiumCollectedBy string            `json:"premiumCollectedBy"`
	Provider           InsuranceProvider `json:"provider"`
	Status             string            `json:"status"`
	CollectionStatus   string            `json:"collectionStatus"`
	PolicyStatus       string            `json:"policyStatus"`
	RefundStatus       string            `json:"refundStatus"`
	DocumentStatus     string            `json:"documentStatus"`
	// PremiumAmount is minor-unit text.
	PremiumAmount string                   `json:"premiumAmount"`
	Currency      string                   `json:"currency"`
	Policies      []InsuranceWebhookPolicy `json:"policies"`
	// ErrorCode is set only on insurance.policy.failed and
	// insurance.refund.required.
	ErrorCode *string `json:"errorCode"`
	CreatedAt string  `json:"createdAt"`
}

// InsuranceWebhookLead is the lead in an insurance.lead.* webhook. No
// personal data, no note, no provider reference.
type InsuranceWebhookLead struct {
	LeadID        string  `json:"leadId"`
	InsuranceType string  `json:"insuranceType"`
	SchemaVersion int     `json:"schemaVersion"`
	Status        string  `json:"status"`
	SubmittedAt   *string `json:"submittedAt"`
	CreatedAt     string  `json:"createdAt"`
}

// InsuranceLeadWebhookData is the data of an insurance.lead.* webhook (all
// InsuranceLeadEventTypes). No other key is delivered.
type InsuranceLeadWebhookData struct {
	// EventID is the same as the envelope's id; dedupe on it.
	EventID    string               `json:"eventId"`
	EventType  string               `json:"eventType"`
	MerchantID string               `json:"merchantId"`
	Lead       InsuranceWebhookLead `json:"lead"`
	CreatedAt  string               `json:"createdAt"`
}

// ParseBankAccountTransactionReceived reads a verified
// bank_account.transaction.received event.
func ParseBankAccountTransactionReceived(event *WebhookEvent) (*BankAccountTransactionReceivedData, error) {
	var data BankAccountTransactionReceivedData
	if err := decodeProductWebhook(event, BankConnectEventTypes(), "bank-account", &data); err != nil {
		return nil, err
	}
	if err := webhookMinor(data.AmountMinor, "amountMinor"); err != nil {
		return nil, err
	}
	return &data, nil
}

// ParseBankTransferWebhook reads a verified bank_transfer.* event.
func ParseBankTransferWebhook(event *WebhookEvent) (*BankTransferWebhookData, error) {
	var data BankTransferWebhookData
	if err := decodeProductWebhook(event, BankTransferEventTypes(), "bank-transfer", &data); err != nil {
		return nil, err
	}
	if err := webhookMinor(data.AmountMinor, "amountMinor"); err != nil {
		return nil, err
	}
	return &data, nil
}

// ParseInsuranceWebhook reads a verified order insurance.* event (every
// InsuranceOrderEventTypes type). A lead event is read with
// ParseInsuranceLeadWebhook.
func ParseInsuranceWebhook(event *WebhookEvent) (*InsuranceWebhookData, error) {
	var data InsuranceWebhookData
	if err := decodeProductWebhook(event, InsuranceOrderEventTypes(), "insurance order", &data); err != nil {
		return nil, err
	}
	if err := webhookMinor(data.PremiumAmount, "premiumAmount"); err != nil {
		return nil, err
	}
	return &data, nil
}

// ParseInsuranceLeadWebhook reads a verified insurance.lead.* event.
func ParseInsuranceLeadWebhook(event *WebhookEvent) (*InsuranceLeadWebhookData, error) {
	var data InsuranceLeadWebhookData
	if err := decodeProductWebhook(event, InsuranceLeadEventTypes(), "insurance lead", &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func decodeProductWebhook(event *WebhookEvent, types []string, product string, target any) error {
	if event == nil {
		return fmt.Errorf("%w: a verified event is required", ErrWebhook)
	}
	if !slices.Contains(types, event.eventType) {
		return fmt.Errorf("%w: the event type is not a %s event", ErrWebhook, product)
	}
	if len(event.data) == 0 || string(event.data) == "null" {
		return fmt.Errorf("%w: the %s webhook has no data", ErrWebhook, product)
	}
	if err := json.Unmarshal(event.data, target); err != nil {
		return fmt.Errorf("%w: the %s webhook data does not match its schema", ErrWebhook, product)
	}
	return nil
}

func webhookMinor(value, field string) error {
	if !validMinor(value, false, false) {
		return fmt.Errorf("%w: %s must be minor-unit text", ErrWebhook, field)
	}
	return nil
}

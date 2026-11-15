package lunixi

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
)

// Para Transferi (BANK_TRANSFER, havale / EFT / FAST): the buyer sends the
// money from their own bank to the merchant's bank account. The money never
// passes through Lunixi; the incoming transaction is read from the merchant's
// connected bank account and matched to the payment by the payment code in the
// description. Start it with WithMethodFields(PaymentMethodBankTransfer, "",
// map[string]any{"bankCode": ...}); the answer is a DISPLAY action read with
// Response.BankTransferInstruction.

// BankTransferOtherBanksCode is the "other banks" option of the bankCode
// field: the buyer pays by EFT/FAST from a bank that is not listed.
const BankTransferOtherBanksCode = "9999"

// BankTransferExceptionLimitMax is the largest limit of ListBankTransferExceptions.
const BankTransferExceptionLimitMax = 200

// Kind of a bank transfer exception.
const (
	// BankTransferExceptionKindLate: arrived after the deadline (within 7 days).
	BankTransferExceptionKindLate = "LATE"
	// BankTransferExceptionKindUnderpaid: the deadline passed with less than
	// expected; the payment waits for your decision.
	BankTransferExceptionKindUnderpaid = "UNDERPAID"
	// BankTransferExceptionKindOverpaid: more than expected arrived; refund the excess.
	BankTransferExceptionKindOverpaid = "OVERPAID"
)

// Status of a bank transfer exception.
const (
	BankTransferExceptionStatusPendingDecision = "PENDING_DECISION"
	BankTransferExceptionStatusAccepted        = "ACCEPTED"
	BankTransferExceptionStatusRefundPending   = "REFUND_PENDING"
	BankTransferExceptionStatusRefunded        = "REFUNDED"
)

// Bank transfer webhook events of a BANK_TRANSFER payment (payment.* envelope,
// payload bankTransfer: { expectedMinor, receivedMinor, remainingMinor,
// excessMinor, currency, late, transfers[] }; events that open an exception
// also carry exceptionId, kind and amountMinor). They do not change the
// payment's status: the outcome is still payment.captured / payment.failed.
// The names differ from Bank Connect's bank_transfer.* events.
const (
	PaymentBankTransferEventPartiallyReceived = "payment.bank_transfer.partially_received"
	PaymentBankTransferEventUnderpaid         = "payment.bank_transfer.underpaid"
	PaymentBankTransferEventOverpaid          = "payment.bank_transfer.overpaid"
	PaymentBankTransferEventLateArrival       = "payment.bank_transfer.late_arrival"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// BankTransferExceptionFilters narrows ListBankTransferExceptions. Empty
// fields are not sent. Environment is "TEST" or "LIVE"; Status is a
// BankTransferExceptionStatus* value; Limit, when set, is 1–200.
type BankTransferExceptionFilters struct {
	Environment string
	Status      string
	Limit       *int
}

func (f BankTransferExceptionFilters) query() (url.Values, error) {
	query := url.Values{}
	if f.Environment != "" {
		if f.Environment != "TEST" && f.Environment != "LIVE" {
			return nil, invalid("environment must be TEST or LIVE")
		}
		query.Set("environment", f.Environment)
	}
	if f.Status != "" {
		switch f.Status {
		case BankTransferExceptionStatusPendingDecision, BankTransferExceptionStatusAccepted, BankTransferExceptionStatusRefundPending, BankTransferExceptionStatusRefunded:
		default:
			return nil, invalid("status must be PENDING_DECISION, ACCEPTED, REFUND_PENDING or REFUNDED")
		}
		query.Set("status", f.Status)
	}
	if f.Limit != nil {
		if *f.Limit < 1 || *f.Limit > BankTransferExceptionLimitMax {
			return nil, invalid("limit must be an integer between 1 and 200")
		}
		query.Set("limit", strconv.Itoa(*f.Limit))
	}
	if len(query) == 0 {
		return nil, nil
	}
	return query, nil
}

// ListBankTransferExceptions lists late (LATE), short (UNDERPAID) and excess
// (OVERPAID) transfers of BANK_TRANSFER payments
// (GET /payments/bank-transfer/exceptions). Data is { items: [{ id, paymentId,
// kind, status, environment, amountMinor, currency, expectedMinor,
// receivedMinor, counterpartyNameMasked, acceptedPaymentId, refundSource,
// decidedAt, refundedAt, createdAt }] }; the sender name is masked.
func (p *PaymentClient) ListBankTransferExceptions(ctx context.Context, filters BankTransferExceptionFilters) (*Response, error) {
	query, err := filters.query()
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: "/api/v1/payments/bank-transfer/exceptions", Query: query})
}

func bankTransferExceptionPath(exceptionID, decision string) (string, error) {
	if !uuidPattern.MatchString(exceptionID) {
		return "", invalid("exceptionId must be the UUID of a bank transfer exception (items[].id)")
	}
	return "/api/v1/payments/bank-transfer/exceptions/" + exceptionID + "/" + decision, nil
}

// AcceptBankTransferException accepts an exception
// (POST /payments/bank-transfer/exceptions/{id}/accept, no body; the key needs
// payment:refund). LATE opens a new captured payment for the same order
// (acceptedPaymentId); UNDERPAID captures the payment with the amount received.
// OVERPAID cannot be accepted (409 payment.bank_transfer.decision_not_supported).
// Data is the exception or null.
func (p *PaymentClient) AcceptBankTransferException(ctx context.Context, exceptionID string) (*Response, error) {
	path, err := bankTransferExceptionPath(exceptionID, "accept")
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: path})
}

// MarkBankTransferExceptionRefunded reports that you refunded the money of an
// exception to the sender from your own bank
// (POST /payments/bank-transfer/exceptions/{id}/refunded, no body; the key
// needs payment:refund). Lunixi does not send the refund. For UNDERPAID the
// waiting payment closes as failed. Data is the exception or null.
func (p *PaymentClient) MarkBankTransferExceptionRefunded(ctx context.Context, exceptionID string) (*Response, error) {
	path, err := bankTransferExceptionPath(exceptionID, "refunded")
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: path})
}

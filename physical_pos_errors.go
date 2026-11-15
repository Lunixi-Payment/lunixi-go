package lunixi

import (
	"fmt"
	"time"
)

// PhysicalPosCardDataRejectedError: a physical POS request carried card data.
// The card is read on the device; the API never takes a card number, expiry
// date, security code, track or PIN. Raised before anything is sent for a
// card-data field name in a body or filter set, or a card number in
// merchantReference, an item name or tax group code, or the merchantReference
// / q filters (StatusCode 0; errors.Is(err, ErrConfiguration) is true). The
// gateway's own refusal (physical_pos.payment.card_data_rejected) maps to the
// same type and unwraps to its *APIError. Field is the path of the offending
// field (items[1].cvv), never its value; empty when the gateway gave none.
type PhysicalPosCardDataRejectedError struct {
	Field          string
	StatusCode     int
	Code           string
	RequestID      string
	IdempotencyKey string
	cause          error
}

func (e *PhysicalPosCardDataRejectedError) Error() string {
	field := e.Field
	if field == "" {
		field = "unknown"
	}
	return fmt.Sprintf("lunixi: physical POS requests must not contain card data: the card is read on the device (field %s)", field)
}

func (e *PhysicalPosCardDataRejectedError) Unwrap() error { return e.cause }

// PhysicalPosVoidAfterSettlementError: 409
// physical_pos.payment.void_after_settlement. The payment is already in a
// device end-of-day, so it can no longer be voided; return the amount with a
// REFUND reversal on the same payment (with a new Idempotency-Key).
// AllowedOperation is details.allowedOperation (REFUND).
type PhysicalPosVoidAfterSettlementError struct {
	AllowedOperation string
	StatusCode       int
	Code             string
	RequestID        string
	IdempotencyKey   string
	cause            error
}

func (e *PhysicalPosVoidAfterSettlementError) Error() string {
	return fmt.Sprintf("lunixi: physical POS payment is already in an end-of-day and cannot be voided (HTTP %d, code %s)", e.StatusCode, e.Code)
}

func (e *PhysicalPosVoidAfterSettlementError) Unwrap() error { return e.cause }

// PhysicalPosRateLimitError: 429 RATE_LIMITED (payment writes are limited
// per API key). RetryAfter is the Retry-After header, or
// details.retryAfterSeconds when the header is missing; 0 when neither was
// given.
type PhysicalPosRateLimitError struct {
	RetryAfter     time.Duration
	StatusCode     int
	Code           string
	RequestID      string
	IdempotencyKey string
	cause          error
}

func (e *PhysicalPosRateLimitError) Error() string {
	return fmt.Sprintf("lunixi: physical POS request rate limited (HTTP %d, code %s, retry after %s)", e.StatusCode, e.Code, e.RetryAfter)
}

func (e *PhysicalPosRateLimitError) Unwrap() error { return e.cause }

// PhysicalPosOutcomeUnknownError: the outcome of a payment write (start,
// reversal, cancel) is unknown; the payment may or may not exist. Raised when
// an attempt got no HTTP answer (also while the body was read, or when the
// context ended), a 2xx without a readable payment, or a 5xx that does not
// say "not started" — and then for the whole call, whatever a later attempt
// answered (a 429 or 409 included).
//
// Call again with the same IdempotencyKey (safe: an existing payment comes
// back with Replayed true), or look it up with ListPayments and the
// merchantReference filter. A new key can start a second payment.
// StatusCode, Code and RequestID are the last HTTP answer's (0 / transport
// code when there was none); Attempts is the number of attempts made; the
// error unwraps to the last attempt's error.
type PhysicalPosOutcomeUnknownError struct {
	IdempotencyKey string
	Attempts       int
	StatusCode     int
	Code           string
	RequestID      string
	RetryAfter     time.Duration
	cause          error
}

func (e *PhysicalPosOutcomeUnknownError) Error() string {
	return fmt.Sprintf("lunixi: the outcome of the physical POS payment request is unknown after %d attempt(s): call again with the same Idempotency-Key, never a new one (last HTTP %d, code %s)", e.Attempts, e.StatusCode, e.Code)
}

func (e *PhysicalPosOutcomeUnknownError) Unwrap() error { return e.cause }

// PhysicalPosWriteError carries the Idempotency-Key a physical POS write sent
// (yours, or the one the SDK generated for an empty key) together with the
// error of that write, so the same key can be used again. It unwraps to the
// underlying error (*APIError, a context error …). The typed physical POS
// errors carry the key themselves and are not wrapped.
type PhysicalPosWriteError struct {
	IdempotencyKey string
	Err            error
}

func (e *PhysicalPosWriteError) Error() string { return e.Err.Error() }

func (e *PhysicalPosWriteError) Unwrap() error { return e.Err }

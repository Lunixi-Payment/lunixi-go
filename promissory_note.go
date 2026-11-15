package lunixi

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// PromissoryNoteMaxScheduleItems is the most rows of a promissory-note
// schedule, and the most notes of one payment.
const PromissoryNoteMaxScheduleItems = 24

// Status of one note (notes[].status).
const (
	PromissoryNoteStatusIssued    = "ISSUED"
	PromissoryNoteStatusPaid      = "PAID"
	PromissoryNoteStatusProtested = "PROTESTED"
	PromissoryNoteStatusCancelled = "CANCELLED"
)

// Document type of the issued notes, from your provider connection settings.
const (
	PromissoryNoteDocumentTypePromissoryNote = "PROMISSORY_NOTE"
	PromissoryNoteDocumentTypePaymentOrder   = "PAYMENT_ORDER"
)

// Per-note result of CancelPromissoryNotes (cancelResults[].result).
const (
	PromissoryNoteCancelResultCancelled = "CANCELLED"
	// PromissoryNoteCancelResultAlreadyClosed: the note was already cancelled.
	PromissoryNoteCancelResultAlreadyClosed = "ALREADY_CLOSED"
	// PromissoryNoteCancelResultNotCancellable: the note was collected or
	// protested; it cannot be cancelled.
	PromissoryNoteCancelResultNotCancellable = "NOT_CANCELLABLE"
	// PromissoryNoteCancelResultFailed: the note was not cancelled; errorCode
	// tells whether to try again.
	PromissoryNoteCancelResultFailed = "FAILED"
)

// cancelResults[].errorCode: a Lunixi code, never the provider's; null for
// CANCELLED and ALREADY_CLOSED.
const (
	// PromissoryNoteCancelErrorNotCancellable: the note was collected or cannot
	// be cancelled (NOT_CANCELLABLE).
	PromissoryNoteCancelErrorNotCancellable = "payment.promissory_note.not_cancellable"
	// PromissoryNoteCancelErrorProviderUnavailable is temporary: cancel again
	// later (FAILED).
	PromissoryNoteCancelErrorProviderUnavailable = "payment.promissory_note.provider_unavailable"
	// PromissoryNoteCancelErrorCancelRejected is permanent: the cancellation
	// was refused (FAILED).
	PromissoryNoteCancelErrorCancelRejected = "payment.promissory_note.cancel_rejected"
)

// Language of the printable notes (DownloadPromissoryNoteDocument).
const (
	PromissoryNoteDocumentLocaleTR = "tr-TR"
	PromissoryNoteDocumentLocaleEN = "en-GB"
)

// Webhook events of a note. The payload adds promissoryNote: { serialNumber,
// sequence, amountMinor, dueDate, status, occurredAt }. Statuses are polled, so
// an event arrives some time after the due date.
const (
	PromissoryNoteEventPaid      = "payment.promissory_note.paid"
	PromissoryNoteEventProtested = "payment.promissory_note.protested"
	PromissoryNoteEventCancelled = "payment.promissory_note.cancelled"
)

// PromissoryNoteScheduleRow is one note of a promissory-note schedule. Amount
// is positive minor-unit text; DueDate is a YYYY-MM-DD calendar date.
type PromissoryNoteScheduleRow struct {
	Amount  string `json:"amount"`
	DueDate string `json:"dueDate"`
}

var (
	dueDatePattern      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	noteCountPattern    = regexp.MustCompile(`^[1-9][0-9]?$`)
	serialNumberPattern = regexp.MustCompile(`^[A-Za-z0-9-]{4,64}$`)
)

const promissoryNoteDocumentAccept = "application/pdf, application/json"

func calendarDate(value string) bool {
	if !dueDatePattern.MatchString(value) {
		return false
	}
	_, err := time.Parse(time.DateOnly, value)
	return err == nil
}

// validateScheduleRows checks 1–24 rows with positive minor-unit amounts and
// calendar due dates in non-decreasing order (the same day twice is fine).
func validateScheduleRows(rows []PromissoryNoteScheduleRow, path string) error {
	if len(rows) == 0 {
		return invalid(path + " must be a non-empty list of { amount, dueDate } rows")
	}
	if len(rows) > PromissoryNoteMaxScheduleItems {
		return invalid(path + " can have at most 24 rows")
	}
	previous := ""
	for index, row := range rows {
		rowPath := path + "[" + strconv.Itoa(index) + "]"
		if !positiveMoney(row.Amount) {
			return invalid(rowPath + ".amount must be positive integer minor-unit text")
		}
		if !calendarDate(row.DueDate) {
			return invalid(rowPath + ".dueDate must be a calendar date in YYYY-MM-DD form")
		}
		if previous != "" && row.DueDate < previous {
			return invalid(rowPath + ".dueDate must not be earlier than the previous row; list the notes in due-date order")
		}
		previous = row.DueDate
	}
	return nil
}

func scheduleMatchesAmount(rows []PromissoryNoteScheduleRow, amount, path string) error {
	var total int64
	for _, row := range rows {
		value := moneyInteger(row.Amount)
		if value > math.MaxInt64-total {
			return invalid(path + " total overflow")
		}
		total += value
	}
	if !positiveMoney(amount) || total != moneyInteger(amount) {
		return invalid(path + " must add up to amount")
	}
	return nil
}

// scheduleFromGeneric reads a decoded schedule: a list of objects that carry
// only amount and dueDate as strings.
func scheduleFromGeneric(value any, path string) ([]PromissoryNoteScheduleRow, error) {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil, invalid(path + " must be a non-empty list of { amount, dueDate } rows")
	}
	rows := make([]PromissoryNoteScheduleRow, 0, len(list))
	for index, entry := range list {
		rowPath := path + "[" + strconv.Itoa(index) + "]"
		object, ok := entry.(map[string]any)
		if !ok {
			return nil, invalid(rowPath + " must be an object { amount, dueDate }")
		}
		for _, key := range sortedKeys(object) {
			if key != "amount" && key != "dueDate" {
				return nil, invalid(rowPath + " has unknown field " + key + "; a row is { amount, dueDate }")
			}
		}
		amount, amountOK := object["amount"].(string)
		dueDate, dateOK := object["dueDate"].(string)
		if !amountOK {
			return nil, invalid(rowPath + ".amount must be positive integer minor-unit text")
		}
		if !dateOK {
			return nil, invalid(rowPath + ".dueDate must be a calendar date in YYYY-MM-DD form")
		}
		rows = append(rows, PromissoryNoteScheduleRow{Amount: amount, DueDate: dueDate})
	}
	return rows, validateScheduleRows(rows, path)
}

// noteCountText accepts a note count 1–24 as a digit string or an integer and
// returns the string that is sent.
func noteCountText(value any, path string) (string, error) {
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	case int:
		text = strconv.Itoa(v)
	default:
		return "", invalid(path + " must be a number of notes from 1 to 24")
	}
	if !noteCountPattern.MatchString(text) {
		return "", invalid(path + " must be a number of notes from 1 to 24")
	}
	if n, _ := strconv.Atoi(text); n > PromissoryNoteMaxScheduleItems {
		return "", invalid(path + " must be a number of notes from 1 to 24")
	}
	return text, nil
}

// validatePromissoryNotePlan checks the plan of a promissoryNote block:
// exactly one of schedule or noteCount; a schedule adds up to amount.
func validatePromissoryNotePlan(block map[string]any, amount, path string) error {
	hasSchedule, hasNoteCount := present(block, "schedule"), present(block, "noteCount")
	if hasSchedule && hasNoteCount {
		return invalid(path + ": send either schedule or noteCount, not both")
	}
	if !hasSchedule && !hasNoteCount {
		return invalid(path + " needs the plan: a schedule ([{ amount, dueDate }]) or a noteCount")
	}
	if hasNoteCount {
		_, err := noteCountText(block["noteCount"], path+".noteCount")
		return err
	}
	rows, err := scheduleFromGeneric(block["schedule"], path+".schedule")
	if err != nil {
		return err
	}
	return scheduleMatchesAmount(rows, amount, path+".schedule")
}

func promissoryNotesPath(paymentID string) (string, error) {
	id, err := idPath(paymentID)
	if err != nil {
		return "", invalid("paymentId is required (data.paymentId of the promissory-note payment)")
	}
	return "/api/v1/payments/" + id + "/promissory-notes", nil
}

// GetPromissoryNotes reads the notes of a promissory-note payment
// (GET /payments/{paymentId}/promissory-notes): documentType, rowSerialNumber,
// totalAmount and per note serialNumber, sequence, amount (minor units),
// dueDate and status (PromissoryNoteStatus*). A payment without notes is a 404
// payment.promissory_note.not_found.
func (p *PaymentClient) GetPromissoryNotes(ctx context.Context, paymentID string) (*Response, error) {
	path, err := promissoryNotesPath(paymentID)
	if err != nil {
		return nil, err
	}
	return p.client.Do(ctx, Request{Method: "GET", Path: path})
}

// CancelPromissoryNotes cancels notes (POST /payments/{paymentId}/promissory-notes/cancel).
// A nil serialNumbers cancels every open note and sends no body; otherwise
// 1–24 serial numbers from GetPromissoryNotes. The answer adds cancelResults[]
// per note (PromissoryNoteCancelResult*, errorCode PromissoryNoteCancelError*).
// The payment stays CAPTURED. key is optional; when given it makes retries safe.
func (p *PaymentClient) CancelPromissoryNotes(ctx context.Context, paymentID string, serialNumbers []string, key string) (*Response, error) {
	path, err := promissoryNotesPath(paymentID)
	if err != nil {
		return nil, err
	}
	var body any
	if serialNumbers != nil {
		if len(serialNumbers) == 0 || len(serialNumbers) > PromissoryNoteMaxScheduleItems {
			return nil, invalid("serialNumbers must list 1 to 24 note serial numbers; pass nil to cancel every open note")
		}
		for _, serial := range serialNumbers {
			if !serialNumberPattern.MatchString(serial) {
				return nil, invalid("serialNumbers contains an invalid serial number (letters, digits and -, 4 to 64 characters)")
			}
		}
		body = map[string]any{"serialNumbers": slices.Clone(serialNumbers)}
	}
	return p.client.Do(ctx, Request{Method: "POST", Path: path + "/cancel", Body: body, IdempotencyKey: key})
}

// DownloadPromissoryNoteDocument returns the printable notes as a PDF, one
// note per page (GET /payments/{paymentId}/promissory-notes/document). The PDF
// is in Response.Bytes with Response.ContentType; an error answer is read as
// the JSON envelope and returned as APIError (503
// payment.promissory_note.document_unavailable: try again later). locale is
// "" (gateway default) or a PromissoryNoteDocumentLocale* value. The document
// is not stored; download it when you need it.
func (p *PaymentClient) DownloadPromissoryNoteDocument(ctx context.Context, paymentID, locale string) (*Response, error) {
	path, err := promissoryNotesPath(paymentID)
	if err != nil {
		return nil, err
	}
	var query url.Values
	if locale != "" {
		if locale != PromissoryNoteDocumentLocaleTR && locale != PromissoryNoteDocumentLocaleEN {
			return nil, invalid("locale must be tr-TR or en-GB")
		}
		query = url.Values{"locale": {locale}}
	}
	return p.client.do(ctx, Request{Method: "GET", Path: path + "/document", Query: query}, promissoryNoteDocumentAccept)
}

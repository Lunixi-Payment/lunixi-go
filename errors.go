package lunixi

import (
	"errors"
	"fmt"
)

var (
	// ErrConfiguration indicates invalid configuration or a rejected request.
	ErrConfiguration = errors.New("lunixi: invalid configuration or request")
	// ErrWebhook indicates an invalid, stale, or mismatched webhook.
	ErrWebhook = errors.New("lunixi: webhook verification failed")
)

// APIError exposes safe diagnostics without including a request body, access
// token, private key, customer information, or an upstream error message.
type APIError struct {
	StatusCode int
	Code       string
	RequestID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("lunixi: API request failed (HTTP %d, code %s)", e.StatusCode, e.Code)
}

func invalid(field string) error { return fmt.Errorf("%w: %s", ErrConfiguration, field) }

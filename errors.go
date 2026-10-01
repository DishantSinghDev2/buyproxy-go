package buyproxy

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for use with errors.Is:
//
//	if errors.Is(err, buyproxy.ErrInsufficientBandwidth) { ... }
var (
	ErrValidation            = errors.New("buyproxy: invalid request")
	ErrAuthentication        = errors.New("buyproxy: authentication failed")
	ErrInsufficientBandwidth = errors.New("buyproxy: insufficient bandwidth")
	ErrForbidden             = errors.New("buyproxy: forbidden")
	ErrNotFound              = errors.New("buyproxy: not found")
	ErrRateLimited           = errors.New("buyproxy: rate limited")
	ErrServer                = errors.New("buyproxy: server error")
)

// APIError is returned for every non-2xx response.
type APIError struct {
	// HTTP status code.
	Status int
	// Message is the API's own error text.
	Message string
	// Body is the raw response body.
	Body []byte
	// RetryAfter is set on 429 when the API said how long to wait.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("buyproxy: %s (HTTP %d)", e.Message, e.Status)
}

// Is makes errors.Is(err, ErrAuthentication) and friends work.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrValidation:
		return e.Status == 400
	case ErrAuthentication:
		return e.Status == 401
	case ErrInsufficientBandwidth:
		return e.Status == 402
	case ErrForbidden:
		return e.Status == 403
	case ErrNotFound:
		return e.Status == 404
	case ErrRateLimited:
		return e.Status == 429
	case ErrServer:
		return e.Status >= 500
	}
	return false
}

// ConnectionError means the request never got a response (DNS, reset, timeout, offline).
type ConnectionError struct{ Err error }

func (e *ConnectionError) Error() string {
	return "buyproxy: could not reach the API: " + e.Err.Error()
}
func (e *ConnectionError) Unwrap() error { return e.Err }

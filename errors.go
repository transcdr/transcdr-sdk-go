package transcdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Error types: the API's `error.type`, which follows the HTTP status class.
const (
	// ErrorTypeInvalidRequest is 400, 404, 409 and 422: the request was wrong.
	ErrorTypeInvalidRequest = "invalid_request_error"
	// ErrorTypeAuthentication is 401: a missing, invalid, expired or revoked token.
	ErrorTypeAuthentication = "authentication_error"
	// ErrorTypePermission is 403: the key lacks a scope, or the user a role.
	ErrorTypePermission = "permission_error"
	// ErrorTypeRateLimit is 429: slow down (see [Error.RetryAfter]).
	ErrorTypeRateLimit = "rate_limit_error"
	// ErrorTypeQuota is 402: not enough credit (insufficient_credit), over a
	// job's max_cost_cents (cost_limit_exceeded), or the monthly limit reached
	// (spend_limit_reached).
	ErrorTypeQuota = "quota_error"
	// ErrorTypeAPI is 5xx: something went wrong on Transcdr's side.
	ErrorTypeAPI = "api_error"
	// ErrorTypeConnection means no response arrived (see [ConnectionError]).
	ErrorTypeConnection = "connection_error"
)

// Error is an error response from the API.
type Error struct {
	// Type is the error class, e.g. [ErrorTypeInvalidRequest].
	Type string `json:"type"`
	// Status is the HTTP status; 0 for a request the SDK refused before
	// sending it.
	Status int `json:"-"`
	// Code is machine-readable, e.g. "validation_failed" or "insufficient_scope".
	Code string `json:"code"`
	// Message is human-readable.
	Message string `json:"message"`
	// Param is the offending parameter in dotted form, e.g. "output.renditions.0.width".
	Param string `json:"param"`
	// Details holds per-field validation messages.
	Details map[string][]string `json:"details"`
	// Errors lists every problem with an output spec that was refused
	// (validation_failed): missing fields first, in document order. Param and
	// Message are the first. The SDK returns the same, with Status 0, for a
	// whole spec it refuses before sending (see [ValidateOutput]).
	Errors []FieldError `json:"errors"`
	// RequestID identifies the request to support (X-Request-Id).
	RequestID string `json:"request_id"`
	// Header is the response's headers.
	Header http.Header `json:"-"`
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("transcdr:")
	if e.Status != 0 {
		fmt.Fprintf(&b, " %d", e.Status)
	}
	if e.Code != "" {
		fmt.Fprintf(&b, " %s", e.Code)
	} else if e.Type != "" {
		fmt.Fprintf(&b, " %s", e.Type)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	if len(e.Details) > 0 {
		keys := make([]string, 0, len(e.Details))
		for k := range e.Details {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			for _, m := range e.Details[k] {
				parts = append(parts, k+": "+m)
			}
		}
		fmt.Fprintf(&b, " (%s)", strings.Join(parts, "; "))
	}
	if len(e.Errors) > 1 {
		parts := make([]string, 0, len(e.Errors)-1)
		for _, fe := range e.Errors[1:] {
			parts = append(parts, fe.Message)
		}
		fmt.Fprintf(&b, " (and: %s)", strings.Join(parts, " "))
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request %s]", e.RequestID)
	}
	return b.String()
}

// RetryAfter is how long a 429 asks the caller to wait, from Retry-After.
func (e *Error) RetryAfter() (time.Duration, bool) {
	if e.Header == nil {
		return 0, false
	}
	v := e.Header.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if s, err := strconv.ParseFloat(v, 64); err == nil && s >= 0 {
		return time.Duration(s * float64(time.Second)), true
	}
	if at, err := http.ParseTime(v); err == nil {
		return max(0, time.Until(at)), true
	}
	return 0, false
}

// ConnectionError means no response arrived: DNS, TLS, a reset connection or
// a timeout.
type ConnectionError struct {
	Message string
	// Timeout is true when the per-attempt timeout expired.
	Timeout bool
	Err     error
}

func (e *ConnectionError) Error() string { return "transcdr: " + e.Message }
func (e *ConnectionError) Unwrap() error { return e.Err }

// WaitTimeoutError is returned by [JobsService.WaitFor] when the job had not
// finished before the timeout.
type WaitTimeoutError struct {
	JobID string
	// Job is the last state seen.
	Job     *Job
	Timeout time.Duration
}

func (e *WaitTimeoutError) Error() string {
	status := "unfinished"
	if e.Job != nil {
		status = string(e.Job.Status)
	}
	return fmt.Sprintf("transcdr: job %s was still %s after %s", e.JobID, status, e.Timeout)
}

// ErrInvalidSignature is returned by [ConstructEvent] when a webhook's
// signature does not verify.
var ErrInvalidSignature = errors.New("transcdr: webhook signature verification failed")

func defaultErrorType(status int) string {
	switch {
	case status == 401:
		return ErrorTypeAuthentication
	case status == 402:
		return ErrorTypeQuota
	case status == 403:
		return ErrorTypePermission
	case status == 429:
		return ErrorTypeRateLimit
	case status >= 500:
		return ErrorTypeAPI
	default:
		return ErrorTypeInvalidRequest
	}
}

// errorFromResponse builds an *Error from a non-2xx response body.
func errorFromResponse(status int, body []byte, header http.Header) *Error {
	var envelope struct {
		Error *Error `json:"error"`
	}
	e := &Error{}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		e = envelope.Error
	} else if text := strings.TrimSpace(string(body)); text != "" {
		if len(text) > 500 {
			text = text[:500]
		}
		e.Message = text
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("Request failed with status %d", status)
	}
	e.Status = status
	e.Header = header
	if e.Type == "" {
		e.Type = defaultErrorType(status)
	}
	if e.RequestID == "" && header != nil {
		e.RequestID = header.Get("X-Request-Id")
	}
	return e
}

// AsError returns the API error in err's chain, if any.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

func hasStatus(err error, status int) bool {
	e, ok := AsError(err)
	return ok && e.Status == status
}

func hasType(err error, typ string) bool {
	e, ok := AsError(err)
	return ok && e.Type == typ
}

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsConflict reports a 409, e.g. connection_in_use or connection_disabled.
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

// IsInvalidRequest reports an invalid_request_error (400, 404, 409, 422).
func IsInvalidRequest(err error) bool { return hasType(err, ErrorTypeInvalidRequest) }

// IsAuthentication reports a 401.
func IsAuthentication(err error) bool { return hasType(err, ErrorTypeAuthentication) }

// IsPermission reports a 403.
func IsPermission(err error) bool { return hasType(err, ErrorTypePermission) }

// IsRateLimit reports a 429.
func IsRateLimit(err error) bool { return hasType(err, ErrorTypeRateLimit) }

// IsQuota reports a 402.
func IsQuota(err error) bool { return hasType(err, ErrorTypeQuota) }

// IsAPIError reports a 5xx.
func IsAPIError(err error) bool { return hasType(err, ErrorTypeAPI) }

// IsConnection reports that no response arrived.
func IsConnection(err error) bool {
	var e *ConnectionError
	return errors.As(err, &e)
}

// IsTimeout reports a per-attempt timeout.
func IsTimeout(err error) bool {
	var e *ConnectionError
	return errors.As(err, &e) && e.Timeout
}

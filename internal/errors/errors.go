package errors

import (
	"errors"
	"net"
	"net/url"
)

// RetryableError wraps an error to indicate it should be retried
type RetryableError struct {
	err error
}

// Error implements the error interface
func (e *RetryableError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error
func (e *RetryableError) Unwrap() error {
	return e.err
}

// NewRetryableError creates a new RetryableError
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{err: err}
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's already a RetryableError
	if _, ok := err.(*RetryableError); ok {
		return true
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Temporary() {
		return true
	}

	// Check for URL errors
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}

	return false
}

// New creates a new error
func New(text string) error {
	return errors.New(text)
}

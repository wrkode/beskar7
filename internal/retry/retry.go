package retry

import (
	"context"
	"math/rand"
	"time"
)

// Config holds the retry configuration
type Config struct {
	// InitialInterval is the initial retry interval
	InitialInterval time.Duration
	// MaxInterval is the maximum retry interval
	MaxInterval time.Duration
	// Multiplier is the factor to multiply the interval by for each retry
	Multiplier float64
	// MaxAttempts is the maximum number of retry attempts (0 means unlimited)
	MaxAttempts int
	// MaxElapsedTime is the maximum total time to retry (0 means unlimited)
	MaxElapsedTime time.Duration
}

// DefaultConfig returns a default retry configuration
func DefaultConfig() *Config {
	return &Config{
		InitialInterval: 1 * time.Second,
		MaxInterval:     5 * time.Minute,
		Multiplier:      2.0,
		MaxAttempts:     5,
		MaxElapsedTime:  15 * time.Minute,
	}
}

// Operation is a function that will be retried
type Operation func() error

// WithContext executes the operation with context support
func WithContext(ctx context.Context, config *Config, operation Operation) error {
	if config == nil {
		config = DefaultConfig()
	}

	var err error
	attempt := 0
	startTime := time.Now()
	interval := config.InitialInterval

	for {
		// Check if we've exceeded max attempts
		if config.MaxAttempts > 0 && attempt >= config.MaxAttempts {
			return err
		}

		// Check if we've exceeded max elapsed time
		if config.MaxElapsedTime > 0 && time.Since(startTime) > config.MaxElapsedTime {
			return err
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the operation
		err = operation()
		if err == nil {
			return nil
		}

		// Increment attempt counter
		attempt++

		// Calculate next interval with exponential backoff
		interval = time.Duration(float64(interval) * config.Multiplier)
		if interval > config.MaxInterval {
			interval = config.MaxInterval
		}

		// Add jitter to prevent thundering herd
		jitter := time.Duration(rand.Float64() * float64(interval) * 0.1)
		interval = interval + jitter

		// Wait for the next retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	// Add logic to determine if an error is retryable
	// For example, network errors, temporary API failures, etc.
	return true
}

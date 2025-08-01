package letterboxd

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrorType represents the type of error that occurred
type ErrorType string

const (
	// ErrorTypeAuth represents authentication errors
	ErrorTypeAuth ErrorType = "auth"
	// ErrorTypeNetwork represents network-related errors
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeUI represents UI/page element errors
	ErrorTypeUI ErrorType = "ui"
	// ErrorTypeTimeout represents timeout errors
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeUnknown represents unknown errors
	ErrorTypeUnknown ErrorType = "unknown"
)

// LetterboxdError represents an error that occurred during Letterboxd operations
type LetterboxdError struct {
	Type          ErrorType
	OriginalError error
	Context       map[string]interface{}
	Retryable     bool
}

// Error implements the error interface
func (e *LetterboxdError) Error() string {
	return fmt.Sprintf("%s error: %v", e.Type, e.OriginalError)
}

// Unwrap returns the original error
func (e *LetterboxdError) Unwrap() error {
	return e.OriginalError
}

// IsAuthError returns true if the error is an authentication error
func IsAuthError(err error) bool {
	var lbErr *LetterboxdError
	return errors.As(err, &lbErr) && lbErr.Type == ErrorTypeAuth
}

// IsNetworkError returns true if the error is a network error
func IsNetworkError(err error) bool {
	var lbErr *LetterboxdError
	return errors.As(err, &lbErr) && lbErr.Type == ErrorTypeNetwork
}

// IsRetryable returns true if the error is retryable
func IsRetryable(err error) bool {
	var lbErr *LetterboxdError
	return errors.As(err, &lbErr) && lbErr.Retryable
}

// RetryConfig holds configuration for the retry mechanism
type RetryConfig struct {
	MaxAttempts      int
	InitialDelay     time.Duration
	MaxDelay         time.Duration
	BackoffFactor    float64
	RetryableErrors  []ErrorType
	NonRetryableErrs []ErrorType
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  500 * time.Millisecond,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
		RetryableErrors: []ErrorType{
			ErrorTypeNetwork,
			ErrorTypeTimeout,
		},
		NonRetryableErrs: []ErrorType{
			ErrorTypeAuth,
		},
	}
}

// WithRetry executes the provided function with retry logic
func WithRetry(op string, fn func() error, config RetryConfig) error {
	var err error
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Execute the operation
		err = fn()
		if err == nil {
			return nil // Success
		}

		// Check if the error is retryable
		var retryable bool
		var lbErr *LetterboxdError
		if errors.As(err, &lbErr) {
			// Check against explicit lists
			retryable = lbErr.Retryable

			// Override based on error type lists if specified
			for _, t := range config.RetryableErrors {
				if lbErr.Type == t {
					retryable = true
					break
				}
			}
			
			for _, t := range config.NonRetryableErrs {
				if lbErr.Type == t {
					retryable = false
					break
				}
			}
		} else {
			// Unknown error type, assume not retryable
			retryable = false
		}

		// If not retryable or this was the last attempt, return the error
		if !retryable || attempt == config.MaxAttempts {
			slog.Error(fmt.Sprintf("Operation %s failed after %d attempts", op, attempt),
				slog.String("error", err.Error()),
				slog.Int("attempts", attempt))
			return err
		}

		// Log retry attempt
		slog.Warn(fmt.Sprintf("Operation %s failed, retrying (%d/%d)", op, attempt, config.MaxAttempts),
			slog.String("error", err.Error()),
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", config.MaxAttempts),
			slog.Duration("delay", delay))

		// Wait before retrying
		time.Sleep(delay)

		// Increase delay with exponential backoff
		delay = time.Duration(float64(delay) * config.BackoffFactor)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	return err // Should never get here
}
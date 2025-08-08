package main

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"
)

// Operation represents a function that might fail and need to be retried
type Operation func() error

// ErrorClassifier is a function that determines if an error should trigger a retry
type ErrorClassifier func(error) bool

// IsTimeoutError checks if an error is a timeout
func IsTimeoutError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "deadline") ||
		strings.Contains(err.Error(), "timed out"))
}

// IsNetworkError checks if an error is a likely network error
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errorMessage := err.Error()
	return strings.Contains(errorMessage, "network") ||
		strings.Contains(errorMessage, "connection") ||
		strings.Contains(errorMessage, "broken pipe") ||
		strings.Contains(errorMessage, "reset") ||
		strings.Contains(errorMessage, "EOF") ||
		strings.Contains(errorMessage, "closed")
}

// IsRetryableError checks if an error should trigger a retry
func IsRetryableError(err error) bool {
	return IsTimeoutError(err) || IsNetworkError(err)
}

// RetryWithBackoff retries an operation with exponential backoff
func RetryWithBackoff(
	ctx context.Context,
	maxRetries int,
	initialBackoff time.Duration,
	maxBackoff time.Duration,
	operation Operation,
	errorClassifier ErrorClassifier,
	logger Logger,
) error {
	var err error
	backoff := initialBackoff

	// Initialize random with current time
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// If this is not the first attempt, log the retry
		if attempt > 0 {
			logger.Info("Retrying operation (attempt %d/%d) after %v delay",
				attempt, maxRetries, backoff)
		}

		// Attempt the operation
		err = operation()

		// If no error or the error is not classified as retryable, return
		if err == nil || !errorClassifier(err) {
			return err
		}

		// If this was the last attempt, return the error
		if attempt == maxRetries {
			logger.Error("Operation failed after %d attempts: %v", maxRetries+1, err)
			return err
		}

		// Calculate next backoff with jitter (randomness)
		jitter := 0.1 * float64(backoff)
		randomJitter := time.Duration(r.Float64() * jitter)
		nextBackoff := backoff + randomJitter

		// Cap backoff at maximum
		if nextBackoff > maxBackoff {
			nextBackoff = maxBackoff
		}

		// Wait for backoff period or until context is cancelled
		select {
		case <-ctx.Done():
			return errors.New("operation cancelled during backoff")
		case <-time.After(nextBackoff):
			// Continue to next attempt
		}

		// Increase backoff for next attempt (exponential)
		backoff = time.Duration(float64(backoff) * 2.0)
	}

	// This should never happen, but to be safe
	return err
}

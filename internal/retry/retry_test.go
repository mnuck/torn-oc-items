package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithRetrySuccess(t *testing.T) {
	config := Config{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Timeout:    1 * time.Second,
	}

	callCount := 0
	operation := func(ctx context.Context) (string, error) {
		callCount++
		return "success", nil
	}

	result, err := WithRetry(context.Background(), config, operation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %s", result)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestWithRetrySuccessAfterRetries(t *testing.T) {
	config := Config{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Timeout:    1 * time.Second,
	}

	callCount := 0
	operation := func(ctx context.Context) (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("temporary failure")
		}
		return "success", nil
	}

	result, err := WithRetry(context.Background(), config, operation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %s", result)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestWithRetryFailureAfterMaxRetries(t *testing.T) {
	config := Config{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Timeout:    1 * time.Second,
	}

	callCount := 0
	operation := func(ctx context.Context) (string, error) {
		callCount++
		return "", errors.New("persistent failure")
	}

	result, err := WithRetry(context.Background(), config, operation)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if result != "" {
		t.Errorf("Expected empty result, got %s", result)
	}
	if callCount != 3 { // MaxRetries + 1
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestWithRetryContextCancellation(t *testing.T) {
	config := Config{
		MaxRetries: 5,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   200 * time.Millisecond,
		Timeout:    1 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	operation := func(ctx context.Context) (string, error) {
		callCount++
		if callCount == 2 {
			cancel() // Cancel after second attempt
		}
		return "", errors.New("failure")
	}

	result, err := WithRetry(ctx, config, operation)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty result, got %s", result)
	}
	if callCount > 3 {
		t.Errorf("Expected at most 3 calls due to cancellation, got %d", callCount)
	}
}

func TestWithRetryContextTimeout(t *testing.T) {
	config := Config{
		MaxRetries: 10,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   200 * time.Millisecond,
		Timeout:    1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	callCount := 0
	operation := func(ctx context.Context) (string, error) {
		callCount++
		return "", errors.New("failure")
	}

	start := time.Now()
	result, err := WithRetry(ctx, config, operation)
	duration := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty result, got %s", result)
	}
	if duration > 150*time.Millisecond {
		t.Errorf("Expected operation to timeout around 100ms, took %v", duration)
	}
}

func TestCalculateBackoffDelay(t *testing.T) {
	baseDelay := 10 * time.Millisecond
	maxDelay := 100 * time.Millisecond

	tests := []struct {
		attempt int
		minDelay time.Duration
		maxExpected time.Duration
	}{
		{0, 5 * time.Millisecond, 15 * time.Millisecond},    // 2^0 * 10ms = 10ms, with jitter 5-15ms
		{1, 10 * time.Millisecond, 30 * time.Millisecond},   // 2^1 * 10ms = 20ms, with jitter 10-30ms
		{2, 20 * time.Millisecond, 60 * time.Millisecond},   // 2^2 * 10ms = 40ms, with jitter 20-60ms
		{3, 40 * time.Millisecond, 100 * time.Millisecond},  // 2^3 * 10ms = 80ms, with jitter 40-120ms, capped at 100ms
		{4, 50 * time.Millisecond, 100 * time.Millisecond},  // 2^4 * 10ms = 160ms, capped at 100ms with jitter
		{5, 50 * time.Millisecond, 100 * time.Millisecond},  // Capped at maxDelay
		{35, 50 * time.Millisecond, 100 * time.Millisecond}, // Large attempt should not overflow, capped at maxDelay
		{100, 50 * time.Millisecond, 100 * time.Millisecond}, // Very large attempt should not overflow
	}

	for _, test := range tests {
		// Test multiple times due to randomness
		for i := 0; i < 10; i++ {
			result := calculateBackoffDelay(test.attempt, baseDelay, maxDelay)
			if result < test.minDelay || result > test.maxExpected {
				t.Errorf("calculateBackoffDelay(%d, %v, %v) = %v, expected between %v and %v",
					test.attempt, baseDelay, maxDelay, result, test.minDelay, test.maxExpected)
			}
		}
	}
}

func TestWithRetryDifferentTypes(t *testing.T) {
	config := Config{
		MaxRetries: 1,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Timeout:    1 * time.Second,
	}

	// Test with int
	intResult, err := WithRetry(context.Background(), config, func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("Expected no error for int, got %v", err)
	}
	if intResult != 42 {
		t.Errorf("Expected 42, got %d", intResult)
	}

	// Test with struct
	type TestStruct struct {
		Value string
	}
	structResult, err := WithRetry(context.Background(), config, func(ctx context.Context) (TestStruct, error) {
		return TestStruct{Value: "test"}, nil
	})
	if err != nil {
		t.Errorf("Expected no error for struct, got %v", err)
	}
	if structResult.Value != "test" {
		t.Errorf("Expected 'test', got %s", structResult.Value)
	}
}

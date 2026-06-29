package notifications

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	topic      string
	enabled    bool
	batchMode  bool
	priority   string
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	// Circuit breaker state
	failures    int
	lastFailure time.Time
	circuitOpen bool
	mutex       sync.RWMutex
	// Metrics
	totalSent    int64
	totalFailed  int64
	totalRetries int64
}

type ItemInfo struct {
	ItemName string
	UserName string
	CrimeURL string
}

type NotificationError struct {
	Type       string
	StatusCode int
	Attempt    int
	Underlying error
}

func (e *NotificationError) Error() string {
	return fmt.Sprintf("notification failed [%s] attempt %d: %v", e.Type, e.Attempt, e.Underlying)
}

func (e *NotificationError) IsRetryable() bool {
	switch e.Type {
	case "network", "server", "timeout", "rate_limit":
		return true
	case "auth", "client":
		return false
	default:
		return e.StatusCode >= 500
	}
}

func NewClient(baseURL, topic string, enabled, batchMode bool, priority string, maxRetries int, baseDelay, maxDelay time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
		topic:      topic,
		enabled:    enabled,
		batchMode:  batchMode,
		priority:   priority,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		maxDelay:   maxDelay,
	}
}

func (c *Client) SendNotification(ctx context.Context, message string) error {
	if !c.enabled {
		slog.Debug("Notifications disabled, skipping")
		return nil
	}

	if c.isCircuitOpen() {
		slog.Warn("Circuit breaker open, skipping notification")
		return &NotificationError{
			Type:       "circuit_open",
			Underlying: fmt.Errorf("circuit breaker is open"),
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateBackoff(attempt)
			slog.Debug("Retrying notification after delay", "attempt", attempt, "delay", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			c.incrementRetries()
		}

		err := c.sendSingleNotification(ctx, message, attempt+1)
		if err == nil {
			c.recordSuccess()
			return nil
		}

		lastErr = err

		if notifErr, ok := err.(*NotificationError); ok {
			if !notifErr.IsRetryable() {
				slog.Warn("Non-retryable error, giving up", "error", err, "attempt", attempt+1)
				c.recordFailure()
				return err
			}
		}

		slog.Warn("Notification attempt failed", "error", err, "attempt", attempt+1, "max_retries", c.maxRetries)
	}

	c.recordFailure()
	return &NotificationError{
		Type:       "max_retries_exceeded",
		Attempt:    c.maxRetries + 1,
		Underlying: lastErr,
	}
}

func (c *Client) sendSingleNotification(ctx context.Context, message string, attempt int) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, c.topic)
	slog.Debug("Sending notification", "url", url, "attempt", attempt)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(message))
	if err != nil {
		return &NotificationError{Type: "client", Attempt: attempt, Underlying: err}
	}

	req.Header.Set("Content-Type", "text/plain")
	if c.priority != "" {
		req.Header.Set("Priority", c.priority)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NotificationError{Type: "network", Attempt: attempt, Underlying: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return &NotificationError{
			Type:       c.categorizeHTTPError(resp.StatusCode),
			StatusCode: resp.StatusCode,
			Attempt:    attempt,
			Underlying: fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status),
		}
	}

	slog.Debug("Notification sent successfully", "status_code", resp.StatusCode, "attempt", attempt)
	return nil
}

func (c *Client) SendNotificationAsync(ctx context.Context, message string) {
	go func() {
		if err := c.SendNotification(ctx, message); err != nil {
			slog.Warn("Async notification failed", "error", err)
		}
	}()
}

func (c *Client) NotifyNewItems(ctx context.Context, items []ItemInfo, totalAdded int) {
	if !c.enabled || totalAdded == 0 {
		return
	}
	if c.batchMode {
		c.sendBatchNotification(ctx, items, totalAdded)
	} else {
		c.sendIndividualNotifications(ctx, items)
	}
}

func (c *Client) NotifyStateTransition(ctx context.Context, crimeID int, crimeName, fromState, toState string) {
	slog.Warn("Crime state transition detected",
		"crime_id", crimeID,
		"crime_name", crimeName,
		"from_state", fromState,
		"to_state", toState,
	)

	if !c.enabled {
		return
	}

	message := fmt.Sprintf("🔄 Crime State Transition\nCrime %d (%s) changed from %s to %s",
		crimeID, crimeName, fromState, toState)
	c.SendNotificationAsync(ctx, message)
}

func (c *Client) sendBatchNotification(ctx context.Context, items []ItemInfo, totalAdded int) {
	slog.Info("Sending batch notification for new items", "items_added", totalAdded)
	c.SendNotificationAsync(ctx, c.formatBatchMessage(items, totalAdded))
}

func (c *Client) sendIndividualNotifications(ctx context.Context, items []ItemInfo) {
	slog.Info("Sending individual notifications for new items", "items_added", len(items))
	for i, item := range items {
		c.SendNotificationAsync(ctx, c.formatIndividualMessage(item, i+1, len(items)))
		if i < len(items)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (c *Client) formatBatchMessage(items []ItemInfo, totalAdded int) string {
	var sb strings.Builder
	if totalAdded == 1 {
		sb.WriteString("🎯 Torn OC: 1 new item needed\n")
	} else {
		fmt.Fprintf(&sb, "🎯 Torn OC: %d new items needed\n", totalAdded)
	}
	maxShow := 10
	if len(items) < maxShow {
		maxShow = len(items)
	}
	for i := 0; i < maxShow; i++ {
		fmt.Fprintf(&sb, "• %s for %s\n", items[i].ItemName, items[i].UserName)
	}
	if len(items) > 10 {
		fmt.Fprintf(&sb, "... and %d more items\n", len(items)-10)
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func (c *Client) formatIndividualMessage(item ItemInfo, itemNum, totalItems int) string {
	var sb strings.Builder
	if totalItems > 1 {
		fmt.Fprintf(&sb, "📋 New item needed (%d/%d)\n", itemNum, totalItems)
	} else {
		sb.WriteString("📋 New item needed\n")
	}
	fmt.Fprintf(&sb, "🎯 **%s**\n", item.ItemName)
	fmt.Fprintf(&sb, "👤 For: %s\n", item.UserName)
	if item.CrimeURL != "" {
		fmt.Fprintf(&sb, "🔗 Crime: %s\n", item.CrimeURL)
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func (c *Client) isCircuitOpen() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if !c.circuitOpen {
		return false
	}

	if time.Since(c.lastFailure) > 30*time.Second {
		c.mutex.RUnlock()
		c.mutex.Lock()
		c.circuitOpen = false
		c.failures = 0
		c.mutex.Unlock()
		c.mutex.RLock()
		slog.Info("Circuit breaker moving to half-open state")
	}

	return c.circuitOpen
}

func (c *Client) recordSuccess() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.totalSent++
	if c.circuitOpen {
		c.circuitOpen = false
		c.failures = 0
		slog.Info("Circuit breaker closed after successful notification")
	}
}

func (c *Client) recordFailure() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.totalFailed++
	c.failures++
	c.lastFailure = time.Now()
	if c.failures >= 5 && !c.circuitOpen {
		c.circuitOpen = true
		slog.Warn("Circuit breaker opened due to consecutive failures", "failures", c.failures)
	}
}

func (c *Client) incrementRetries() {
	c.mutex.Lock()
	c.totalRetries++
	c.mutex.Unlock()
}

func (c *Client) calculateBackoff(attempt int) time.Duration {
	base := float64(c.baseDelay)
	backoff := base * math.Pow(2, float64(attempt-1))
	jitter := rand.Float64()*0.5 - 0.25
	backoff = backoff * (1 + jitter)
	if backoff > float64(c.maxDelay) {
		backoff = float64(c.maxDelay)
	}
	return time.Duration(backoff)
}

func (c *Client) categorizeHTTPError(statusCode int) string {
	switch {
	case statusCode == 401 || statusCode == 403:
		return "auth"
	case statusCode == 429:
		return "rate_limit"
	case statusCode >= 400 && statusCode < 500:
		return "client"
	case statusCode >= 500:
		return "server"
	default:
		return "unknown"
	}
}

func (c *Client) GetMetrics() (sent, failed, retries int64) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.totalSent, c.totalFailed, c.totalRetries
}

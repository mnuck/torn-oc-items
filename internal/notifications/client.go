package notifications

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Client struct {
	httpClient   *http.Client
	baseURL      string
	topic        string
	enabled      bool
	batchMode    bool
	priority     string
	maxRetries   int
	baseDelay    time.Duration
	maxDelay     time.Duration
	// Circuit breaker state
	failures     int
	lastFailure  time.Time
	circuitOpen  bool
	mutex        sync.RWMutex
	// Metrics
	totalSent     int64
	totalFailed   int64
	totalRetries  int64
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
	case "network", "server", "timeout":
		return true
	case "rate_limit":
		return true
	case "auth", "client":
		return false
	default:
		return e.StatusCode >= 500
	}
}

func NewClient(baseURL, topic string, enabled, batchMode bool, priority string, maxRetries int, baseDelay, maxDelay time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:   baseURL,
		topic:     topic,
		enabled:   enabled,
		batchMode: batchMode,
		priority:  priority,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		maxDelay:   maxDelay,
	}
}

func (c *Client) SendNotification(ctx context.Context, message string) error {
	if !c.enabled {
		log.Debug().Msg("Notifications disabled, skipping")
		return nil
	}

	// Check circuit breaker
	if c.isCircuitOpen() {
		log.Warn().Msg("Circuit breaker open, skipping notification")
		return &NotificationError{
			Type:       "circuit_open",
			StatusCode: 0,
			Attempt:    0,
			Underlying: fmt.Errorf("circuit breaker is open"),
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateBackoff(attempt)
			log.Debug().
				Int("attempt", attempt).
				Dur("delay", delay).
				Msg("Retrying notification after delay")
			
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
		
		// Check if error is retryable
		if notifErr, ok := err.(*NotificationError); ok {
			if !notifErr.IsRetryable() {
				log.Warn().
					Err(err).
					Int("attempt", attempt+1).
					Msg("Non-retryable error, giving up")
				c.recordFailure()
				return err
			}
		}

		log.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", c.maxRetries).
			Msg("Notification attempt failed")
	}

	c.recordFailure()
	return &NotificationError{
		Type:       "max_retries_exceeded",
		StatusCode: 0,
		Attempt:    c.maxRetries + 1,
		Underlying: lastErr,
	}
}

func (c *Client) sendSingleNotification(ctx context.Context, message string, attempt int) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, c.topic)

	log.Debug().
		Str("url", url).
		Str("message", message).
		Int("attempt", attempt).
		Msg("Sending notification")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(message))
	if err != nil {
		return &NotificationError{
			Type:       "client",
			StatusCode: 0,
			Attempt:    attempt,
			Underlying: err,
		}
	}

	req.Header.Set("Content-Type", "text/plain")
	if c.priority != "" {
		req.Header.Set("Priority", c.priority)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NotificationError{
			Type:       "network",
			StatusCode: 0,
			Attempt:    attempt,
			Underlying: err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errType := c.categorizeHTTPError(resp.StatusCode)
		return &NotificationError{
			Type:       errType,
			StatusCode: resp.StatusCode,
			Attempt:    attempt,
			Underlying: fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status),
		}
	}

	log.Debug().
		Int("status_code", resp.StatusCode).
		Int("attempt", attempt).
		Msg("Notification sent successfully")

	return nil
}

func (c *Client) SendNotificationAsync(ctx context.Context, message string) {
	go func() {
		if err := c.SendNotification(ctx, message); err != nil {
			log.Warn().Err(err).Msg("Async notification failed")
		}
	}()
}

func (c *Client) NotifyNewItems(ctx context.Context, items []ItemInfo, totalAdded int) {
	if !c.enabled {
		return
	}

	if totalAdded == 0 {
		log.Debug().Msg("No new items to notify about")
		return
	}

	if c.batchMode {
		c.sendBatchNotification(ctx, items, totalAdded)
	} else {
		c.sendIndividualNotifications(ctx, items)
	}
}

func (c *Client) sendBatchNotification(ctx context.Context, items []ItemInfo, totalAdded int) {
	message := c.formatBatchMessage(items, totalAdded)

	log.Info().
		Int("items_added", totalAdded).
		Msg("Sending batch notification for new items")

	c.SendNotificationAsync(ctx, message)
}

func (c *Client) sendIndividualNotifications(ctx context.Context, items []ItemInfo) {
	log.Info().
		Int("items_added", len(items)).
		Msg("Sending individual notifications for new items")

	for i, item := range items {
		message := c.formatIndividualMessage(item, i+1, len(items))
		c.SendNotificationAsync(ctx, message)
		
		// Small delay between individual notifications to avoid overwhelming
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
		sb.WriteString(fmt.Sprintf("🎯 Torn OC: %d new items needed\n", totalAdded))
	}

	maxItemsToShow := 10
	itemsToShow := len(items)
	if itemsToShow > maxItemsToShow {
		itemsToShow = maxItemsToShow
	}

	for i := 0; i < itemsToShow; i++ {
		item := items[i]
		sb.WriteString(fmt.Sprintf("• %s for %s\n", item.ItemName, item.UserName))
	}

	if len(items) > maxItemsToShow {
		remaining := len(items) - maxItemsToShow
		sb.WriteString(fmt.Sprintf("... and %d more items\n", remaining))
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

func (c *Client) formatIndividualMessage(item ItemInfo, itemNum, totalItems int) string {
	var sb strings.Builder
	
	// Title with item counter if multiple items
	if totalItems > 1 {
		sb.WriteString(fmt.Sprintf("📋 New item needed (%d/%d)\n", itemNum, totalItems))
	} else {
		sb.WriteString("📋 New item needed\n")
	}
	
	// Item details with rich formatting
	sb.WriteString(fmt.Sprintf("🎯 **%s**\n", item.ItemName))
	sb.WriteString(fmt.Sprintf("👤 For: %s\n", item.UserName))
	
	// Add crime link if available
	if item.CrimeURL != "" {
		sb.WriteString(fmt.Sprintf("🔗 Crime: %s\n", item.CrimeURL))
	}
	
	return strings.TrimSuffix(sb.String(), "\n")
}

// Circuit breaker and retry helper methods

func (c *Client) isCircuitOpen() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	if !c.circuitOpen {
		return false
	}
	
	// Check if we should try to close the circuit (half-open state)
	if time.Since(c.lastFailure) > 30*time.Second {
		c.mutex.RUnlock()
		c.mutex.Lock()
		c.circuitOpen = false
		c.failures = 0
		c.mutex.Unlock()
		c.mutex.RLock()
		log.Info().Msg("Circuit breaker moving to half-open state")
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
		log.Info().Msg("Circuit breaker closed after successful notification")
	}
}

func (c *Client) recordFailure() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.totalFailed++
	c.failures++
	c.lastFailure = time.Now()
	
	// Open circuit breaker after 5 consecutive failures
	if c.failures >= 5 && !c.circuitOpen {
		c.circuitOpen = true
		log.Warn().
			Int("failures", c.failures).
			Msg("Circuit breaker opened due to consecutive failures")
	}
}

func (c *Client) incrementRetries() {
	c.mutex.Lock()
	c.totalRetries++
	c.mutex.Unlock()
}

func (c *Client) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	base := float64(c.baseDelay)
	backoff := base * math.Pow(2, float64(attempt-1))
	
	// Add jitter (±25%)
	jitter := rand.Float64()*0.5 - 0.25  // -0.25 to +0.25
	backoff = backoff * (1 + jitter)
	
	// Cap at maxDelay
	maxBackoff := float64(c.maxDelay)
	if backoff > maxBackoff {
		backoff = maxBackoff
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

// GetMetrics returns current notification metrics
func (c *Client) GetMetrics() (sent, failed, retries int64) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.totalSent, c.totalFailed, c.totalRetries
}

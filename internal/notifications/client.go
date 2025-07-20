package notifications

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	topic      string
	enabled    bool
}

type ItemInfo struct {
	ItemName string
	UserName string
	CrimeURL string
}

func NewClient(baseURL, topic string, enabled bool) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
		topic:   topic,
		enabled: enabled,
	}
}

func (c *Client) SendNotification(ctx context.Context, message string) error {
	if !c.enabled {
		log.Debug().Msg("Notifications disabled, skipping")
		return nil
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, c.topic)

	log.Debug().
		Str("url", url).
		Str("message", message).
		Msg("Sending notification")

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("failed to create notification request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send notification")
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Warn().
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Msg("Notification request failed")
		return fmt.Errorf("notification request failed with status: %s", resp.Status)
	}

	log.Debug().
		Int("status_code", resp.StatusCode).
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

	message := c.formatBatchMessage(items, totalAdded)

	log.Info().
		Int("items_added", totalAdded).
		Msg("Sending notification for new items")

	c.SendNotificationAsync(ctx, message)
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

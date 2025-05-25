package torn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Client struct {
	apiKey    string
	client    *http.Client
	itemCache sync.Map
	userCache sync.Map
}

type Item struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Effect      string  `json:"effect"`
	Type        string  `json:"type"`
	BuyPrice    int     `json:"buy_price"`
	SellPrice   int     `json:"sell_price"`
	MarketValue float64 `json:"market_value"`
	Circulation int     `json:"circulation"`
	Image       string  `json:"image"`
	Tradeable   bool    `json:"tradeable"`
}

type ItemsResponse struct {
	Items map[string]Item `json:"items"`
}

// User API types
type UserStatus struct {
	Description string `json:"description"`
	Details     string `json:"details"`
	State       string `json:"state"`
	Color       string `json:"color"`
	Until       int    `json:"until"`
}

type UserInfo struct {
	Level    int        `json:"level"`
	Gender   string     `json:"gender"`
	PlayerID int        `json:"player_id"`
	Name     string     `json:"name"`
	Status   UserStatus `json:"status"`
}

// Crime-related types
type ItemRequirement struct {
	ID          int  `json:"id"`
	IsReusable  bool `json:"is_reusable"`
	IsAvailable bool `json:"is_available"`
}

type User struct {
	ID       int     `json:"id"`
	JoinedAt int     `json:"joined_at"`
	Progress float64 `json:"progress"`
}

type Slot struct {
	Position           string           `json:"position"`
	ItemRequirement    *ItemRequirement `json:"item_requirement"`
	User               *User            `json:"user"`
	CheckpointPassRate int              `json:"checkpoint_pass_rate"`
}

type Crime struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Slots  []Slot `json:"slots"`
}

type CrimesResponse struct {
	Crimes []Crime `json:"crimes"`
}

type UnavailableItem struct {
	ItemID  int `json:"item_id"`
	UserID  int `json:"user_id"`
	CrimeID int `json:"crime_id"`
}

type cachedItem struct {
	item      *Item
	timestamp time.Time
}

type cachedUser struct {
	user      *UserInfo
	timestamp time.Time
}

// Log API types
type LogItem struct {
	ID  int `json:"id"`
	UID int `json:"uid"`
	Qty int `json:"qty"`
}

type ItemSendData struct {
	Receiver int       `json:"receiver"`
	Items    []LogItem `json:"items"`
	Message  string    `json:"message"`
}

type LogEntry struct {
	Log       int          `json:"log"`
	Title     string       `json:"title"`
	Timestamp int64        `json:"timestamp"`
	Category  string       `json:"category"`
	Data      ItemSendData `json:"data"`
}

type LogResponse struct {
	Log map[string]LogEntry `json:"log"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) GetItem(ctx context.Context, itemID string) (*Item, error) {
	// Check cache first
	if cached, ok := c.itemCache.Load(itemID); ok {
		cachedItem := cached.(cachedItem)
		// Cache valid for 1 hour
		if time.Since(cachedItem.timestamp) < time.Hour {
			return cachedItem.item, nil
		}
	}

	url := fmt.Sprintf("https://api.torn.com/torn/%s?selections=items&key=%s", itemID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items map[string]Item `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	item, ok := result.Items[itemID]
	if !ok {
		return nil, fmt.Errorf("item %s not found", itemID)
	}

	// Cache the result
	c.itemCache.Store(itemID, cachedItem{
		item:      &item,
		timestamp: time.Now(),
	})

	return &item, nil
}

func (c *Client) GetUser(ctx context.Context, userID string) (*UserInfo, error) {
	// Check cache first
	if cached, ok := c.userCache.Load(userID); ok {
		cachedUser := cached.(cachedUser)
		// Cache valid for 1 hour
		if time.Since(cachedUser.timestamp) < time.Hour {
			return cachedUser.user, nil
		}
	}

	url := fmt.Sprintf("https://api.torn.com/user/%s?selections=basic&key=%s", userID, c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Cache the result
	c.userCache.Store(userID, cachedUser{
		user:      &userInfo,
		timestamp: time.Now(),
	})

	return &userInfo, nil
}

func (c *Client) GetFactionCrimes(ctx context.Context, category string, offset int) (*CrimesResponse, error) {
	url := fmt.Sprintf("https://api.torn.com/v2/faction/crimes?key=%s&cat=%s&offset=%d", c.apiKey, category, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	var crimesResp CrimesResponse
	if err := json.NewDecoder(resp.Body).Decode(&crimesResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &crimesResp, nil
}

func (c *Client) GetUnavailableItems(ctx context.Context) ([]UnavailableItem, error) {
	crimesResp, err := c.GetFactionCrimes(ctx, "planning", 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get planning crimes: %w", err)
	}

	var unavailableItems []UnavailableItem

	for _, crime := range crimesResp.Crimes {
		for _, slot := range crime.Slots {
			if slot.ItemRequirement != nil && !slot.ItemRequirement.IsAvailable && slot.User != nil {
				unavailableItems = append(unavailableItems, UnavailableItem{
					ItemID:  slot.ItemRequirement.ID,
					UserID:  slot.User.ID,
					CrimeID: crime.ID,
				})
			}
		}
	}

	return unavailableItems, nil
}

func (c *Client) GetItemSendLogs(ctx context.Context) (*LogResponse, error) {
	log.Debug().Msg("Making request to item send logs API")

	// Calculate timestamps for last 48 hours
	now := time.Now()
	from := now.Add(-48 * time.Hour).Unix()
	to := now.Unix()

	url := fmt.Sprintf("https://api.torn.com/user?selections=log&log=4102&from=%d&to=%d&key=%s", from, to, c.apiKey)

	log.Debug().
		Int64("from_timestamp", from).
		Int64("to_timestamp", to).
		Str("from_time", time.Unix(from, 0).Format("2006-01-02 15:04:05")).
		Str("to_time", time.Unix(to, 0).Format("2006-01-02 15:04:05")).
		Msg("Querying logs for time range")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Increment API call counter
	c.IncrementAPICall()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	log.Debug().
		Int("status_code", resp.StatusCode).
		Str("content_type", resp.Header.Get("Content-Type")).
		Msg("Received API response")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Debug().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Msg("Non-200 response from API")
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the entire response body first for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Debug().
		Int("body_length", len(body)).
		Str("response_body_preview", string(body[:min(500, len(body))])).
		Msg("Read response body")

	var logResp LogResponse
	if err := json.Unmarshal(body, &logResp); err != nil {
		log.Debug().
			Err(err).
			Str("response_body", string(body)).
			Msg("Failed to unmarshal JSON response")
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Debug().
		Int("log_entries_count", len(logResp.Log)).
		Msg("Successfully parsed log response")

	// Log a few sample log IDs if available
	if len(logResp.Log) > 0 {
		count := 0
		for logID := range logResp.Log {
			if count >= 3 {
				break
			}
			log.Debug().
				Str("sample_log_id", logID).
				Msg("Sample log entry ID")
			count++
		}
	}

	return &logResp, nil
}

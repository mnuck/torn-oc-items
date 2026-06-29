package torn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"torn_oc_items/internal/config"
	"torn_oc_items/internal/retry"

	"log/slog"
)

type Client struct {
	apiKey        string
	factionApiKey string
	client        *http.Client
	itemCache     sync.Map
	userCache     sync.Map
	apiCallCount  int64
	apiCallMutex  sync.Mutex
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

type SuppliedItem struct {
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
	Log []LogEntry `json:"log"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewClient(apiKey string, factionApiKey string) *Client {
	return &Client{
		apiKey:        apiKey,
		factionApiKey: factionApiKey,
		client:        &http.Client{
			// No timeout - let retry logic's context handle all timeouts
		},
	}
}

// IncrementAPICall safely increments the API call counter
func (c *Client) IncrementAPICall() {
	c.apiCallMutex.Lock()
	c.apiCallCount++
	c.apiCallMutex.Unlock()
}

// makeAPIRequest creates and executes an HTTP GET request to the Torn API with retry logic
func (c *Client) makeAPIRequest(ctx context.Context, url string) (*http.Response, error) {
	return retry.WithRetry(ctx, config.DefaultResilienceConfig.APIRequest, func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			slog.Debug("API request failed", "error", err, "url", url)
			return nil, fmt.Errorf("failed to make request: %w", err)
		}

		// Only increment API call counter after successful request
		c.IncrementAPICall()

		return resp, nil
	})
}

// handleAPIResponse processes the HTTP response and returns the body bytes
func (c *Client) handleAPIResponse(resp *http.Response) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug("Failed to read response body - detailed error info", "error", err, "status_code", resp.StatusCode, "content_type", resp.Header.Get("Content-Type"))

		// If we successfully got headers but failed reading body, this is likely a network issue
		// that should be retried rather than treated as a permanent failure
		if resp.StatusCode == 200 && err.Error() == "context canceled" {
			return nil, fmt.Errorf("network connection interrupted during body read: %w", err)
		}

		return nil, fmt.Errorf("failed to read response body (status: %d): %w", resp.StatusCode, err)
	}

	return body, nil
}

// GetAPICallCount returns the current API call count
func (c *Client) GetAPICallCount() int64 {
	c.apiCallMutex.Lock()
	defer c.apiCallMutex.Unlock()
	return c.apiCallCount
}

// ResetAPICallCount resets the API call counter to zero
func (c *Client) ResetAPICallCount() {
	c.apiCallMutex.Lock()
	c.apiCallCount = 0
	c.apiCallMutex.Unlock()
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

	return retry.WithRetry(ctx, config.DefaultResilienceConfig.APIRequest, func(ctx context.Context) (*Item, error) {
		url := fmt.Sprintf("https://api.torn.com/torn/%s?selections=items&key=%s", itemID, c.apiKey)
		resp, err := c.makeAPIRequest(ctx, url)
		if err != nil {
			return nil, err
		}

		body, err := c.handleAPIResponse(resp)
		if err != nil {
			return nil, err
		}

		var result struct {
			Items map[string]Item `json:"items"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
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
	})
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

	return retry.WithRetry(ctx, config.DefaultResilienceConfig.APIRequest, func(ctx context.Context) (*UserInfo, error) {
		url := fmt.Sprintf("https://api.torn.com/user/%s?selections=basic&key=%s", userID, c.apiKey)

		resp, err := c.makeAPIRequest(ctx, url)
		if err != nil {
			return nil, err
		}

		body, err := c.handleAPIResponse(resp)
		if err != nil {
			return nil, err
		}

		var userInfo UserInfo
		if err := json.Unmarshal(body, &userInfo); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Cache the result
		c.userCache.Store(userID, cachedUser{
			user:      &userInfo,
			timestamp: time.Now(),
		})

		return &userInfo, nil
	})
}

func (c *Client) GetFactionCrimes(ctx context.Context, category string, offset int) (*CrimesResponse, error) {
	return retry.WithRetry(ctx, config.DefaultResilienceConfig.APIRequest, func(ctx context.Context) (*CrimesResponse, error) {
		url := fmt.Sprintf("https://api.torn.com/v2/faction/crimes?key=%s&cat=%s&offset=%d", c.factionApiKey, category, offset)

		resp, err := c.makeAPIRequest(ctx, url)
		if err != nil {
			return nil, err
		}

		body, err := c.handleAPIResponse(resp)
		if err != nil {
			return nil, err
		}

		var crimesResp CrimesResponse
		if err := json.Unmarshal(body, &crimesResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		return &crimesResp, nil
	})
}

func (c *Client) GetSuppliedItems(ctx context.Context) ([]SuppliedItem, error) {
	slog.Debug("Fetching faction crimes for supplied items")
	crimesResp, err := c.GetFactionCrimes(ctx, "planning", 0)
	if err != nil {
		slog.Error("Failed to get planning crimes", "error", err)
		return nil, fmt.Errorf("failed to get planning crimes: %w", err)
	}

	slog.Debug("Retrieved faction crimes", "total_crimes", len(crimesResp.Crimes))

	suppliedItems := c.processCrimesForSuppliedItems(crimesResp.Crimes)

	slog.Debug("Finished processing supplied items", "total_supplied_items", len(suppliedItems))

	return suppliedItems, nil
}

func (c *Client) GetCompletedCrimes(ctx context.Context) (*CrimesResponse, error) {
	slog.Debug("Fetching completed faction crimes")
	return c.GetFactionCrimes(ctx, "completed", 0)
}

func (c *Client) GetPlanningCrimes(ctx context.Context) (*CrimesResponse, error) {
	slog.Debug("Fetching planning faction crimes")
	return c.GetFactionCrimes(ctx, "planning", 0)
}

// processCrimesForSuppliedItems processes all crimes and returns supplied items
func (c *Client) processCrimesForSuppliedItems(crimes []Crime) []SuppliedItem {
	var suppliedItems []SuppliedItem

	for _, crime := range crimes {
		c.logCrimeProcessing(crime)
		crimeSuppliedItems := c.processCrimeSlots(crime)
		suppliedItems = append(suppliedItems, crimeSuppliedItems...)
	}

	return suppliedItems
}

// logCrimeProcessing logs information about the crime being processed
func (c *Client) logCrimeProcessing(crime Crime) {
	slog.Debug("Processing crime", "crime_id", crime.ID, "crime_name", crime.Name, "crime_status", crime.Status, "slots", len(crime.Slots))
}

// processCrimeSlots processes all slots in a crime and returns supplied items
func (c *Client) processCrimeSlots(crime Crime) []SuppliedItem {
	var suppliedItems []SuppliedItem

	for slotIndex, slot := range crime.Slots {
		c.logSlotProcessing(crime.ID, slotIndex, slot)

		if suppliedItem := c.processSlotForSuppliedItem(crime.ID, slotIndex, slot); suppliedItem != nil {
			suppliedItems = append(suppliedItems, *suppliedItem)
		}
	}

	return suppliedItems
}

// logSlotProcessing logs detailed information about slot processing
func (c *Client) logSlotProcessing(crimeID, slotIndex int, slot Slot) {
	slog.Debug("Processing slot", "crime_id", crimeID, "slot_index", slotIndex, "position", slot.Position, "has_item_requirement", slot.ItemRequirement != nil, "has_user", slot.User != nil)
	if slot.ItemRequirement != nil {
		slog.Debug("Item requirement details", "crime_id", crimeID, "slot_index", slotIndex, "item_id", slot.ItemRequirement.ID, "is_reusable", slot.ItemRequirement.IsReusable, "is_available", slot.ItemRequirement.IsAvailable)
	}
	if slot.User != nil {
		slog.Debug("User details", "crime_id", crimeID, "slot_index", slotIndex, "user_id", slot.User.ID, "progress", slot.User.Progress)
	}
}

// processSlotForSuppliedItem processes a single slot and returns a supplied item if conditions are met
func (c *Client) processSlotForSuppliedItem(crimeID, slotIndex int, slot Slot) *SuppliedItem {
	// Early exit if there is no item requirement
	if slot.ItemRequirement == nil {
		return nil
	}

	// Check if item should be supplied based on reusability and availability
	if !c.shouldSupplyItem(slot.ItemRequirement) {
		return nil
	}

	// Must have a user to supply the item to
	if slot.User == nil {
		return nil
	}

	slog.Info("Found supplied item", "crime_id", crimeID, "slot_index", slotIndex, "item_id", slot.ItemRequirement.ID, "user_id", slot.User.ID)

	return &SuppliedItem{
		ItemID:  slot.ItemRequirement.ID,
		UserID:  slot.User.ID,
		CrimeID: crimeID,
	}
}

// shouldSupplyItem determines if an item should be supplied based on its requirements
func (c *Client) shouldSupplyItem(requirement *ItemRequirement) bool {
	// If the item is not reusable, we will always provide it
	// If the item is reusable, we will only provide it if it is not available
	return !requirement.IsReusable || (requirement.IsReusable && !requirement.IsAvailable)
}

func (c *Client) GetItemSendLogs(ctx context.Context) (*LogResponse, error) {
	slog.Debug("Making request to item send logs API")

	// Calculate timestamps for last 48 hours
	now := time.Now()
	from := now.Add(-48 * time.Hour).Unix()
	to := now.Unix()

	return retry.WithRetry(ctx, config.DefaultResilienceConfig.APIRequest, func(ctx context.Context) (*LogResponse, error) {
		url := fmt.Sprintf("https://api.torn.com/user?selections=log&log=4102&from=%d&to=%d&key=%s", from, to, c.apiKey)

		slog.Debug("Querying logs for time range", "from_timestamp", from, "to_timestamp", to, "from_time", time.Unix(from, 0).Format("2006-01-02 15:04:05"), "to_time", time.Unix(to, 0).Format("2006-01-02 15:04:05"))

		resp, err := c.makeAPIRequest(ctx, url)
		if err != nil {
			return nil, err
		}

		slog.Debug("Received API response", "status_code", resp.StatusCode, "content_type", resp.Header.Get("Content-Type"))

		body, err := c.handleAPIResponse(resp)
		if err != nil {
			return nil, err
		}

		slog.Debug("Read response body", "body_length", len(body), "response_body_preview", string(body[:min(500, len(body))]))

		var logResp LogResponse
		if err := json.Unmarshal(body, &logResp); err != nil {
			slog.Debug("Failed to unmarshal JSON response", "error", err, "response_body", string(body))
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		slog.Debug("Successfully parsed log response", "log_entries_count", len(logResp.Log))

		// Log a few sample entries if available
		if len(logResp.Log) > 0 {
			count := min(3, len(logResp.Log))
			for i := 0; i < count; i++ {
				slog.Debug("Sample log entry", "log_entry_index", i, "log_type", logResp.Log[i].Log)
			}
		}

		return &logResp, nil
	})
}

func (c *Client) WhoAmI(ctx context.Context) (string, error) {
	return retry.WithRetry(ctx, config.DefaultResilienceConfig.APIRequest, func(ctx context.Context) (string, error) {
		url := fmt.Sprintf("https://api.torn.com/user/?selections=basic&key=%s", c.apiKey)

		resp, err := c.makeAPIRequest(ctx, url)
		if err != nil {
			return "", err
		}

		body, err := c.handleAPIResponse(resp)
		if err != nil {
			return "", err
		}

		var userInfo UserInfo
		if err := json.Unmarshal(body, &userInfo); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}

		return userInfo.Name, nil
	})
}

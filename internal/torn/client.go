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

	"github.com/rs/zerolog/log"
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
	Log map[string]LogEntry `json:"log"`
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
		client: &http.Client{
			Timeout: config.DefaultResilienceConfig.APIRequest.Timeout,
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
			log.Debug().
				Err(err).
				Str("url", url).
				Msg("API request failed")
			return nil, fmt.Errorf("failed to make request: %w", err)
		}

		// Only increment API call counter after successful request
		c.IncrementAPICall()

		return resp, nil
	})
}

// handleAPIResponse processes the HTTP response and returns the body bytes
func (c *Client) handleAPIResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
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
}

func (c *Client) GetFactionCrimes(ctx context.Context, category string, offset int) (*CrimesResponse, error) {
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
}

func (c *Client) GetSuppliedItems(ctx context.Context) ([]SuppliedItem, error) {
	log.Debug().Msg("Fetching faction crimes for supplied items")
	crimesResp, err := c.GetFactionCrimes(ctx, "planning", 0)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get planning crimes")
		return nil, fmt.Errorf("failed to get planning crimes: %w", err)
	}

	log.Debug().
		Int("total_crimes", len(crimesResp.Crimes)).
		Msg("Retrieved faction crimes")

	suppliedItems := c.processCrimesForSuppliedItems(crimesResp.Crimes)

	log.Debug().
		Int("total_supplied_items", len(suppliedItems)).
		Msg("Finished processing supplied items")

	return suppliedItems, nil
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
	log.Debug().
		Int("crime_id", crime.ID).
		Str("crime_name", crime.Name).
		Str("crime_status", crime.Status).
		Int("slots", len(crime.Slots)).
		Msg("Processing crime")
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
	log.Debug().
		Int("crime_id", crimeID).
		Int("slot_index", slotIndex).
		Str("position", slot.Position).
		Bool("has_item_requirement", slot.ItemRequirement != nil).
		Bool("has_user", slot.User != nil).
		Msg("Processing slot")

	if slot.ItemRequirement != nil {
		log.Debug().
			Int("crime_id", crimeID).
			Int("slot_index", slotIndex).
			Int("item_id", slot.ItemRequirement.ID).
			Bool("is_reusable", slot.ItemRequirement.IsReusable).
			Bool("is_available", slot.ItemRequirement.IsAvailable).
			Msg("Item requirement details")
	}

	if slot.User != nil {
		log.Debug().
			Int("crime_id", crimeID).
			Int("slot_index", slotIndex).
			Int("user_id", slot.User.ID).
			Float64("progress", slot.User.Progress).
			Msg("User details")
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

	log.Info().
		Int("crime_id", crimeID).
		Int("slot_index", slotIndex).
		Int("item_id", slot.ItemRequirement.ID).
		Int("user_id", slot.User.ID).
		Msg("Found supplied item")

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

	resp, err := c.makeAPIRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	log.Debug().
		Int("status_code", resp.StatusCode).
		Str("content_type", resp.Header.Get("Content-Type")).
		Msg("Received API response")

	body, err := c.handleAPIResponse(resp)
	if err != nil {
		return nil, err
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

func (c *Client) WhoAmI(ctx context.Context) (string, error) {
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
}

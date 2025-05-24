package torn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	apiKey string
	client *http.Client
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

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (c *Client) GetItem(ctx context.Context, itemID string) (*Item, error) {
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

	var itemsResp ItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&itemsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	item, ok := itemsResp.Items[itemID]
	if !ok {
		return nil, fmt.Errorf("item %s not found", itemID)
	}

	return &item, nil
}

func (c *Client) GetUser(ctx context.Context, userID string) (*UserInfo, error) {
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

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

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

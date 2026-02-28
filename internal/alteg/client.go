package alteg

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrUnauthorized is returned when the API responds with HTTP 401.
// It signals that the bearer token has expired and must be renewed.
var ErrUnauthorized = errors.New("bearer token is expired or invalid (401)")

const (
	apiURL    = "https://n1338118.alteg.io/api/v1/activity/777474/search"
	CompanyID = 777474
	ServiceID = 12995896
	StaffID   = 2811495

	httpTimeout = 30 * time.Second
)

// Staff holds information about the coach.
type Staff struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Service holds information about the activity service.
type Service struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// Activity represents a single activity entry from the API response.
type Activity struct {
	ID           int     `json:"id"`
	Date         string  `json:"date"`
	Capacity     int     `json:"capacity"`
	RecordsCount int     `json:"records_count"`
	Staff        Staff   `json:"staff"`
	Service      Service `json:"service"`
}

// searchResponse is the top-level API response structure.
type searchResponse struct {
	Success bool       `json:"success"`
	Data    []Activity `json:"data"`
}

// AvailablePlaces returns how many spots are free for the activity.
func (a Activity) AvailablePlaces() int {
	return a.Capacity - a.RecordsCount
}

// ActivityURL builds the link to the activity booking page.
func ActivityURL(activityID int) string {
	return fmt.Sprintf("https://n1338118.alteg.io/company/%d/activity/info/%d", CompanyID, activityID)
}

// Client is responsible for communicating with the Alteg API.
type Client struct {
	bearerToken string
	http        *http.Client
}

// NewClient creates a new Alteg API client.
func NewClient(bearerToken string) *Client {
	return &Client{
		bearerToken: bearerToken,
		http:        &http.Client{Timeout: httpTimeout},
	}
}

// UpdateToken replaces the bearer token used for API requests.
func (c *Client) UpdateToken(newToken string) {
	c.bearerToken = newToken
}

// FetchAvailableActivities calls the Alteg API and returns only activities that have free spots.
func (c *Client) FetchAvailableActivities() ([]Activity, error) {
	now := time.Now()

	from := now.Format("2006-01-02")

	// till = 1st day of next month + 2 months ahead
	firstOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	till := firstOfNextMonth.AddDate(0, 2, 0).Format("2006-01-02")

	params := url.Values{}
	params.Set("from", from)
	params.Set("till", till)
	params.Add("service_ids[]", strconv.Itoa(ServiceID))
	params.Add("staff_ids[]", strconv.Itoa(StaffID))

	reqURL := apiURL + "?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !sr.Success {
		return nil, fmt.Errorf("API returned success=false")
	}

	var available []Activity
	for _, a := range sr.Data {
		if a.AvailablePlaces() > 0 {
			available = append(available, a)
		}
	}
	return available, nil
}

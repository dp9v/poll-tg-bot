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
	defaultAPIURL = "https://n1338118.alteg.io/api/v1/activity/777474/search"
	CompanyID     = 777474
	// ServiceID and StaffID are kept for the activity-URL helper and as a
	// reference; they are no longer used to filter API requests.
	ServiceID = 12995896
	StaffID   = 2811495

	httpTimeout = 30 * time.Second

	// defaultPageSize is the default number of activities requested per page.
	defaultPageSize = 200
	// maxPages is a hard safety limit to avoid runaway pagination loops.
	maxPages = 100
)

// Staff holds information about the coach.
type Staff struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Specialization string  `json:"specialization"`
	Avatar         string  `json:"avatar"`
	Rating         float64 `json:"rating"`
}

// Category represents a service category. The Alteg API exposes a recursive
// `category` field; we only model two attributes we actually persist.
type Category struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	ParentID int    `json:"category_id"` // id of the parent category (0 == root)
}

// Service holds information about the activity service.
type Service struct {
	ID         int      `json:"id"`
	Title      string   `json:"title"`
	CategoryID int      `json:"category_id"`
	PriceMin   int      `json:"price_min"`
	PriceMax   int      `json:"price_max"`
	Category   Category `json:"category"`
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

// searchMeta is the pagination metadata returned by the API.
type searchMeta struct {
	Count int `json:"count"`
}

// searchResponse is the top-level API response structure.
type searchResponse struct {
	Success bool       `json:"success"`
	Data    []Activity `json:"data"`
	Meta    searchMeta `json:"meta"`
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
	apiURL      string
	pageSize    int
}

// NewClient creates a new Alteg API client.
func NewClient(bearerToken string) *Client {
	return &Client{
		bearerToken: bearerToken,
		http:        &http.Client{Timeout: httpTimeout},
		apiURL:      defaultAPIURL,
		pageSize:    defaultPageSize,
	}
}

// WithBaseURL overrides the API base URL. Useful for tests.
func (c *Client) WithBaseURL(u string) *Client {
	c.apiURL = u
	return c
}

// WithPageSize overrides the pagination page size.
// Values <= 0 fall back to the default.
func (c *Client) WithPageSize(n int) *Client {
	if n > 0 {
		c.pageSize = n
	}
	return c
}

// UpdateToken replaces the bearer token used for API requests.
func (c *Client) UpdateToken(newToken string) {
	c.bearerToken = newToken
}

// FetchActivities calls the Alteg API and returns all activities (including fully booked ones)
// within the given [from, till] date range. Pagination is handled transparently using the
// `count` and `page` query parameters.
func (c *Client) FetchActivities(from, till time.Time) ([]Activity, error) {
	const dateLayout = "2006-01-02"

	pageSize := c.pageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}

	var all []Activity
	for page := 1; page <= maxPages; page++ {
		params := url.Values{}
		params.Set("from", from.Format(dateLayout))
		params.Set("till", till.Format(dateLayout))
		params.Set("count", strconv.Itoa(pageSize))
		params.Set("page", strconv.Itoa(page))

		sr, err := c.doSearch(params)
		if err != nil {
			return nil, err
		}

		all = append(all, sr.Data...)

		// Termination conditions:
		//  - empty page: nothing more to read
		//  - short page: last page reached
		//  - meta.count provided and we've already collected at least that many rows
		if len(sr.Data) == 0 {
			break
		}
		if len(sr.Data) < pageSize {
			break
		}
		if sr.Meta.Count > 0 && len(all) >= sr.Meta.Count {
			break
		}
	}

	return all, nil
}

// doSearch performs a single paginated request and decodes the response.
func (c *Client) doSearch(params url.Values) (*searchResponse, error) {
	reqURL := c.apiURL + "?" + params.Encode()

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
	return &sr, nil
}

package flashduty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client represents a Flashduty API client
type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	appKey     string
	userAgent  string
}

// GetFlashdutyClientFn is a function that returns a flashduty client
type GetFlashdutyClientFn func(context.Context) (context.Context, *Client, error)

// NewClient creates a new Flashduty API client
func NewClient(appKey, baseURL, userAgent string) (*Client, error) {
	if appKey == "" {
		return nil, fmt.Errorf("APP key is required")
	}

	if baseURL == "" {
		baseURL = "https://api.flashcat.cloud"
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	return &Client{
		httpClient: &http.Client{},
		baseURL:    parsedURL,
		appKey:     appKey,
		userAgent:  userAgent,
	}, nil
}

// SetUserAgent sets the user agent for the client
func (c *Client) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

// makeRequest makes an HTTP request to the Flashduty API
func (c *Client) makeRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	// Construct full URL with app_key query parameter
	fullURL := c.baseURL.ResolveReference(&url.URL{Path: strings.TrimPrefix(path, "/")})
	query := fullURL.Query()
	query.Set("app_key", c.appKey)
	fullURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	return resp, nil
}

// parseResponse parses the HTTP response into the given interface
func parseResponse(resp *http.Response, v interface{}) error {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// FlashdutyResponse represents the standard Flashduty API response structure
type FlashdutyResponse struct {
	Error *DutyError  `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

// DutyError represents Flashduty API error
type DutyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MemberListResponse represents the response for member list API
type MemberListResponse struct {
	Error *DutyError `json:"error,omitempty"`
	Data  *struct {
		P     int          `json:"p"`
		Limit int          `json:"limit"`
		Total int          `json:"total"`
		Items []MemberItem `json:"items"`
	} `json:"data,omitempty"`
}

// MemberItem represents a member item as defined in the OpenAPI spec
type MemberItem struct {
	MemberID       int    `json:"member_id"`
	MemberName     string `json:"member_name"`
	Phone          string `json:"phone,omitempty"`
	PhoneVerified  bool   `json:"phone_verified,omitempty"`
	Email          string `json:"email,omitempty"`
	EmailVerified  bool   `json:"email_verified,omitempty"`
	AccountRoleIDs []int  `json:"account_role_ids,omitempty"`
	TimeZone       string `json:"time_zone,omitempty"`
	Locale         string `json:"locale,omitempty"`
	Status         string `json:"status"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	RefID          string `json:"ref_id,omitempty"`
}

// MemberItemShort represents a short member item for invite response
type MemberItemShort struct {
	MemberID   int    `json:"MemberID"`
	MemberName string `json:"MemberName"`
}

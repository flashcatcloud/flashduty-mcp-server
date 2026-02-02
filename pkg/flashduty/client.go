package flashduty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcplog "github.com/flashcatcloud/flashduty-mcp-server/pkg/log"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/trace"
)

const (
	// maxResponseBodySize limits the response body size to prevent OOM attacks (10MB)
	maxResponseBodySize = 10 * 1024 * 1024
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
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:   parsedURL,
		appKey:    appKey,
		userAgent: userAgent,
	}, nil
}

// SetUserAgent sets the user agent for the client
func (c *Client) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

// makeRequest makes an HTTP request to the Flashduty API
func (c *Client) makeRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	var reqBodyBytes []byte

	if body != nil {
		var err error
		reqBodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("invalid request body: unable to serialize to JSON: %w", err)
		}
		reqBody = bytes.NewBuffer(reqBodyBytes)
	}

	// Parse path to handle query parameters correctly
	parsedPath, err := url.Parse(strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse path: %w", err)
	}

	// Construct full URL with app_key query parameter
	fullURL := c.baseURL.ResolveReference(parsedPath)
	query := fullURL.Query()
	query.Set("app_key", c.appKey)
	fullURL.RawQuery = query.Encode()

	// Extract trace context for logging and propagation
	traceCtx := trace.FromContext(ctx)

	// Log request with trace_id first (after msg in output)
	logAttrs := []any{}
	if traceCtx != nil {
		logAttrs = append(logAttrs, "trace_id", traceCtx.TraceID)
	}
	logAttrs = append(logAttrs, "method", method, "url", sanitizeURL(fullURL), "body", mcplog.TruncateBodyDefault(string(reqBodyBytes)))
	slog.Info("duty request", logAttrs...)

	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	// Propagate trace context to downstream service
	if traceCtx != nil {
		traceCtx.SetHTTPHeaders(req.Header)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Sanitize error to avoid leaking app_key in logs
		return nil, fmt.Errorf("failed to make request to %s %s: %v", method, sanitizeURL(fullURL), sanitizeError(err))
	}

	return resp, nil
}

// sanitizeURL removes sensitive query parameters from URL for safe logging
func sanitizeURL(u *url.URL) string {
	sanitized := *u
	q := sanitized.Query()
	if q.Has("app_key") {
		q.Set("app_key", "[REDACTED]")
		sanitized.RawQuery = q.Encode()
	}
	return sanitized.String()
}

// sanitizeError removes potential URL with sensitive data from error messages
func sanitizeError(err error) string {
	errStr := err.Error()
	idx := strings.Index(errStr, "app_key=")
	if idx == -1 {
		return errStr
	}

	endIdx := strings.IndexAny(errStr[idx:], "& ")
	if endIdx == -1 {
		return errStr[:idx] + "app_key=[REDACTED]"
	}
	return errStr[:idx] + "app_key=[REDACTED]" + errStr[idx+endIdx:]
}

// parseResponse parses the HTTP response into the given interface.
// Note: caller is responsible for closing resp.Body.
func parseResponse(resp *http.Response, v interface{}) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Build log attributes with trace context
	logAttrs := []any{}
	if traceCtx := trace.FromContext(resp.Request.Context()); traceCtx != nil {
		logAttrs = append(logAttrs, "trace_id", traceCtx.TraceID)
	}
	logAttrs = append(logAttrs, "status", resp.StatusCode, "body", mcplog.TruncateBodyDefault(string(body)))

	requestID := resp.Header.Get("Flashcat-Request-Id")

	if resp.StatusCode >= 500 {
		slog.Error("duty response", logAttrs...)
		return fmt.Errorf("API server error (HTTP %d, request_id: %s): %s", resp.StatusCode, requestID, string(body))
	}

	if resp.StatusCode >= 400 {
		slog.Warn("duty response", logAttrs...)
		return fmt.Errorf("API client error (HTTP %d, request_id: %s): %s", resp.StatusCode, requestID, string(body))
	}

	slog.Info("duty response", logAttrs...)

	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return fmt.Errorf("invalid API response: failed to parse JSON (response size: %d bytes, request_id: %s): %w", len(body), requestID, err)
		}
	}

	return nil
}

// handleAPIError reads the response body and returns a detailed error message.
// This function should be called when resp.StatusCode != http.StatusOK.
// It returns the full response body which contains request_id for debugging.
func handleAPIError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("API request failed (HTTP %d): unable to read response body: %v", resp.StatusCode, err)
	}

	// Build log attributes with trace context
	logAttrs := []any{}
	if traceCtx := trace.FromContext(resp.Request.Context()); traceCtx != nil {
		logAttrs = append(logAttrs, "trace_id", traceCtx.TraceID)
	}
	logAttrs = append(logAttrs, "status", resp.StatusCode, "body", mcplog.TruncateBodyDefault(string(body)))

	requestID := resp.Header.Get("Flashcat-Request-Id")

	if resp.StatusCode >= 500 {
		slog.Error("duty error", logAttrs...)
		return fmt.Errorf("API server error (HTTP %d, request_id: %s): %s", resp.StatusCode, requestID, string(body))
	}

	slog.Warn("duty error", logAttrs...)
	return fmt.Errorf("API client error (HTTP %d, request_id: %s): %s", resp.StatusCode, requestID, string(body))
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

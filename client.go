// Package ifax is the official Go client for the INeedToSendAFax API.
//
// Send faxes programmatically, track their status, and verify webhook
// callbacks. The client is safe for concurrent use.
//
// Create a client with your API key and send a fax:
//
//	client := ifax.New(os.Getenv("IFAX_API_KEY"))
//	fax, err := client.SendFax(ctx, &ifax.SendFaxParams{
//		To:    "15551234567",
//		Files: []ifax.File{ifax.FilePath("invoice.pdf")},
//	})
//
// See https://api.ineedtosendafax.com/docs for the API reference.
package ifax

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API endpoint.
const DefaultBaseURL = "https://api.ineedtosendafax.com"

// Version is the SDK version.
const Version = "1.0.0"

const userAgent = "ineedtosendafax-go/" + Version

// Client talks to the INeedToSendAFax API. It is safe for concurrent use; do
// not modify its fields directly. Construct it with New.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (useful for testing).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient uses a custom *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithTimeout sets the request timeout on a default HTTP client. It is ignored
// when WithHTTPClient is also supplied.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.httpClient.Timeout = d
		}
	}
}

// WithMaxRetries sets how many times transient failures (429 and 5xx responses,
// network errors) are retried. The default is 2.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// New returns a Client using the given API key. Options may override defaults.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		maxRetries: 2,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ErrNoAPIKey is returned by NewFromEnv when no key is present.
var ErrNoAPIKey = errors.New("ifax: no API key (set IFAX_API_KEY)")

// NewFromEnv builds a Client from the IFAX_API_KEY environment variable.
func NewFromEnv(opts ...Option) (*Client, error) {
	key := os.Getenv("IFAX_API_KEY")
	if key == "" {
		return nil, ErrNoAPIKey
	}
	return New(key, opts...), nil
}

// APIError is returned for any non-2xx API response.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	// RetryAfter is set when the API returns a Retry-After header.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("ifax: %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("ifax: HTTP %d: %s", e.StatusCode, e.Message)
}

// Is reports whether the error is an *APIError with the given code.
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	return ok && (t.Code == "" || t.Code == e.Code) && (t.StatusCode == 0 || t.StatusCode == e.StatusCode)
}

// Sentinel API errors for use with errors.Is.
var (
	ErrNotFound            = &APIError{StatusCode: http.StatusNotFound, Code: "not_found"}
	ErrInsufficientCredits = &APIError{StatusCode: http.StatusPaymentRequired, Code: "insufficient_credits"}
	ErrRateLimited         = &APIError{StatusCode: http.StatusTooManyRequests, Code: "rate_limited"}
	ErrInvalidKey          = &APIError{StatusCode: http.StatusUnauthorized, Code: "invalid_key"}
)

type apiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// do performs an HTTP request with retries and decodes the JSON response.
func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType string, headers map[string]string, out any) error {
	if c.apiKey == "" {
		return ErrNoAPIKey
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := backoff(attempt, lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil || len(data) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("ifax: decoding response: %w", err)
			}
			return nil
		}

		apiErr := parseAPIError(resp, data)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = apiErr
			continue
		}
		return apiErr
	}
	if lastErr == nil {
		lastErr = errors.New("ifax: request failed")
	}
	return lastErr
}

func parseAPIError(resp *http.Response, data []byte) *APIError {
	e := &APIError{StatusCode: resp.StatusCode}
	var body apiErrorBody
	if json.Unmarshal(data, &body) == nil {
		e.Code = body.Error.Code
		e.Message = body.Error.Message
	}
	if e.Message == "" {
		e.Message = strings.TrimSpace(string(data))
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return e
}

// backoff returns how long to wait before the given retry attempt (1-based).
func backoff(attempt int, lastErr error) time.Duration {
	var apiErr *APIError
	if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	d := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

// randomKey returns a random hex idempotency key.
func randomKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsInsufficientCredits reports whether err is a 402 (out of credit).
func IsInsufficientCredits(err error) bool { return errors.Is(err, ErrInsufficientCredits) }

// IsRateLimited reports whether err is a 429.
func IsRateLimited(err error) bool { return errors.Is(err, ErrRateLimited) }

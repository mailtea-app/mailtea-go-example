// Package mailtea is a minimal client for the Mailtea HTTP API.
//
// There is no official Mailtea SDK for Go, so this talks to the REST API
// directly with net/http and encoding/json. No third-party dependencies, and
// small enough to read in one sitting or paste into your own project.
package mailtea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the hosted Mailtea API.
const DefaultBaseURL = "https://api.mailtea.app"

// Options configures a Client. The zero value is what you want in production.
type Options struct {
	// BaseURL points the client at a different Mailtea. Only needed for local
	// dev or a self-hosted instance; empty means DefaultBaseURL, so passing
	// os.Getenv("MAILTEA_API_BASE_URL") is safe when the variable is unset.
	BaseURL string

	// HTTPClient supplies your own transport, proxy, or timeout.
	HTTPClient *http.Client
}

// Client talks to one Mailtea instance with one API key.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New builds a client. The key is an mt_pat_… or mt_svc_… token.
func New(apiKey string, opts Options) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		// net/http's default client has no timeout at all, and a send that
		// hangs forever is worse than one that fails.
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{apiKey: apiKey, baseURL: baseURL, httpClient: httpClient}
}

// Tag is a key/value label carried with a send, for filtering later.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SendEmailRequest is the body of POST /v1/emails. Set From or SenderID (not
// both), and at least one of HTML or Text.
type SendEmailRequest struct {
	// From is a verified sender, e.g. "Acme <hello@acme.com>".
	From string `json:"from,omitempty"`
	// SenderID selects a saved sender ("snd_…") instead of From.
	SenderID string `json:"sender_id,omitempty"`

	To      []string `json:"to"`
	Subject string   `json:"subject"`

	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`

	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	ReplyTo []string `json:"reply_to,omitempty"`

	// ScheduledAt queues the send for later, RFC 3339: "2026-09-01T09:00:00Z".
	ScheduledAt string `json:"scheduled_at,omitempty"`

	Tags    []Tag             `json:"tags,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SendEmailResponse is what POST /v1/emails returns: the id you look the send
// up by, and the id webhooks reference.
type SendEmailResponse struct {
	ID string `json:"id"`
}

// Email is one send, as returned by GET /v1/emails/{id}.
type Email struct {
	Object  string `json:"object"`
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`

	// LastEvent is where the send got to: queued, scheduled, sent, delivered,
	// delivery_delayed, bounced, complained, failed, suppressed, canceled.
	// Polling this is how you check on a send without setting up a webhook.
	LastEvent string `json:"last_event"`

	// Error is the delivery failure reason, when there is one.
	Error string `json:"error"`

	CreatedAt   string `json:"created_at"`
	ScheduledAt string `json:"scheduled_at"`
}

// SendEmail sends one email, or schedules it when ScheduledAt is set.
func (c *Client) SendEmail(ctx context.Context, req SendEmailRequest) (*SendEmailResponse, error) {
	var out SendEmailResponse
	if err := c.do(ctx, http.MethodPost, "/v1/emails", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEmail looks up one send by id. The id goes through url.PathEscape: a real
// "txemail_…" passes through untouched, and an id that came from somewhere less
// trustworthy cannot walk out of the path segment it belongs in.
func (c *Client) GetEmail(ctx context.Context, id string) (*Email, error) {
	var out Email
	if err := c.do(ctx, http.MethodGet, "/v1/emails/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelEmail cancels a send that is still in the "scheduled" state. Anything
// else — including the "queued" of an ordinary immediate send — answers 422, so
// treat that status as "too late to stop it", not as a bug.
func (c *Client) CancelEmail(ctx context.Context, id string) (*Email, error) {
	var out Email
	if err := c.do(ctx, http.MethodPost, "/v1/emails/"+url.PathEscape(id)+"/cancel", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("mailtea: encoding request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return fmt.Errorf("mailtea: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mailtea: %s %s: %w", method, path, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("mailtea: reading %s %s: %w", method, path, err)
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return newAPIError(res.StatusCode, raw)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("mailtea: decoding %s %s: %w", method, path, err)
	}
	return nil
}

// APIError is a non-2xx response. Reach the status code and the API's own
// message with errors.As:
//
//	var apiErr *mailtea.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == 422 {
//		// too late to cancel
//	}
type APIError struct {
	// StatusCode is the HTTP status, e.g. 401, 403, 422.
	StatusCode int
	// Message is the API's "error" field: "Unauthorized", "Validation failed",
	// "Domain not verified", and so on.
	Message string
	// Body is the raw response, which carries the "details" array on a
	// validation failure — the part that says which field was wrong.
	Body string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mailtea: API error %d: %s", e.StatusCode, e.Message)
}

func newAPIError(status int, raw []byte) *APIError {
	apiErr := &APIError{StatusCode: status, Body: string(raw)}

	var parsed struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Error != "" {
		apiErr.Message = parsed.Error
	} else {
		apiErr.Message = strings.TrimSpace(string(raw))
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}

	return apiErr
}

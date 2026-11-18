package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client calls the Dimdam's REST API with an API key (Authorization: Bearer dim_live_…).
type Client struct {
	BaseURL   string
	APIKey    string
	UserAgent string
	HTTP      *http.Client
}

// APIError is a non-2xx answer, or a 2xx answer with "success": false.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("Dimdam's API (HTTP %d) : %s", e.Status, e.Message) }

// IsNotFound reports whether the error means the object no longer exists.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func NewClient(baseURL, apiKey, version string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("adresse du panel invalide : %q", baseURL)
	}
	loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("l'adresse du panel doit être en HTTPS : la clé d'API ne doit jamais circuler en clair")
	}
	return &Client{
		BaseURL:   u.Scheme + "://" + u.Host,
		APIKey:    apiKey,
		UserAgent: "terraform-provider-dimdams/" + version,
		HTTP: &http.Client{
			Timeout: 2 * time.Minute,
			// Never resend the key to another address.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// HostPath is the address of a sub-resource of a service.
func HostPath(hostID, suffix string) string {
	return "/api/hosts/" + url.PathEscape(hostID) + suffix
}

// Do sends a JSON request and decodes the JSON answer into out (when not nil).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("impossible de joindre %s : %w", c.BaseURL, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return &APIError{Status: res.StatusCode, Message: "redirection refusée vers " + res.Header.Get("Location")}
	}
	var envelope struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &envelope)
	if res.StatusCode >= 400 || (envelope.Success != nil && !*envelope.Success) {
		msg := envelope.Error
		if msg == "" {
			msg = envelope.Message
		}
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		status := res.StatusCode
		if status < 400 {
			status = http.StatusBadRequest
		}
		return &APIError{Status: status, Message: msg}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("réponse inattendue du panel : %w", err)
		}
	}
	return nil
}

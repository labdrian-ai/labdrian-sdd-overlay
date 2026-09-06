// Package embed is the only production package in the longterm-mem module
// permitted to import net/http or net (R-071). It talks to a local
// embedding backend (Ollama-shaped: POST /api/embeddings with
// {"model","prompt"}, response carries "embedding") over loopback only,
// unless the caller explicitly opts into a remote endpoint for a single
// invocation.
//
// The refusal rule is deliberately blunt: the endpoint host must be a
// literal loopback IP or the exact string "localhost". Any other hostname
// is refused outright, even if it happens to resolve to loopback today —
// resolving and checking loses to DNS rebinding, and refusing the whole
// class makes the hazard impossible instead of merely handled. A redirect
// is refused for the same reason a warning is not good enough: a boundary
// that can be silently crossed is not a boundary.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// DefaultEndpoint matches the local Ollama default used elsewhere in this
// project's tooling (rerank.py). It is loopback, so it never triggers the
// refusal below.
const DefaultEndpoint = "http://127.0.0.1:11434"

// DefaultTimeout is the explicit per-request timeout used when Config.Timeout
// is not set. A Client is never constructed without a timeout: the zero
// value of http.Client.Timeout means "wait forever," which is not a value
// this package is willing to hand a caller by omission.
const DefaultTimeout = 30 * time.Second

// ErrNonLoopbackEndpoint is returned by NewClient when Endpoint is not a
// literal loopback address or "localhost" and AllowRemote was not set.
var ErrNonLoopbackEndpoint = errors.New("embed: endpoint is not loopback; pass Config.AllowRemote to opt in for this invocation")

// ErrRedirectRefused is returned when the backend responds with an HTTP
// redirect. The client never follows one; refusal happens before the
// redirect target is ever requested.
var ErrRedirectRefused = errors.New("embed: backend attempted a redirect; refused")

// BackendUnreachableError means the embedding backend did not answer at
// all: connection refused, DNS failure, or the request timed out before any
// response arrived. Distinct from ModelMissingError so a caller (R-070) can
// name which condition applies rather than collapsing both into "it failed."
type BackendUnreachableError struct{ Err error }

func (e *BackendUnreachableError) Error() string {
	return fmt.Sprintf("embed: backend unreachable: %v", e.Err)
}

func (e *BackendUnreachableError) Unwrap() error { return e.Err }

// ModelMissingError means the backend answered but the requested model is
// not pulled. Distinct from BackendUnreachableError for the same reason.
type ModelMissingError struct{ Model string }

func (e *ModelMissingError) Error() string {
	return fmt.Sprintf("embed: model %q is not pulled on the backend", e.Model)
}

// Config configures a Client.
type Config struct {
	// Endpoint is the embedding backend's base URL. Defaults to
	// DefaultEndpoint when empty.
	Endpoint string
	// Model is the embedding model name sent to the backend.
	Model string
	// Timeout is the explicit per-request timeout. Defaults to
	// DefaultTimeout when zero or negative.
	Timeout time.Duration
	// AllowRemote opts a single Client construction into a non-loopback
	// Endpoint. It is per-invocation only and is never persisted (e.g. to
	// install-state.json) by this package.
	AllowRemote bool
}

// Client is the embedding backend client. It constructs exactly one
// *http.Client with an explicit timeout and a redirect policy that refuses
// every redirect. It is never exported as an *http.Client, and it never
// accepts one from a caller — both would let a caller route around the
// egress boundary this package exists to hold.
type Client struct {
	endpoint   string
	model      string
	httpClient *http.Client
}

// NewClient validates cfg and constructs a Client. It refuses at
// construction time — not with a warning — when the endpoint is
// non-loopback and AllowRemote was not set.
func NewClient(cfg Config) (*Client, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	if err := checkLoopback(endpoint, cfg.AllowRemote); err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &Client{
		endpoint: endpoint,
		model:    cfg.Model,
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return ErrRedirectRefused
			},
		},
	}, nil
}

// checkLoopback refuses any endpoint host that is not the exact string
// "localhost" or a literal IP for which netip.Addr.IsLoopback() is true.
// It deliberately does not resolve hostnames: resolution can change between
// this check and the dial (DNS rebinding), so the whole class of hostnames
// is refused rather than individually validated.
func checkLoopback(endpoint string, allowRemote bool) error {
	if allowRemote {
		return nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("embed: invalid endpoint %q: %w", endpoint, err)
	}

	host := u.Hostname()
	if host == "localhost" {
		return nil
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		// Not a literal IP at all — refuse outright.
		return ErrNonLoopbackEndpoint
	}
	if !addr.IsLoopback() {
		return ErrNonLoopbackEndpoint
	}
	return nil
}

// Embed requests an embedding vector for text from the configured backend.
// It returns *BackendUnreachableError when the backend does not answer, and
// *ModelMissingError when it answers but the configured model is not
// pulled — the two conditions R-070 requires a caller to name distinctly.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	payload, err := json.Marshal(map[string]string{
		"model":  c.model,
		"prompt": text,
	})
	if err != nil {
		return nil, fmt.Errorf("embed: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("embed: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, ErrRedirectRefused) {
			return nil, ErrRedirectRefused
		}
		return nil, &BackendUnreachableError{Err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &BackendUnreachableError{Err: err}
	}

	if resp.StatusCode == http.StatusNotFound || looksLikeModelMissing(body) {
		return nil, &ModelMissingError{Model: c.model}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &BackendUnreachableError{Err: fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))}
	}

	var parsed struct {
		Embedding []float32 `json:"embedding"`
		Error     string    `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("embed: decoding response: %w", err)
	}
	if parsed.Error != "" {
		if looksLikeModelMissing([]byte(parsed.Error)) {
			return nil, &ModelMissingError{Model: c.model}
		}
		return nil, &BackendUnreachableError{Err: errors.New(parsed.Error)}
	}

	return parsed.Embedding, nil
}

// looksLikeModelMissing matches Ollama's own error text for an unpulled
// model ("model \"x\" not found, try pulling it first"), used both when the
// backend signals it via HTTP status and when it signals it in a 200-shaped
// JSON error field.
func looksLikeModelMissing(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "not found") || strings.Contains(lower, "try pulling")
}

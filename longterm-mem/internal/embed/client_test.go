package embed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewClientRefusesNonLoopbackEndpoint pins R-071: a non-loopback
// endpoint is refused at construction, not warned about, unless the caller
// opts in per invocation via AllowRemote.
func TestNewClientRefusesNonLoopbackEndpoint(t *testing.T) {
	t.Run("literal non-loopback IP is refused", func(t *testing.T) {
		_, err := NewClient(Config{Endpoint: "http://93.184.216.34:11434", Model: "m"})
		if !errors.Is(err, ErrNonLoopbackEndpoint) {
			t.Fatalf("NewClient err = %v, want ErrNonLoopbackEndpoint", err)
		}
	})

	t.Run("AllowRemote lifts the refusal for this invocation", func(t *testing.T) {
		c, err := NewClient(Config{Endpoint: "http://93.184.216.34:11434", Model: "m", AllowRemote: true})
		if err != nil {
			t.Fatalf("NewClient with AllowRemote err = %v, want nil", err)
		}
		if c == nil {
			t.Fatal("NewClient with AllowRemote returned nil client")
		}
	})

	t.Run("default endpoint is loopback and is never refused", func(t *testing.T) {
		c, err := NewClient(Config{Model: "m"})
		if err != nil {
			t.Fatalf("NewClient with default endpoint err = %v, want nil", err)
		}
		if c == nil {
			t.Fatal("NewClient with default endpoint returned nil client")
		}
	})
}

// TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback pins the design
// choice: the endpoint host must be a literal loopback IP or the exact
// string "localhost". Any other hostname is refused even if it would
// resolve to loopback, because resolution can change between the check and
// the dial (DNS rebinding).
func TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback(t *testing.T) {
	_, err := NewClient(Config{Endpoint: "http://loopback.example.test:11434", Model: "m"})
	if !errors.Is(err, ErrNonLoopbackEndpoint) {
		t.Fatalf("NewClient err = %v, want ErrNonLoopbackEndpoint", err)
	}
}

// TestClientRefusesRedirectAndNeverRequestsTheTarget pins that the
// embedding client never follows an HTTP redirect: the redirect target
// handler must never be invoked.
func TestClientRefusesRedirectAndNeverRequestsTheTarget(t *testing.T) {
	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/embeddings", http.StatusFound)
	}))
	defer redirector.Close()

	c, err := NewClient(Config{Endpoint: redirector.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewClient err = %v, want nil", err)
	}

	_, err = c.Embed(context.Background(), "hello world")
	if !errors.Is(err, ErrRedirectRefused) {
		t.Fatalf("Embed err = %v, want ErrRedirectRefused", err)
	}
	if got := atomic.LoadInt32(&targetHits); got != 0 {
		t.Fatalf("target handler invoked %d times, want 0 — the redirect must never be followed", got)
	}
}

// TestClientTimeoutIsExplicit pins that a Client is never constructed
// without a timeout: a backend that never answers must fail within the
// configured budget rather than hanging indefinitely.
func TestClientTimeoutIsExplicit(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	// Close the server before releasing the blocked handler goroutine:
	// httptest.Server.Close waits for active connections to finish, and the
	// handler above will not finish until block is closed. Deferred in this
	// order (close(block) registered after srv.Close) so LIFO runs
	// close(block) first.
	defer srv.Close()
	defer close(block)

	c, err := NewClient(Config{Endpoint: srv.URL, Model: "m", Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewClient err = %v, want nil", err)
	}

	start := time.Now()
	_, err = c.Embed(context.Background(), "hello")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Embed against a server that never answers returned nil error, want a timeout error")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Embed took %s to fail; the configured 20ms timeout was not honored", elapsed)
	}
}

// TestUnreachableBackendAndMissingModelAreDistinctErrors pins R-070's
// query-facing consequence at the client layer: a caller must be able to
// tell "the backend did not answer" apart from "the backend answered but
// the model is not pulled" by error type, not by string matching.
func TestUnreachableBackendAndMissingModelAreDistinctErrors(t *testing.T) {
	t.Run("unreachable backend", func(t *testing.T) {
		// Bind a listener, then close it immediately so the address is
		// guaranteed to refuse connections.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		addr := ln.Addr().String()
		if err := ln.Close(); err != nil {
			t.Fatalf("closing listener: %v", err)
		}

		c, err := NewClient(Config{Endpoint: "http://" + addr, Model: "m", Timeout: time.Second})
		if err != nil {
			t.Fatalf("NewClient err = %v, want nil", err)
		}

		_, embedErr := c.Embed(context.Background(), "hello")
		var unreachable *BackendUnreachableError
		if !errors.As(embedErr, &unreachable) {
			t.Fatalf("Embed err = %v (%T), want *BackendUnreachableError", embedErr, embedErr)
		}
		var missing *ModelMissingError
		if errors.As(embedErr, &missing) {
			t.Fatalf("Embed err classified as ModelMissingError, want BackendUnreachableError only")
		}
	})

	t.Run("model not pulled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"error":"model %q not found, try pulling it first"}`, "some-model")
		}))
		defer srv.Close()

		c, err := NewClient(Config{Endpoint: srv.URL, Model: "some-model"})
		if err != nil {
			t.Fatalf("NewClient err = %v, want nil", err)
		}

		_, embedErr := c.Embed(context.Background(), "hello")
		var missing *ModelMissingError
		if !errors.As(embedErr, &missing) {
			t.Fatalf("Embed err = %v (%T), want *ModelMissingError", embedErr, embedErr)
		}
		var unreachable *BackendUnreachableError
		if errors.As(embedErr, &unreachable) {
			t.Fatalf("Embed err classified as BackendUnreachableError, want ModelMissingError only")
		}
	})
}

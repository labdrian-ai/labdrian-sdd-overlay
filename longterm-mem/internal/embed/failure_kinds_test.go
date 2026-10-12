package embed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// The two failures of a backend say what they are in the terms the query package owns, so that the query tells
// them apart without importing this package: a model that is not pulled, and a backend that did not answer.
var (
	_ query.EmbedModelMissing       = (*ModelMissingError)(nil)
	_ query.EmbedBackendUnreachable = (*BackendUnreachableError)(nil)
)

// What Embed returns from a real exchange is held to it too, through the interfaces and however it is wrapped:
// a 404 is a model that is not pulled, a refused connection is a backend that did not answer, and neither is
// taken for the other.
func TestEmbedFailuresAreToldApartThroughTheQuerysInterfaces(t *testing.T) {
	notPulled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer notPulled.Close()
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	for _, tc := range []struct {
		name             string
		endpoint         string
		wantMissingModel bool
	}{
		{"a model that is not pulled", notPulled.URL, true},
		{"a backend that did not answer", closedURL, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(Config{Endpoint: tc.endpoint, Model: "m"})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, embedErr := client.Embed(context.Background(), "text")
			wrapped := fmt.Errorf("embedding the query: %w", embedErr)

			var missing query.EmbedModelMissing
			var unreachable query.EmbedBackendUnreachable
			if got := errors.As(wrapped, &missing); got != tc.wantMissingModel {
				t.Errorf("a model-missing error: %v, want %v (error: %v)", got, tc.wantMissingModel, embedErr)
			}
			if got := errors.As(wrapped, &unreachable); got == tc.wantMissingModel {
				t.Errorf("an unreachable-backend error: %v, want %v (error: %v)", got, !tc.wantMissingModel, embedErr)
			}
		})
	}
}

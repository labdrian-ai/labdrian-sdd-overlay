package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// The query tool forwards its optional goal to the query (ADR C23 M5, decision 3); a call without one forwards
// none, so it asks what it asked before goals.
func TestServer_QueryForwardsItsGoal(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"with a goal", map[string]any{"project": "p", "query": "q", "goal": "g-42"}, "g-42"},
		{"without a goal", map[string]any{"project": "p", "query": "q"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotReq query.Request
			session := connectInMemory(t, Deps{Query: func(_ context.Context, req query.Request) (query.Result, error) {
				gotReq = req
				return query.Result{Project: req.Project, Query: req.Query}, nil
			}})
			if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "query", Arguments: tc.args}); err != nil {
				t.Fatalf("CallTool(query): %v", err)
			}
			if gotReq.Goal != tc.want || gotReq.Project != "p" || gotReq.Query != "q" {
				t.Fatalf("handler received %+v, want goal %q forwarded with the project and query", gotReq, tc.want)
			}
		})
	}
}

// The query tool lists goal as an optional string property that says what it does; project and query stay the
// only required ones.
func TestServer_QueryToolListsAnOptionalGoal(t *testing.T) {
	session := connectInMemory(t, Deps{})
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "query" {
			continue
		}
		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal the input schema: %v", err)
		}
		var schema struct {
			Properties map[string]struct {
				Type        json.RawMessage `json:"type"`
				Description string          `json:"description"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("decode the input schema: %v", err)
		}
		goal, ok := schema.Properties["goal"]
		if !ok || string(goal.Type) != `"string"` || !strings.Contains(goal.Description, "goal") {
			t.Fatalf("the goal property is %+v (present %v), want a described string", goal, ok)
		}
		for _, r := range schema.Required {
			if r == "goal" {
				t.Fatalf("goal is required: %v", schema.Required)
			}
		}
		return
	}
	t.Fatal("the tool listing has no query tool")
}

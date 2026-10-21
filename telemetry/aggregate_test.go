package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func cost(v float64) *float64 { return &v }

func TestTotals(t *testing.T) {
	var a Aggregator
	a.Add(Call{Input: 2, CacheRead: 3, CacheWrite: 5, Output: 7, Reasoning: 11})
	a.Add(Call{Input: 13, CacheRead: 17, CacheWrite: 19, Output: 23, Reasoning: 29})
	g := a.Report().Summary
	got := []int64{g.Calls, g.Input, g.CacheRead, g.CacheWrite, g.Output, g.Reasoning, g.ContextTokens}
	if want := []int64{2, 15, 20, 24, 30, 40, 59}; !reflect.DeepEqual(got, want) || g.Key != "all" {
		t.Fatalf("summary = %+v; totals = %v, want %v and key all", g, got, want)
	}
}

func TestRatios(t *testing.T) {
	for _, tt := range []struct {
		name string
		get  func(Totals) *float64
		want float64
	}{
		{"cache_read_share", func(v Totals) *float64 { return v.CacheReadShare }, 0.6},
		{"context_per_output", func(v Totals) *float64 { return v.ContextPerOutput }, 5},
		{"context_per_call", func(v Totals) *float64 { return v.ContextPerCall }, 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var a Aggregator
			if got := tt.get(a.Report().Summary.Totals); got != nil {
				t.Fatalf("empty ratio = %v, want nil", *got)
			}
			a.Add(Call{Input: 3, CacheRead: 6, CacheWrite: 1, Output: 2})
			if got := tt.get(a.Report().Summary.Totals); got == nil || *got != tt.want {
				t.Fatalf("ratio = %v, want %v", got, tt.want)
			}
		})
	}
	var a Aggregator
	a.Add(Call{})
	g := a.Report().Summary
	if g.CacheReadShare != nil || g.ContextPerOutput != nil || g.ContextPerCall == nil || *g.ContextPerCall != 0 {
		t.Fatalf("zero-token call ratios = %+v", g.Totals)
	}
}

func TestPercentiles(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want Distribution
	}{
		{0, Distribution{}},
		{1, Distribution{1, 1, 1, 1, 1}},
		{2, Distribution{1, 1, 1, 2, 2}},
		{5, Distribution{1, 2, 3, 5, 5}},
		{10, Distribution{1, 3, 5, 9, 10}},
		{100, Distribution{5, 25, 50, 90, 100}},
	} {
		t.Run(fmt.Sprint(tt.n), func(t *testing.T) {
			var a Aggregator
			for i := tt.n; i > 0; i-- {
				a.Add(Call{Input: int64(i)})
			}
			got := a.Report().Summary.ContextPerCallDistribution
			if tt.n == 0 {
				if got != nil {
					t.Fatalf("empty distribution = %+v, want nil", got)
				}
			} else if got == nil || *got != tt.want {
				t.Fatalf("distribution = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGrouping(t *testing.T) {
	var a Aggregator
	for _, c := range []Call{
		{Source: "pi", Role: "sub", Model: "z", Project: "z", Day: "2026-10-22", AgentType: "writer", Input: 2},
		{Source: "claude", Role: "main", Model: "a", Project: "a", Day: "2026-10-21", Input: 1},
		{Source: "pi", Role: "sub", Model: "z", Project: "z", Day: "2026-10-22", AgentType: "writer", Input: 3},
	} {
		a.Add(c)
	}
	r := a.Report()
	for _, tt := range []struct {
		name string
		got  []Group
		keys []string
	}{
		{"source", r.BySource, []string{"claude", "pi"}},
		{"role", r.ByRole, []string{"main", "sub"}},
		{"model", r.ByModel, []string{"a", "z"}},
		{"project", r.ByProject, []string{"a", "z"}},
		{"day", r.ByDay, []string{"2026-10-21", "2026-10-22"}},
		{"agent_type", r.ByAgentType, []string{"(none)", "writer"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.got) != 2 {
				t.Fatalf("groups = %+v, want two", tt.got)
			}
			for i, g := range tt.got {
				wantCalls, wantContext := int64(1+i), int64(1+4*i)
				wantDistribution := []Distribution{{1, 1, 1, 1, 1}, {2, 2, 2, 3, 3}}[i]
				if g.Key != tt.keys[i] || g.Calls != wantCalls || g.ContextTokens != wantContext || g.ContextPerCallDistribution == nil || *g.ContextPerCallDistribution != wantDistribution {
					t.Fatalf("group %d = %+v", i, g)
				}
			}
		})
	}
	var b Aggregator
	b.Add(Call{Role: "main", AgentType: "explicit"})
	if groups := b.Report().ByAgentType; len(groups) != 1 || groups[0].Key != "explicit" {
		t.Fatalf("main call's supplied agent type lost: %+v", groups)
	}
}

func TestCosts(t *testing.T) {
	for _, tt := range []struct {
		name  string
		costs []*float64
		want  *float64
		known int64
	}{
		{"all_known", []*float64{cost(1.25), cost(2.5), cost(0)}, cost(3.75), 3},
		{"all_unknown", []*float64{nil, nil}, nil, 0},
		{"mixed", []*float64{cost(1.25), nil, cost(2.5)}, cost(3.75), 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var a Aggregator
			for _, v := range tt.costs {
				a.Add(Call{CostUSD: v})
			}
			g := a.Report().Summary
			if g.Calls != int64(len(tt.costs)) || g.CostCalls != tt.known || !reflect.DeepEqual(g.CostUSD, tt.want) {
				t.Fatalf("cost totals = %+v, want cost %v / coverage %d", g.Totals, tt.want, tt.known)
			}
		})
	}
}

func TestInsertionOrder(t *testing.T) {
	calls := []Call{
		{Source: "pi", Model: "z", Input: 9, CostUSD: cost(1e16)},
		{Source: "claude", Model: "a", Input: 2, CostUSD: cost(1)},
		{Source: "pi", Model: "a", Input: 3, CostUSD: cost(1)},
		{Source: "pi", Model: "a", Input: 4},
	}
	rng := rand.New(rand.NewSource(42))
	var want []byte
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(calls), func(i, j int) { calls[i], calls[j] = calls[j], calls[i] })
		var a Aggregator
		for _, c := range calls {
			a.Add(c)
		}
		r := a.Report()
		if r.Summary.Calls != int64(len(calls)) {
			t.Fatalf("calls = %d, want %d", r.Summary.Calls, len(calls))
		}
		got, err := r.JSON()
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && !bytes.Equal(got, want) {
			t.Fatal("JSON changed with insertion order")
		}
		want = got
	}
}

func TestJSON(t *testing.T) {
	var a Aggregator
	got, err := a.Report().JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"{\n  \"summary\": {", "\n}\n", `"key": "all"`,
		`"cache_read_share": null`, `"context_per_output": null`, `"context_per_call": null`,
		`"cost_usd": null`, `"cost_calls": 0`, `"context_tokens": 0`,
		`"context_per_call_distribution": null`, `"by_source": []`, `"by_role": []`,
		`"by_model": []`, `"by_project": []`, `"by_day": []`, `"by_agent_type": []`,
		`"cache_read": 0`, `"cache_write": 0`, `"calls": 0`, `"input": 0`, `"output": 0`, `"reasoning": 0`,
	} {
		if !bytes.Contains(got, []byte(fragment)) {
			t.Errorf("JSON missing %q: %s", fragment, got)
		}
	}
	if !json.Valid(got) {
		t.Fatal("invalid JSON")
	}
}

func parsedPrices(t *testing.T) PriceTable {
	t.Helper()
	p, err := ParsePrices([]byte(validPrices))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEstimateArithmetic(t *testing.T) {
	a := NewAggregator(parsedPrices(t))
	a.Add(Call{Model: "exact", Input: 1e6, CacheRead: 2e6, CacheWrite: 3e6, Output: 4e6, Reasoning: 99e6})
	r := a.Report()
	for _, groups := range [][]Group{{r.Summary}, r.BySource, r.ByRole, r.ByModel, r.ByProject, r.ByDay, r.ByAgentType} {
		for _, g := range groups {
			if g.EstimatedCostUSD == nil || *g.EstimatedCostUSD != 30 || g.EstimatedCalls != 1 || g.CostUSD != nil || g.CostCalls != 0 {
				t.Fatalf("estimate = %+v, want $30 estimated only", g)
			}
		}
	}
	b, err := r.JSON()
	if err != nil || !bytes.Contains(b, []byte(`"estimated_cost_usd": 30`)) || !bytes.Contains(b, []byte(`"estimated_calls": 1`)) {
		t.Fatalf("estimated JSON: %s %v", b, err)
	}
}

func TestEstimateExactMatchOnly(t *testing.T) {
	for _, model := range []string{"exact", "exact-prefix", "exac", "EXACT", "unknown", ""} {
		t.Run(model, func(t *testing.T) {
			a := NewAggregator(parsedPrices(t))
			a.Add(Call{Model: model, Input: 1e6})
			g := a.Report().Summary
			b, err := a.Report().JSON()
			if model == "exact" {
				if g.EstimatedCalls != 1 || g.EstimatedCostUSD == nil || *g.EstimatedCostUSD != 1 {
					t.Fatalf("matching control unpriced: %+v", g)
				}
			} else if g.Calls != 1 || g.CostCalls != 0 || g.CostUSD != nil || g.EstimatedCalls != 0 || g.EstimatedCostUSD != nil || err != nil || bytes.Contains(b, []byte("estimated_")) {
				t.Fatalf("unmatched model priced: %+v %s %v", g, b, err)
			}
		})
	}
}

func TestRecordedCostNeverEstimated(t *testing.T) {
	a := NewAggregator(parsedPrices(t))
	a.Add(Call{Model: "exact", Input: 1e6, CostUSD: cost(7)})
	a.Add(Call{Model: "exact", Input: 1e6, CostUSD: cost(0)})
	a.Add(Call{Model: "exact", Input: 1e6})
	g := a.Report().Summary
	if g.CostUSD == nil || *g.CostUSD != 7 || g.CostCalls != 2 || g.EstimatedCalls != 1 || g.EstimatedCostUSD == nil || *g.EstimatedCostUSD != 1 {
		t.Fatalf("recorded calls re-estimated: %+v", g)
	}
}

func TestEstimatedInsertionOrder(t *testing.T) {
	prices, err := ParsePrices([]byte(`{"unit":"usd_per_million_tokens","models":{"exact":{"input":1000000,"cache_read":0,"cache_write":0,"output":0}}}`))
	if err != nil {
		t.Fatal(err)
	}
	// Estimates 1e16, 1, 1 exercise order-sensitive floating-point addition.
	calls := []Call{{Model: "exact", Input: 10000000000000000}, {Model: "exact", Input: 1}, {Model: "exact", Input: 1}, {Model: "unknown", Input: 2}, {Model: "exact", CostUSD: cost(3)}}
	rng := rand.New(rand.NewSource(42))
	var want []byte
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(calls), func(i, j int) { calls[i], calls[j] = calls[j], calls[i] })
		a := NewAggregator(prices)
		for _, c := range calls {
			a.Add(c)
		}
		r := a.Report()
		if r.Summary.EstimatedCalls != 3 || r.Summary.EstimatedCostUSD == nil {
			t.Fatalf("missing estimates: %+v", r.Summary)
		}
		got, err := r.JSON()
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && !bytes.Equal(want, got) {
			t.Fatal("estimated JSON changed with insertion order")
		}
		want = got
	}
}

func TestNoPricesOmitsEstimates(t *testing.T) {
	var a Aggregator
	a.Add(Call{Model: "exact", Input: 1e6, CostUSD: cost(2)})
	a.Add(Call{Model: "exact", Input: 1e6})
	got, err := a.Report().JSON()
	if err != nil || bytes.Contains(got, []byte("estimated_")) {
		t.Fatalf("no-price JSON changed: %s %v", got, err)
	}
}

func TestAddCopiesCost(t *testing.T) {
	var a Aggregator
	c := Call{CostUSD: cost(2)}
	a.Add(c)
	*c.CostUSD = 99
	if got := a.Report().Summary.CostUSD; got == nil || *got != 2 {
		t.Fatalf("stored cost = %v, want 2", got)
	}
}

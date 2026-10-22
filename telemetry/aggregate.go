package telemetry

import (
	"encoding/json"
	"sort"
)

// Totals contains recorded usage and derived context metrics. CostCalls exposes
// cost coverage: CostUSD is nil only when no call carried a recorded cost.
type Totals struct {
	Calls            int64    `json:"calls"`
	Input            int64    `json:"input"`
	CacheRead        int64    `json:"cache_read"`
	CacheWrite       int64    `json:"cache_write"`
	Output           int64    `json:"output"`
	Reasoning        int64    `json:"reasoning"`
	ContextTokens    int64    `json:"context_tokens"`
	CostUSD          *float64 `json:"cost_usd"`
	CostCalls        int64    `json:"cost_calls"`
	EstimatedCostUSD *float64 `json:"estimated_cost_usd,omitempty"`
	EstimatedCalls   int64    `json:"estimated_calls,omitempty"`
	CacheReadShare   *float64 `json:"cache_read_share"`
	ContextPerOutput *float64 `json:"context_per_output"`
	ContextPerCall   *float64 `json:"context_per_call"`
}

// Distribution describes per-call context token counts.
type Distribution struct {
	P5  int64 `json:"p5"`
	P25 int64 `json:"p25"`
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	Max int64 `json:"max"`
}

// Group is one dimension key and its aggregate usage.
type Group struct {
	Key string `json:"key"`
	Totals
	ContextPerCallDistribution *Distribution `json:"context_per_call_distribution"`
}

// Report contains only aggregate usage, with dimension slices sorted by key.
type Report struct {
	Summary     Group   `json:"summary"`
	BySource    []Group `json:"by_source"`
	ByRole      []Group `json:"by_role"`
	ByModel     []Group `json:"by_model"`
	ByProject   []Group `json:"by_project"`
	ByDay       []Group `json:"by_day"`
	ByAgentType []Group `json:"by_agent_type"`
}

// JSON returns indented JSON terminated by a newline.
func (r Report) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// The grouping dimensions index Aggregator.groups, the keys Add builds and the
// Report slices it fills. All three spell them out by name, so reordering this
// list cannot silently pair a key with the wrong slice.
const (
	bySource = iota
	byRole
	byModel
	byProject
	byDay
	byAgentType
	dimensions // the number of dimensions; keep last
)

// Aggregator accumulates calls; its zero value is ready to use. Retained
// per-call context and cost values require O(calls) memory across dimensions.
type Aggregator struct {
	prices  PriceTable
	summary accumulator
	groups  [dimensions]map[string]*accumulator
}

type accumulator struct {
	totals    Totals
	contexts  []int64
	costs     []float64
	estimates []float64
}

// NewAggregator creates an aggregator with optional user-supplied prices.
// PriceTable is immutable after parsing; no caller-owned mutable data is kept.
func NewAggregator(prices PriceTable) *Aggregator { return &Aggregator{prices: prices} }

func (a *accumulator) add(c Call, estimate *float64) {
	t := &a.totals
	t.Calls++
	t.Input += c.Input
	t.CacheRead += c.CacheRead
	t.CacheWrite += c.CacheWrite
	t.Output += c.Output
	t.Reasoning += c.Reasoning
	context := c.Input + c.CacheRead + c.CacheWrite
	t.ContextTokens += context
	a.contexts = append(a.contexts, context)
	if c.CostUSD != nil {
		a.costs = append(a.costs, *c.CostUSD)
		t.CostCalls++
	}
	if estimate != nil {
		a.estimates = append(a.estimates, *estimate)
		t.EstimatedCalls++
	}
}

// Add records a call without retaining caller-owned pointers. Empty agent types
// are grouped as "(none)"; supplied types are kept regardless of role.
func (a *Aggregator) Add(c Call) {
	estimate := a.prices.estimate(c)
	a.summary.add(c, estimate)
	agent := c.AgentType
	if agent == "" {
		agent = "(none)"
	}
	keys := [dimensions]string{bySource: c.Source, byRole: c.Role, byModel: c.Model, byProject: c.Project, byDay: c.Day, byAgentType: agent}
	for i, key := range keys {
		if a.groups[i] == nil {
			a.groups[i] = make(map[string]*accumulator)
		}
		if a.groups[i][key] == nil {
			a.groups[i][key] = &accumulator{}
		}
		a.groups[i][key].add(c, estimate)
	}
}

func ratio(numerator, denominator int64) *float64 {
	if denominator == 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

// canonicalCost sums in a canonical order because floating-point addition is
// order-sensitive. Recorded and estimated costs use the same invariant.
func canonicalCost(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	costs := append([]float64(nil), values...)
	sort.Float64s(costs)
	var sum float64
	for _, cost := range costs {
		sum += cost
	}
	return &sum
}

func (a *accumulator) group(key string) Group {
	t := a.totals
	t.CacheReadShare = ratio(t.CacheRead, t.ContextTokens)
	t.ContextPerOutput = ratio(t.ContextTokens, t.Output)
	t.ContextPerCall = ratio(t.ContextTokens, t.Calls)
	t.CostUSD = canonicalCost(a.costs)
	t.EstimatedCostUSD = canonicalCost(a.estimates)
	g := Group{Key: key, Totals: t}
	if len(a.contexts) > 0 {
		contexts := append([]int64(nil), a.contexts...)
		sort.Slice(contexts, func(i, j int) bool { return contexts[i] < contexts[j] })
		// Nearest rank: ceil(p/100 * n), 1-based, on sorted per-call contexts.
		percentile := func(p int) int64 { return contexts[(p*len(contexts)+99)/100-1] }
		g.ContextPerCallDistribution = &Distribution{
			P5: percentile(5), P25: percentile(25), P50: percentile(50),
			P90: percentile(90), Max: contexts[len(contexts)-1],
		}
	}
	return g
}

// Report returns an independent snapshot of the accumulated usage.
func (a *Aggregator) Report() Report {
	r := Report{Summary: a.summary.group("all")}
	outputs := [dimensions]*[]Group{bySource: &r.BySource, byRole: &r.ByRole, byModel: &r.ByModel, byProject: &r.ByProject, byDay: &r.ByDay, byAgentType: &r.ByAgentType}
	for i, groups := range a.groups {
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		*outputs[i] = make([]Group, 0, len(keys))
		for _, key := range keys {
			*outputs[i] = append(*outputs[i], groups[key].group(key))
		}
	}
	return r
}

package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPrices = `{"unit":"usd_per_million_tokens","models":{"exact":{"input":1,"cache_read":2,"cache_write":3,"output":4}}}`

func TestParsePricesValid(t *testing.T) {
	p, err := ParsePrices([]byte(validPrices))
	if err != nil || p.Len() != 1 {
		t.Fatalf("prices=%+v err=%v", p, err)
	}
}

func TestParsePricesRejects(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"wrong_unit", strings.Replace(validPrices, "usd_per_million_tokens", "usd", 1)},
		{"missing_unit", `{"models":{"exact":{"input":1,"cache_read":2,"cache_write":3,"output":4}}}`},
		{"missing_models", `{"unit":"usd_per_million_tokens"}`},
		{"negative", strings.Replace(validPrices, `"input":1`, `"input":-1`, 1)},
		{"NaN", strings.Replace(validPrices, `"input":1`, `"input":NaN`, 1)},
		{"Inf", strings.Replace(validPrices, `"input":1`, `"input":Infinity`, 1)},
		{"overflow", strings.Replace(validPrices, `"input":1`, `"input":1e999`, 1)},
		{"unknown_top", strings.Replace(validPrices, `"unit":`, `"comment":"fake","unit":`, 1)},
		{"unknown_rate", strings.Replace(validPrices, `"input":1`, `"extra":0,"input":1`, 1)},
		{"case_variant_field", strings.Replace(validPrices, `"input":1`, `"Input":1`, 1)},
		{"trailing", validPrices + `{}`},
		{"trailing_invalid", validPrices + `garbage`},
		{"empty_models", `{"unit":"usd_per_million_tokens","models":{}}`},
		{"null_models", `{"unit":"usd_per_million_tokens","models":null}`},
		{"duplicate_model", strings.Replace(validPrices, `"exact":`, `"exact":{},"exact":`, 1)},
		{"duplicate_rate", strings.Replace(validPrices, `"input":1`, `"input":0,"input":1`, 1)},
		{"duplicate_top", strings.Replace(validPrices, `"unit":`, `"unit":"usd_per_million_tokens","unit":`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParsePrices([]byte(tc.data))
			if err == nil || p.Len() != 0 {
				t.Fatalf("accepted invalid table: %+v err=%v", p, err)
			}
		})
	}
	for _, field := range []string{"input", "cache_read", "cache_write", "output"} {
		for _, replacement := range []string{"missing", "null"} {
			t.Run(field+"_"+replacement, func(t *testing.T) {
				value := map[string]string{"input": "1", "cache_read": "2", "cache_write": "3", "output": "4"}[field]
				old, next := `"`+field+`":`+value, `"`+field+`":null`
				if replacement == "missing" {
					next = ""
					if field == "output" {
						old = "," + old
					} else {
						old += ","
					}
				}
				if p, err := ParsePrices([]byte(strings.Replace(validPrices, old, next, 1))); err == nil || p.Len() != 0 {
					t.Fatalf("accepted %s %s", replacement, field)
				}
			})
		}
	}
}

func TestLoadPricesReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(path, []byte(validPrices), 0444); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPrices(path)
	if err != nil || p.Len() != 1 {
		t.Fatalf("prices=%+v err=%v", p, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != validPrices {
		t.Fatalf("price file changed: %q %v", after, err)
	}
	if p, err := LoadPrices(path + ".missing"); err == nil || p.Len() != 0 {
		t.Fatal("missing file accepted")
	}
}

func TestParsePricesZeroRates(t *testing.T) {
	p, err := ParsePrices([]byte(`{"unit":"usd_per_million_tokens","models":{"exact":{"input":0,"cache_read":0,"cache_write":0,"output":0}}}`))
	if err != nil {
		t.Fatal(err)
	}
	a := NewAggregator(p)
	a.Add(Call{Model: "exact", Input: 100})
	g := a.Report().Summary
	if g.EstimatedCalls != 1 || g.EstimatedCostUSD == nil || *g.EstimatedCostUSD != 0 {
		t.Fatalf("zero estimate lost: %+v", g)
	}
}

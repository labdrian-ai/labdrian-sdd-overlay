package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
)

// PriceTable contains validated, user-supplied per-million-token prices.
// Its zero value contains no prices; model IDs are matched exactly.
type PriceTable struct{ models map[string]modelPrice }

type modelPrice struct{ input, cacheRead, cacheWrite, output float64 }

// Len returns the number of exact model IDs in the table.
func (p PriceTable) Len() int { return len(p.models) }

// LoadPrices reads and validates a user-selected price table, without writing it.
func LoadPrices(path string) (PriceTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PriceTable{}, err
	}
	return ParsePrices(data)
}

// ParsePrices validates a price table without accessing the filesystem.
// All fields are required, case-sensitive, and unique, including model IDs.
func ParsePrices(data []byte) (PriceTable, error) {
	fail := func(err error) (PriceTable, error) { return PriceTable{}, err }
	doc, err := priceObject(data, "unit", "models")
	if err != nil {
		return fail(err)
	}
	var unit string
	if json.Unmarshal(doc["unit"], &unit) != nil || unit != "usd_per_million_tokens" {
		return fail(fmt.Errorf("unit must be usd_per_million_tokens"))
	}
	models, err := priceObject(doc["models"])
	if err != nil {
		return fail(fmt.Errorf("models: %w", err))
	}
	if len(models) == 0 {
		return fail(fmt.Errorf("models must not be empty"))
	}
	table := PriceTable{models: make(map[string]modelPrice, len(models))}
	modelIDs := make([]string, 0, len(models))
	for model := range models {
		modelIDs = append(modelIDs, model)
	}
	sort.Strings(modelIDs)
	for _, model := range modelIDs {
		fields, err := priceObject(models[model], "input", "cache_read", "cache_write", "output")
		if err != nil {
			return fail(fmt.Errorf("model %q: %w", model, err))
		}
		var price modelPrice
		// Fixed field order also makes validation errors deterministic.
		for _, field := range []struct {
			name  string
			value *float64
		}{
			{"input", &price.input}, {"cache_read", &price.cacheRead},
			{"cache_write", &price.cacheWrite}, {"output", &price.output},
		} {
			var value *float64
			err := json.Unmarshal(fields[field.name], &value)
			if err != nil || value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
				return fail(fmt.Errorf("model %q: %s must be a finite nonnegative number", model, field.name))
			}
			*field.value = *value
		}
		table.models[model] = price
	}
	return table, nil
}

// priceObject rejects duplicate keys before decoding can silently overwrite
// them. With allowed fields supplied it also rejects unknown or missing keys.
func priceObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, fmt.Errorf("expected object")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("expected field name")
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate field %q", key)
		}
		if len(allowed) > 0 {
			known := false
			for _, name := range allowed {
				known = known || key == name
			}
			if !known {
				return nil, fmt.Errorf("unknown field %q", key)
			}
		}
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, err
		}
		fields[key] = raw
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if err := d.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, fmt.Errorf("trailing data")
	}
	for _, name := range allowed {
		if _, ok := fields[name]; !ok {
			return nil, fmt.Errorf("missing field %q", name)
		}
	}
	return fields, nil
}

func (p PriceTable) estimate(c Call) *float64 {
	if c.CostUSD != nil {
		return nil
	}
	rate, ok := p.models[c.Model]
	if !ok {
		return nil
	}
	// Reasoning tokens are already included in Output, never added again.
	value := (float64(c.Input)*rate.input + float64(c.CacheRead)*rate.cacheRead + float64(c.CacheWrite)*rate.cacheWrite + float64(c.Output)*rate.output) / 1e6
	return &value
}

package main

import (
	"strings"
	"testing"
)

// TestHeaderShowsOuroborosLogo locks in the ouroboros hero logo + wordmark
// so a future header refactor can't silently drop the brand.
func TestHeaderShowsOuroborosLogo(t *testing.T) {
	out := logoBanner(80)
	if !strings.Contains(out, "labdrian") {
		t.Error("logoBanner must contain the 'labdrian' wordmark")
	}
	if !strings.Contains(out, "⣿") {
		t.Error("logoBanner must render the Braille ouroboros art")
	}
	if len(ouroborosArt) != 14 {
		t.Errorf("ouroborosArt: expected 14 rows, got %d", len(ouroborosArt))
	}
}

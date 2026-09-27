package roles

import "testing"

func TestVocabularyCanonicalOrder(t *testing.T) {
	want := []Role{
		RolePrototyper,
		RoleShaper,
		RoleEstimator,
		RoleBuilder,
		RoleSweeper,
		RolePolisher,
		RoleReviewer,
		RoleDelivery,
	}
	if len(Vocabulary) != len(want) {
		t.Fatalf("Vocabulary length = %d, want %d", len(Vocabulary), len(want))
	}
	for i, r := range want {
		if Vocabulary[i] != r {
			t.Fatalf("Vocabulary[%d] = %q, want %q", i, Vocabulary[i], r)
		}
	}
}

func TestCatalogCoversEveryRoleWithData(t *testing.T) {
	if len(Catalog) != len(Vocabulary) {
		t.Fatalf("Catalog length = %d, want %d", len(Catalog), len(Vocabulary))
	}
	seen := make(map[Role]bool, len(Catalog))
	for _, info := range Catalog {
		if info.Purpose == "" {
			t.Fatalf("role %q has blank Purpose", info.Role)
		}
		if info.Consumes == "" {
			t.Fatalf("role %q has blank Consumes", info.Role)
		}
		if info.Produces == "" {
			t.Fatalf("role %q has blank Produces", info.Role)
		}
		seen[info.Role] = true
	}
	for _, r := range Vocabulary {
		if !seen[r] {
			t.Fatalf("Catalog is missing role %q", r)
		}
	}
}

func TestIsKnownRole(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{"prototyper known", RolePrototyper, true},
		{"delivery known", RoleDelivery, true},
		{"unknown role rejected", Role("astronaut"), false},
		{"empty role rejected", Role(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownRole(tt.role); got != tt.want {
				t.Fatalf("IsKnownRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name string
		from Role
		to   Role
		want bool
	}{
		{"prototyper to shaper", RolePrototyper, RoleShaper, true},
		{"shaper to estimator", RoleShaper, RoleEstimator, true},
		{"estimator to builder", RoleEstimator, RoleBuilder, true},
		{"builder to sweeper", RoleBuilder, RoleSweeper, true},
		{"builder skips sweeper to polisher", RoleBuilder, RolePolisher, true},
		{"builder skips both to reviewer", RoleBuilder, RoleReviewer, true},
		{"sweeper to polisher", RoleSweeper, RolePolisher, true},
		{"sweeper skips polisher to reviewer", RoleSweeper, RoleReviewer, true},
		{"polisher to reviewer", RolePolisher, RoleReviewer, true},
		{"reviewer to delivery", RoleReviewer, RoleDelivery, true},
		{"reviewer back to builder is the only backward transition", RoleReviewer, RoleBuilder, true},
		{"builder back to estimator is refused", RoleBuilder, RoleEstimator, false},
		{"reviewer back to shaper is refused", RoleReviewer, RoleShaper, false},
		{"prototyper cannot skip shaper to estimator", RolePrototyper, RoleEstimator, false},
		{"delivery is terminal: nothing follows it", RoleDelivery, RoleReviewer, false},
		{"delivery cannot restart the chain", RoleDelivery, RolePrototyper, false},
		{"self transition is refused", RoleBuilder, RoleBuilder, false},
		{"unknown from role refused", Role("astronaut"), RoleShaper, false},
		{"unknown to role refused", RoleShaper, Role("astronaut"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("IsValidTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestNextRoles(t *testing.T) {
	tests := []struct {
		name string
		from Role
		want []Role
	}{
		{"builder allows sweeper, polisher, or reviewer", RoleBuilder, []Role{RoleSweeper, RolePolisher, RoleReviewer}},
		{"reviewer allows delivery or rework to builder", RoleReviewer, []Role{RoleBuilder, RoleDelivery}},
		{"delivery is terminal", RoleDelivery, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextRoles(tt.from)
			if len(got) != len(tt.want) {
				t.Fatalf("NextRoles(%q) = %v, want %v", tt.from, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("NextRoles(%q) = %v, want %v", tt.from, got, tt.want)
				}
			}
		})
	}
}

func TestEmptyChainDigestIsSixtyFourZeros(t *testing.T) {
	if len(EmptyChainDigest) != 64 {
		t.Fatalf("EmptyChainDigest length = %d, want 64", len(EmptyChainDigest))
	}
	for _, c := range EmptyChainDigest {
		if c != '0' {
			t.Fatalf("EmptyChainDigest = %q, want all zeros", EmptyChainDigest)
		}
	}
}

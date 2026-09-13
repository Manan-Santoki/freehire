package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/search/search"
)

// The auto_apply_available search facet (internal/search/search's
// AutoApplyProviders) is a manually-synced mirror of this package's own
// fillProviders ∪ browserUseProviders — search (layer 6) cannot import
// atsapply (layer 8), so the facet's copy cannot be verified from the
// search package itself. This test closes that gap from the side that CAN
// see both: atsapply (layer 8) is free to import search (layer 6). Without
// it, an edit to fillProviders or browserUseProviders with no matching edit
// in internal/search/search/document.go passes every check in both
// packages and the facet silently mis-reports eligibility.
func TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps(t *testing.T) {
	want := make(map[string]bool, len(fillProviders)+len(browserUseProviders))
	for p := range fillProviders {
		want[p] = true
	}
	for p := range browserUseProviders {
		want[p] = true
	}

	got := search.AutoApplyProviders
	if len(got) != len(want) {
		t.Fatalf("search.AutoApplyProviders = %v, want exactly %v (fillProviders ∪ browserUseProviders)", got, want)
	}
	for p := range want {
		if !got[p] {
			t.Errorf("search.AutoApplyProviders missing %q, present in atsapply's own fillProviders/browserUseProviders", p)
		}
	}
}

package search

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// TestAutoApplyProviders_ExactExpectedSet is a local snapshot check, not a
// cross-package one: it cannot see atsapply's real fillProviders/
// browserUseProviders (search, layer 6, cannot import atsapply, in api,
// layer 8), so it only catches an accidental same-PR edit to this literal,
// not atsapply's own maps drifting away from it unnoticed.
// TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps in
// internal/api/atsapply is what closes that real gap, from the side that
// can see both.
func TestAutoApplyProviders_ExactExpectedSet(t *testing.T) {
	want := map[string]bool{
		"greenhouse": true,
		"lever":      true,
		"ashby":      true,
		"workable":   true,
	}
	if len(AutoApplyProviders) != len(want) {
		t.Fatalf("AutoApplyProviders = %v, want exactly %v", AutoApplyProviders, want)
	}
	for provider := range want {
		if !AutoApplyProviders[provider] {
			t.Errorf("AutoApplyProviders missing expected provider %q", provider)
		}
	}
}

func TestFromJob_AutoApplyAvailable(t *testing.T) {
	for _, provider := range []string{"greenhouse", "lever", "ashby", "workable"} {
		doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: provider})
		if err != nil {
			t.Fatalf("FromJob(%q): %v", provider, err)
		}
		if !doc.AutoApplyAvailable {
			t.Errorf("source %q: AutoApplyAvailable = false, want true", provider)
		}
	}
}

func TestFromJob_AutoApplyAvailable_NotEligibleProvider(t *testing.T) {
	for _, provider := range []string{"recruitee", "djinni", ""} {
		doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: provider})
		if err != nil {
			t.Fatalf("FromJob(%q): %v", provider, err)
		}
		if doc.AutoApplyAvailable {
			t.Errorf("source %q: AutoApplyAvailable = true, want false", provider)
		}
	}
}

// marshalToMap round-trips a JobDocument through JSON into a generic map, so a
// test can assert a key's presence (or absence) rather than just its value —
// `omitempty` drops the key entirely, which a typed struct field can't observe.
func marshalToMap(t *testing.T, doc JobDocument) map[string]any {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return raw
}

func TestFromJob_AutoApplyAvailable_OmittedFromJSONWhenFalse(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "recruitee"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if _, present := marshalToMap(t, doc)["auto_apply_available"]; present {
		t.Error("auto_apply_available key present in JSON when false, want omitted")
	}
}

func TestFromJob_AutoApplyAvailable_PresentInJSONWhenTrue(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "greenhouse"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if v, present := marshalToMap(t, doc)["auto_apply_available"]; !present || v != true {
		t.Errorf("auto_apply_available = %v, present=%v, want true, present=true", v, present)
	}
}

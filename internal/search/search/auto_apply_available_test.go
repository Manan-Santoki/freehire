package search

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// TestAutoApplyProviders_ExactExpectedSet guards against silent drift from
// atsapply's own fillProviders/browserUseProviders, which this package
// cannot import (search, layer 6, sits below api, layer 8). A future change
// to either side must edit this list by hand, not fall out of sync silently.
func TestAutoApplyProviders_ExactExpectedSet(t *testing.T) {
	want := map[string]bool{
		"greenhouse": true,
		"lever":      true,
		"ashby":      true,
		"workable":   true,
	}
	if len(autoApplyProviders) != len(want) {
		t.Fatalf("autoApplyProviders = %v, want exactly %v", autoApplyProviders, want)
	}
	for provider := range want {
		if !autoApplyProviders[provider] {
			t.Errorf("autoApplyProviders missing expected provider %q", provider)
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

func TestFromJob_AutoApplyAvailable_OmittedFromJSONWhenFalse(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "recruitee"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, present := raw["auto_apply_available"]; present {
		t.Errorf("auto_apply_available key present in JSON when false, want omitted: %s", b)
	}
}

func TestFromJob_AutoApplyAvailable_PresentInJSONWhenTrue(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "greenhouse"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v, present := raw["auto_apply_available"]; !present || v != true {
		t.Errorf("auto_apply_available = %v, present=%v, want true, present=true", v, present)
	}
}

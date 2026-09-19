package cv

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func TestSeedMapsStructured(t *testing.T) {
	s := resumeextract.Structured{
		FullName: "Ada Lovelace",
		Headline: "Backend Engineer",
		Location: "London, UK",
		Email:    "ada@example.com",
		Phone:    "+44 000",
		Summary:  "Ten years of systems work.",
		Links:    []string{"github.com/ada"},
		Skills:   []string{"Go", "PostgreSQL"},
		Experience: []resumeextract.Experience{
			{Title: "Senior Engineer", Company: "Analytical Engines", Location: "London", Start: &perioddate.PeriodDate{Year: 2018}, Current: true,
				Summary: "Pioneering computing company.", Highlights: []string{"Built the difference engine."}, Stack: []string{"Assembly"}},
		},
		Education: []resumeextract.Education{
			{Degree: "BSc Mathematics", Institution: "Cambridge", Year: &perioddate.PeriodDate{Year: 1835}},
		},
		Languages: []string{"English", "French"},
		Projects: []resumeextract.Project{
			{Name: "opensched", Link: "opensched.dev", Highlights: []string{"A tiny cron scheduler."}},
		},
		Certifications: []string{"AWS Certified Solutions Architect", "CKA"},
	}

	doc := Seed(s)

	// Contact header must land in full — same guarantee as skills: present on the structure
	// means present on the seeded Document. A missing field here is a silent blank CV top.
	wantHeader := Header{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+44 000",
		Location: "London, UK",
		Links:    []string{"github.com/ada"},
	}
	if !reflect.DeepEqual(doc.Header, wantHeader) {
		t.Errorf("header = %+v, want %+v", doc.Header, wantHeader)
	}
	if len(doc.Skills) != 1 || !reflect.DeepEqual(doc.Skills[0].Items, []string{"Go", "PostgreSQL"}) {
		t.Errorf("skills = %+v, want a single group with Go/PostgreSQL", doc.Skills)
	}
	// Summary prefers the extracted summary (falls back to the headline when absent).
	if doc.Summary != "Ten years of systems work." {
		t.Errorf("summary not seeded: %q", doc.Summary)
	}
	if len(doc.Experience) != 1 {
		t.Fatalf("experience not seeded: %+v", doc.Experience)
	}
	e := doc.Experience[0]
	if e.Role != "Senior Engineer" || e.Company != "Analytical Engines" || e.Location != "London" ||
		e.Start == nil || *e.Start != (perioddate.PeriodDate{Year: 2018}) || !e.Current {
		t.Errorf("experience fields not seeded: %+v", e)
	}
	if e.Summary != "Pioneering computing company." {
		t.Errorf("experience summary not seeded: %q", e.Summary)
	}
	if len(e.Bullets) != 1 || e.Bullets[0] != "Built the difference engine." {
		t.Errorf("experience highlights not carried into bullets: %+v", e.Bullets)
	}
	if len(e.Stack) != 1 || e.Stack[0] != "Assembly" {
		t.Errorf("experience stack not seeded: %+v", e.Stack)
	}
	if len(doc.Education) != 1 || doc.Education[0].Degree != "BSc Mathematics" ||
		doc.Education[0].End == nil || *doc.Education[0].End != (perioddate.PeriodDate{Year: 1835}) {
		t.Errorf("education not seeded: %+v", doc.Education)
	}
	if len(doc.Languages) != 2 || doc.Languages[0].Name != "English" {
		t.Errorf("languages not seeded: %+v", doc.Languages)
	}
	if len(doc.Projects) != 1 || doc.Projects[0].Name != "opensched" || doc.Projects[0].Link != "opensched.dev" {
		t.Errorf("projects not seeded: %+v", doc.Projects)
	}
	if len(doc.Certifications) != 2 || doc.Certifications[0].Name != "AWS Certified Solutions Architect" ||
		doc.Certifications[1].Name != "CKA" {
		t.Errorf("certifications not seeded: %+v", doc.Certifications)
	}
}

// TestSeedMapsEveryContactField is the tripwire for "contacts always land like skills":
// every Header field has a Structured counterpart that Seed must copy. Naming fields in the
// assertion would miss a newly added contact identifier the same way a blacklist would.
func TestSeedMapsEveryContactField(t *testing.T) {
	s := resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+44 000",
		Location: "London, UK",
		Links:    []string{"github.com/ada", "linkedin.com/in/ada"},
	}
	doc := Seed(s)

	hv := reflect.ValueOf(doc.Header)
	ht := hv.Type()
	for i := range hv.NumField() {
		f := hv.Field(i)
		name := ht.Field(i).Name
		if f.IsZero() {
			t.Errorf("Header.%s is empty after Seed — contact fields must always be copied from Structured when present", name)
		}
	}
	if len(doc.Header.Links) != 2 {
		t.Errorf("links = %v, want both Structured links", doc.Header.Links)
	}
}

func TestSeedEmptyStructureIsValidSkeleton(t *testing.T) {
	doc := Seed(resumeextract.Structured{})
	before := doc
	doc.Sanitize()
	if !equalDocument(before, doc) {
		t.Errorf("seed of empty structure is not sanitize-stable")
	}
}

// The bug this pinned: an experience-bank employment (or the bank's placeless bucket)
// accumulating more highlights than MaxBullets over years of real use built a seed that
// was already over cvedit's write-gate ceiling — and since reseeding rebuilds this exact
// seed from the bank every time, the refusal was permanent. Capping HERE, not only at the
// bank-projection layer, is what keeps every OTHER reader of the bank (WorkHistory /
// Professional — fit-analysis scoring, /me/profile) seeing every highlight uncapped: see
// internal/candidate/experience/professional.go, which deliberately does not cap.
func TestSeedCapsExperienceHighlightsAtMaxBullets(t *testing.T) {
	const extra = 5
	total := MaxBullets + extra
	highlights := make([]string, total)
	for i := range highlights {
		// Oldest first, matching how the bank's ListExperienceAtoms orders atoms.
		highlights[i] = fmt.Sprintf("claim %d", i)
	}

	doc := Seed(resumeextract.Structured{
		Experience: []resumeextract.Experience{{Company: "Acme", Highlights: highlights}},
	})

	if len(doc.Experience) != 1 {
		t.Fatalf("experience = %+v, want one row", doc.Experience)
	}
	bullets := doc.Experience[0].Bullets
	if len(bullets) != MaxBullets {
		t.Fatalf("bullets = %d, want capped at %d", len(bullets), MaxBullets)
	}
	if bullets[0] != fmt.Sprintf("claim %d", extra) {
		t.Errorf("first surviving bullet = %q, want the oldest dropped — kept ones should start at claim %d", bullets[0], extra)
	}
	if last := bullets[len(bullets)-1]; last != fmt.Sprintf("claim %d", total-1) {
		t.Errorf("last surviving bullet = %q, want the most recently added claim", last)
	}
}

// A project's highlights need the same cap as an experience entry's — the write gate
// enforces MaxBullets on both (cv.MaxBullets is "per experience (and per project)").
func TestSeedCapsProjectHighlightsAtMaxBullets(t *testing.T) {
	total := MaxBullets + 3
	highlights := make([]string, total)
	for i := range highlights {
		highlights[i] = fmt.Sprintf("claim %d", i)
	}

	doc := Seed(resumeextract.Structured{
		Projects: []resumeextract.Project{{Name: "opensched", Highlights: highlights}},
	})

	if len(doc.Projects) != 1 {
		t.Fatalf("projects = %+v, want one row", doc.Projects)
	}
	if len(doc.Projects[0].Bullets) != MaxBullets {
		t.Fatalf("bullets = %d, want capped at %d", len(doc.Projects[0].Bullets), MaxBullets)
	}
}

package sources

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// githublistsNow is the clock every fixture below is dated against.
var githublistsNow = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func githublistsEpoch(daysAgo int) string {
	return strconv.FormatInt(githublistsNow.AddDate(0, 0, -daysAgo).Unix(), 10)
}

// githublistsListing is a Simplify-shaped internship list: one live entry whose page embeds a
// JobPosting, one whose employer is covered first-party, one whose page carries no ld+json,
// and three that must be filtered — inactive, hidden, stale.
func githublistsListing() string {
	return `[
	  {"id": "e1", "company_name": "Stripe", "title": "Software Engineer Intern",
	   "url": "https://boards.greenhouse.io/stripe/jobs/1", "locations": ["San Francisco, CA", "New York, NY"],
	   "terms": ["Summer 2027"], "sponsorship": "Does Not Offer Sponsorship", "degrees": ["Bachelor's", "Master's"],
	   "category": "Software Engineering", "active": true, "is_visible": true,
	   "date_posted": ` + githublistsEpoch(3) + `, "date_updated": ` + githublistsEpoch(1) + `},
	  {"id": "e5", "company_name": "Covered Co", "title": "Backend Intern",
	   "url": "https://jobs.lever.co/covered/2", "locations": ["Remote in USA"], "terms": ["Summer 2027"],
	   "sponsorship": "Other", "active": true, "is_visible": true, "date_posted": ` + githublistsEpoch(2) + `, "date_updated": ` + githublistsEpoch(2) + `},
	  {"id": "e6", "company_name": "Plain Co", "title": "Data Intern",
	   "url": "https://example.com/nold", "locations": [], "terms": ["Fall 2026"],
	   "sponsorship": "U.S. Citizenship is Required", "active": true, "is_visible": true, "date_posted": ` + githublistsEpoch(2) + `, "date_updated": ` + githublistsEpoch(2) + `},
	  {"id": "e2", "company_name": "ClosedCo", "title": "Inactive Intern", "url": "https://example.com/closed",
	   "active": false, "is_visible": true, "date_posted": ` + githublistsEpoch(2) + `, "date_updated": ` + githublistsEpoch(2) + `},
	  {"id": "e3", "company_name": "HiddenCo", "title": "Hidden Intern", "url": "https://example.com/hidden",
	   "active": true, "is_visible": false, "date_posted": ` + githublistsEpoch(2) + `, "date_updated": ` + githublistsEpoch(2) + `},
	  {"id": "e4", "company_name": "StaleCo", "title": "Stale Intern", "url": "https://example.com/stale",
	   "active": true, "is_visible": true, "date_posted": ` + githublistsEpoch(100) + `, "date_updated": ` + githublistsEpoch(100) + `}
	]`
}

const githublistsLDPage = `<html><head><script type="application/ld+json">
{"@context": "https://schema.org", "@type": "JobPosting", "title": "Software Engineer Intern",
 "description": "<p>Build the payments stack.</p>", "hiringOrganization": {"name": "Stripe"}}
</script></head><body></body></html>`

func githublistsFake() *routedHTTP {
	return (&routedHTTP{}).
		route("listings.json", githublistsListing()).
		route("boards.greenhouse.io/stripe/jobs/1", githublistsLDPage).
		route("example.com/nold", `<html><body><h1>No structured data here</h1></body></html>`)
}

func githublistsUnderTest(fake *routedHTTP) githublists {
	return githublists{http: fake, now: func() time.Time { return githublistsNow }}
}

func TestGithubListsFetchNewGatedHydratesOnlyWhatNeedsABody(t *testing.T) {
	fake := githublistsFake()
	src := githublistsUnderTest(fake)
	entry := CompanyEntry{Company: "SimplifyJobs — Summer 2027 internships", Board: "SimplifyJobs/Summer2027-Internships"}
	seen := func(string) bool { return false }
	covered := func(names []string) map[string]bool {
		if len(names) != 3 {
			t.Errorf("covered asked about %v, want the three live employers", names)
		}
		return map[string]bool{"Covered Co": true}
	}
	jobs, err := src.FetchNewGated(context.Background(), entry, seen, covered)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (inactive, hidden and stale filtered): %+v", len(jobs), jobs)
	}
	// The listing, Stripe's page, Plain Co's page. The covered employer's page is never read.
	if fake.calls != 3 {
		t.Errorf("requests = %d, want 3", fake.calls)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	got := byID["e1"]
	if got.Company != "Stripe" || got.Title != "Software Engineer Intern" || got.URL != "https://boards.greenhouse.io/stripe/jobs/1" {
		t.Errorf("e1 identity = %+v", got)
	}
	if got.Location != "San Francisco, CA; New York, NY" {
		t.Errorf("e1 Location = %q", got.Location)
	}
	if got.EmploymentType != "internship" || got.Seniority != "intern" {
		t.Errorf("e1 EmploymentType/Seniority = %q/%q, want internship/intern from the repo name", got.EmploymentType, got.Seniority)
	}
	for _, want := range []string{"Terms: Summer 2027", "Category: Software Engineering", "Degrees: Bachelor&#39;s, Master&#39;s", "does not offer visa sponsorship", "Build the payments stack."} {
		if !strings.Contains(got.Description, want) {
			t.Errorf("e1 Description lacks %q: %s", want, got.Description)
		}
	}
	if got.PostedAt == nil || !got.PostedAt.Equal(githublistsNow.AddDate(0, 0, -3)) {
		t.Errorf("e1 PostedAt = %v, want date_posted", got.PostedAt)
	}

	covered5 := byID["e5"]
	if strings.Contains(covered5.Description, "<p>") && !strings.Contains(covered5.Description, "Terms") {
		t.Errorf("e5 Description = %q", covered5.Description)
	}
	if !strings.Contains(covered5.Description, "Terms: Summer 2027") {
		t.Errorf("e5 (covered) Description = %q, want the list's notes only", covered5.Description)
	}
	if !covered5.Remote {
		t.Error("e5 'Remote in USA' not flagged remote")
	}

	plain := byID["e6"]
	if !strings.Contains(plain.Description, "U.S. citizenship is required") || strings.Contains(plain.Description, "structured") {
		t.Errorf("e6 Description = %q, want the notes kept when the page carries no JobPosting", plain.Description)
	}
}

// A stored entry is a liveness refresh: no page read, no content.
func TestGithubListsSeenEntryIsRefreshedWithoutARequest(t *testing.T) {
	fake := githublistsFake()
	src := githublistsUnderTest(fake)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Company: "x", Board: "SimplifyJobs/Summer2027-Internships"},
		func(id string) bool { return id == "e1" })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ExternalID == "e1" && (!j.SeenRefresh || strings.Contains(j.Description, "payments")) {
			t.Errorf("e1 = %+v, want a SeenRefresh without the body", j)
		}
	}
	// The listing, Covered Co's page (not gated here), Plain Co's page — never Stripe's.
	if fake.calls != 3 {
		t.Errorf("requests = %d, want 3", fake.calls)
	}
}

func TestGithubListsBoardParsing(t *testing.T) {
	cases := []struct {
		board string
		want  githublistsRepo
		ok    bool
	}{
		{"SimplifyJobs/New-Grad-Positions", githublistsRepo{"SimplifyJobs", "New-Grad-Positions", "dev"}, true},
		{" vanshb03/Summer2027-Internships@main ", githublistsRepo{"vanshb03", "Summer2027-Internships", "main"}, true},
		{"no-slash", githublistsRepo{}, false},
		{"a/b/c", githublistsRepo{}, false},
		{"a/b@", githublistsRepo{}, false},
	}
	for _, c := range cases {
		got, err := githublistsBoard(c.board)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("githublistsBoard(%q) = %+v, %v; want %+v, ok=%v", c.board, got, err, c.want, c.ok)
		}
	}
	if u := (githublistsRepo{"SimplifyJobs", "New-Grad-Positions", "dev"}).url(); u != "https://raw.githubusercontent.com/SimplifyJobs/New-Grad-Positions/dev/.github/scripts/listings.json" {
		t.Errorf("url = %s", u)
	}
}

// What a list is about comes from its name: new-grad lists are full-time junior roles and
// state "New Grad" as their term; a vanshb03 season borrows its year from the repo name.
func TestGithubListsRepoShapeAndTerms(t *testing.T) {
	newGrad := githublistsRepo{"SimplifyJobs", "New-Grad-Positions", "dev"}
	if e, s := newGrad.shape(); e != "full_time" || s != "junior" {
		t.Errorf("new-grad shape = %q/%q", e, s)
	}
	if got := (githublistsEntry{Terms: []string{"ignored"}}).termsLine(newGrad); got != "Terms: New Grad" {
		t.Errorf("new-grad terms = %q", got)
	}
	vansh := githublistsRepo{"vanshb03", "Summer2027-Internships", "dev"}
	if got := (githublistsEntry{Season: "Summer"}).termsLine(vansh); got != "Terms: Summer 2027" {
		t.Errorf("vansh terms = %q", got)
	}
	if e, s := (githublistsRepo{"x", "Jobs", "dev"}).shape(); e != "" || s != "" {
		t.Errorf("unknown list shape = %q/%q, want none", e, s)
	}
}

// A list is whole or not read at all: a file cut mid-entry fails the board rather than
// reporting the entries before the cut as the catalogue.
func TestGithubListsTruncatedFileFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("listings.json", `[{"id": "e1", "company_name": "A", "title": "T", "url": "https://a.example/1", "active": true}, {"id": "e2", "compa`)
	_, err := githublistsUnderTest(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "a/b"})
	if err == nil || !strings.Contains(err.Error(), "decode entry") {
		t.Fatalf("err = %v, want the decode failure", err)
	}
}

func TestGithubListsMarkers(t *testing.T) {
	src := NewGithubLists(nil)
	if src.Provider() != "githublists" {
		t.Errorf("Provider() = %q", src.Provider())
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("githublists is not an aggregator")
	}
	if _, ok := src.(boardless); ok {
		t.Error("githublists is boardless; the repository is the board")
	}
	if _, ok := src.(fullCatalog); !ok {
		t.Error("githublists is not a fullCatalog; an entry flipped inactive would never close")
	}
	if _, ok := src.(CoverageGated); !ok {
		t.Error("githublists is not CoverageGated; it would buy every covered employer's body every run")
	}
}

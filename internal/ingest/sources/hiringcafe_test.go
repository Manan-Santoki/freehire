package sources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// hiringcafePage wraps a props.pageProps JSON object in the same minimal page shape
// hiringcafe.com serves: a <title> (an expired posting is recognised by it) and the
// __NEXT_DATA__ script the adapter reads.
func hiringcafePageHTML(title, pagePropsJSON string) string {
	return `<!DOCTYPE html><html><head><title>` + title + `</title></head><body>` +
		`<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":` + pagePropsJSON + `}}` +
		`</script></body></html>`
}

// hiringcafeHitJSON is one search hit with the fields the adapter maps. It mirrors the live
// shape captured 2026-09-14: no body on the hit, the structured reading under
// v5_processed_job_data, per-frequency compensation pairs.
func hiringcafeHitJSON(id, req, title, company string, expired bool) string {
	exp := "false"
	if expired {
		exp = "true"
	}
	return `{
	  "id": "` + id + `", "requisition_id": "` + req + `", "is_expired": ` + exp + `,
	  "apply_url": "https://jobs.example.com/` + id + `", "board_token": "acme-board",
	  "job_information": {"title": "` + title + `", "description": null},
	  "enriched_company_data": {"name": "Acme Enriched"},
	  "v5_processed_job_data": {
	    "core_job_title": "` + title + `", "company_name": "` + company + `",
	    "formatted_workplace_location": "Reston, Virginia, United States",
	    "workplace_countries": ["US", "US", "CA"], "workplace_type": "Onsite",
	    "commitment": ["Full Time"], "seniority_level": "Senior Level",
	    "technical_tools": ["Python", "Go"], "estimated_publish_date": "2026-09-10T03:50:29.663Z",
	    "min_industry_and_role_yoe": 5,
	    "listed_compensation_currency": "USD", "listed_compensation_frequency": "Yearly",
	    "yearly_min_compensation": 120000, "yearly_max_compensation": 150000.4,
	    "hourly_min_compensation": null, "hourly_max_compensation": null
	  }
	}`
}

// hiringcafeSearchJSON is one search page's pageProps.
func hiringcafeSearchJSON(last bool, hits ...string) string {
	l := "false"
	if last {
		l = "true"
	}
	return `{"ssrHits": [` + strings.Join(hits, ",") + `], "ssrPage": 0, "ssrTotalCount": 4,
	  "ssrPageSize": 40, "ssrIsLastPage": ` + l + `, "ssrError": null}`
}

const hiringcafeDetailA = `{"job": {"id": "src___a", "requisition_id": "req-a", "is_expired": false,
  "job_information": {"title": "Software Engineer", "description": "<p>Build <b>things</b>.</p>"},
  "v5_processed_job_data": {"company_name": "Acme"}}, "similarJobs": []}`

func hiringcafeFake() *routedHTTP {
	// The detail route must precede the search route: a posting page URL never carries
	// "searchState", but ordering keeps that independent of the fake's matching.
	return (&routedHTTP{}).
		route("/job/req-a", hiringcafePageHTML("Acme - Software Engineer", hiringcafeDetailA)).
		route("searchState", hiringcafePageHTML("HiringCafe", hiringcafeSearchJSON(true,
			hiringcafeHitJSON("src___a", "req-a", "Software Engineer", "Acme", false),
			hiringcafeHitJSON("src___seen", "req-seen", "Seen Engineer", "Acme", false),
			hiringcafeHitJSON("src___expired", "req-x", "Expired Engineer", "Acme", true),
			// No employer name anywhere: unattributable, dropped before any request.
			`{"id": "src___nameless", "requisition_id": "req-n", "apply_url": "https://x.example/1",
			  "job_information": {"title": "Ghost"}, "v5_processed_job_data": {}}`,
		)))
}

func TestHiringCafeFetchNewMapsAPostingAndRefreshesASeenOne(t *testing.T) {
	fake := hiringcafeFake()
	src := NewHiringCafe(fake).(HydratingSource)
	seen := func(id string) bool { return id == "src___seen" }
	jobs, err := src.FetchNew(context.Background(),
		CompanyEntry{Company: "hiring.cafe — software engineer", Board: "software engineer", Region: "US"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one hydrated, one refreshed): %+v", len(jobs), jobs)
	}
	// One search page plus exactly one posting page: the seen hit, the expired hit and the
	// nameless hit must cost nothing.
	if fake.calls != 2 {
		t.Errorf("requests = %d, want 2", fake.calls)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	got := byID["src___a"]
	if got.SeenRefresh {
		t.Error("new posting flagged SeenRefresh")
	}
	if got.URL != "https://jobs.example.com/src___a" {
		t.Errorf("URL = %q, want the employer's apply_url", got.URL)
	}
	if got.Title != "Software Engineer" || got.Company != "Acme" {
		t.Errorf("Title/Company = %q/%q", got.Title, got.Company)
	}
	if !strings.Contains(got.Description, "Build <b>things</b>.") {
		t.Errorf("Description = %q, want the posting page's HTML body", got.Description)
	}
	if got.Location != "Reston, Virginia, United States" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.WorkMode != "onsite" || got.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want onsite/false", got.WorkMode, got.Remote)
	}
	if len(got.Countries) != 2 || got.Countries[0] != "us" || got.Countries[1] != "ca" {
		t.Errorf("Countries = %v, want [us ca] (deduplicated, in order)", got.Countries)
	}
	if got.Seniority != "senior" || got.EmploymentType != "full_time" {
		t.Errorf("Seniority/EmploymentType = %q/%q", got.Seniority, got.EmploymentType)
	}
	if got.ExperienceYearsMin == nil || *got.ExperienceYearsMin != 5 {
		t.Errorf("ExperienceYearsMin = %v, want 5", got.ExperienceYearsMin)
	}
	if len(got.Skills) == 0 {
		t.Error("Skills empty, want the technical_tools canonicalized")
	}
	if got.SalaryMin == nil || *got.SalaryMin != 120000 || got.SalaryMax == nil || *got.SalaryMax != 150000 ||
		got.SalaryCurrency != "USD" || got.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s, want 120000-150000 USD/year", got.SalaryMin, got.SalaryMax, got.SalaryCurrency, got.SalaryPeriod)
	}
	if got.PostedAt == nil || got.PostedAt.UTC().Format("2006-01-02") != "2026-09-10" {
		t.Errorf("PostedAt = %v, want 2026-09-10", got.PostedAt)
	}

	refreshed := byID["src___seen"]
	if !refreshed.SeenRefresh || refreshed.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", refreshed)
	}
}

// A hit stating no salary bounds, or a frequency freehire has no period for, yields no
// structured salary rather than a half-qualified one.
func TestHiringCafeSalaryNeedsABoundAndAKnownPeriod(t *testing.T) {
	cases := []struct {
		name string
		p    hiringcafeProcessed
		want bool
	}{
		{"no bounds", hiringcafeProcessed{CompensationCurrency: "USD", CompensationFrequency: "Yearly"}, false},
		{"no currency", hiringcafeProcessed{CompensationFrequency: "Yearly", YearlyMin: ptrFloat(1)}, false},
		{"weekly", hiringcafeProcessed{CompensationCurrency: "USD", CompensationFrequency: "Weekly", YearlyMin: ptrFloat(1)}, false},
		{"hourly", hiringcafeProcessed{CompensationCurrency: "usd", CompensationFrequency: "Hourly", HourlyMin: ptrFloat(27.5)}, true},
	}
	for _, c := range cases {
		var job Job
		c.p.applySalary(&job)
		if got := job.SalaryPeriod != ""; got != c.want {
			t.Errorf("%s: salary set = %v, want %v (%+v)", c.name, got, c.want, job)
		}
		if c.name == "hourly" && (job.SalaryCurrency != "USD" || job.SalaryPeriod != "hour" || *job.SalaryMin != 28) {
			t.Errorf("hourly: got %v %s/%s", *job.SalaryMin, job.SalaryCurrency, job.SalaryPeriod)
		}
	}
}

func ptrFloat(v float64) *float64 { return &v }

// An unknown region fails the board before any request: a mistyped code must not search the
// world under one country's label.
func TestHiringCafeRefusesAnUnknownRegion(t *testing.T) {
	fake := hiringcafeFake()
	_, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go", Region: "ZZ"})
	if err == nil || !strings.Contains(err.Error(), "hiringcafeCountryNames") {
		t.Fatalf("err = %v, want the unknown-region error", err)
	}
	if fake.calls != 0 {
		t.Errorf("requests = %d, want 0", fake.calls)
	}
}

// The search state carries the keyword, newest-first ordering and the country filter; a
// later page rides in ?page=N.
func TestHiringCafeSearchStateAndURL(t *testing.T) {
	state, err := hiringcafeState(CompanyEntry{Board: " data engineer ", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"searchQuery":"data engineer"`, `"sortBy":"date"`, `"short_name":"US"`, `"long_name":"United States"`, `"userLocation":null`} {
		if !strings.Contains(state, want) {
			t.Errorf("state %s lacks %s", state, want)
		}
	}
	if u := hiringcafeSearchURL(state, 0); strings.Contains(u, "page=") || !strings.HasPrefix(u, hiringcafeBaseURL+"?searchState=") {
		t.Errorf("page 0 url = %s", u)
	}
	if u := hiringcafeSearchURL(state, 3); !strings.HasSuffix(u, "&page=3") {
		t.Errorf("page 3 url = %s", u)
	}
	worldwide, _ := hiringcafeState(CompanyEntry{Board: "go"})
	if !strings.Contains(worldwide, `"locations":[]`) {
		t.Errorf("worldwide state = %s, want an empty locations filter", worldwide)
	}
}

// A challenge interstitial carries no __NEXT_DATA__; on page 0 that is a board failure, never
// an empty keyword.
func TestHiringCafeChallengePageFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("searchState", `<html><head><title>Just a moment...</title></head><body></body></html>`)
	_, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err == nil || !strings.Contains(err.Error(), "__NEXT_DATA__") {
		t.Fatalf("err = %v, want the missing-payload error", err)
	}
}

// A crawl that listed new postings and read none of their bodies is a wall, not a quiet source.
func TestHiringCafeReadingNoBodyFailsTheCrawl(t *testing.T) {
	fake := hiringcafeFake().routeErr("/job/", &StatusError{Method: "GET", Code: http.StatusInternalServerError, URL: "x"})
	_, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Fatalf("err = %v, want the read-none failure", err)
	}
}

// The walk ends on a page that adds nothing new, deduplicating a hit that straddles pages.
func TestHiringCafeWalkStopsWhenAPageAddsNothing(t *testing.T) {
	hitA := hiringcafeHitJSON("src___a", "req-a", "Software Engineer", "Acme", false)
	fake := (&routedHTTP{}).
		route("/job/req-a", hiringcafePageHTML("Acme", hiringcafeDetailA)).
		route("page=1", hiringcafePageHTML("HiringCafe", hiringcafeSearchJSON(false, hitA))).
		route("searchState", hiringcafePageHTML("HiringCafe", hiringcafeSearchJSON(false, hitA)))
	jobs, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(jobs) = %d, want 1", len(jobs))
	}
	// page 0, page 1, one detail.
	if fake.calls != 3 {
		t.Errorf("requests = %d, want 3", fake.calls)
	}
}

// An expired posting page is recognised by its title and dropped, not stored body-less.
func TestHiringCafeExpiredPostingPageIsDropped(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/job/req-a", hiringcafePageHTML("HiringCafe - Expired Job", `{"job": null}`)).
		route("/job/req-b", hiringcafePageHTML("Acme", `{"job": {"id": "src___b", "job_information": {"description": "<p>Live.</p>"}}}`)).
		route("searchState", hiringcafePageHTML("HiringCafe", hiringcafeSearchJSON(true,
			hiringcafeHitJSON("src___a", "req-a", "Gone", "Acme", false),
			hiringcafeHitJSON("src___b", "req-b", "Live", "Acme", false))))
	jobs, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "src___b" {
		t.Errorf("jobs = %+v, want only src___b", jobs)
	}
}

// flakyHTML refuses the first request as a burst and serves the rest.
type flakyHTML struct {
	inner    HTMLGetter
	refusals int
	calls    int
}

func (f *flakyHTML) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	f.calls++
	if f.calls <= f.refusals {
		return nil, &StatusError{Method: "GET", Code: http.StatusTooManyRequests, URL: url}
	}
	return f.inner.GetHTML(ctx, url)
}

// A 429 is retried once per rung of the ladder, beneath the limiter.
func TestHiringCafeRetriesARefusedBurst(t *testing.T) {
	saved := hiringcafeRetryDelays
	hiringcafeRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { hiringcafeRetryDelays = saved }()

	flaky := &flakyHTML{inner: hiringcafeFake(), refusals: 2}
	jobs, err := NewHiringCafe(flaky).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err != nil {
		t.Fatalf("Fetch after two refusals: %v", err)
	}
	if len(jobs) == 0 {
		t.Error("no jobs after the retries succeeded")
	}
	// Three refusals exhaust the ladder and fail page 0 with the refusal itself.
	exhausted := &flakyHTML{inner: hiringcafeFake(), refusals: 3}
	_, err = NewHiringCafe(exhausted).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusTooManyRequests {
		t.Errorf("err past the ladder = %v, want the 429 surfaced", err)
	}
}

// Once a refusal survives the retry ladder, no further detail request is made this run: the
// remaining new hits are dropped (they stay new for the next run) instead of re-earning the
// block, and a run that read nothing reports the wall.
func TestHiringCafeBreakerStopsDetailRequestsAfterARefusal(t *testing.T) {
	saved := hiringcafeRetryDelays
	hiringcafeRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { hiringcafeRetryDelays = saved }()

	fake := hiringcafeFake().routeErr("/job/", &StatusError{Method: "GET", Code: http.StatusTooManyRequests, URL: "x"})
	_, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err == nil || !strings.Contains(err.Error(), "walled=true") {
		t.Fatalf("err = %v, want the wall reported", err)
	}
	// One listing page, then at most the two in-flight workers' ladders (three requests each)
	// before the breaker trips; never one ladder per listed hit.
	if fake.calls > 1+2*3 {
		t.Errorf("requests = %d, want at most 7", fake.calls)
	}
}

// Only the first hiringcafeMaxNewPerRun new hits of a board are attempted per run; the rest
// cost nothing and stay new.
func TestHiringCafeBudgetBoundsDetailRequestsPerRun(t *testing.T) {
	saved := hiringcafeMaxNewPerRun
	hiringcafeMaxNewPerRun = 1
	defer func() { hiringcafeMaxNewPerRun = saved }()

	// Both new hits have a readable page; the two-worker pool decides which one wins the
	// single slot, so the assertion is on the count, not the identity.
	fake := hiringcafeFake().route("/job/req-seen", hiringcafePageHTML("Acme", `{"job": {"id": "src___seen",
	  "job_information": {"description": "<p>Also live.</p>"}}}`))
	jobs, err := NewHiringCafe(fake).Fetch(context.Background(), CompanyEntry{Company: "x", Board: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("jobs = %+v, want exactly one hydrated posting", jobs)
	}
	if fake.calls != 2 {
		t.Errorf("requests = %d, want the listing plus one detail", fake.calls)
	}
}

// A covered employer's hit is yielded body-less without a request, so the edge's scarce
// budget is never spent on a copy the coverage gate will discard.
func TestHiringCafeFetchNewGatedSkipsCoveredEmployers(t *testing.T) {
	fake := hiringcafeFake().route("/job/req-seen", hiringcafePageHTML("Acme", `{"job": {"id": "src___seen",
	  "job_information": {"description": "<p>Also live.</p>"}}}`))
	src := NewHiringCafe(fake).(CoverageGated)
	covered := func(names []string) map[string]bool {
		if len(names) != 2 {
			t.Errorf("covered asked about %v, want the two live, attributable hits", names)
		}
		return map[string]bool{"Acme": true}
	}
	jobs, err := src.FetchNewGated(context.Background(), CompanyEntry{Company: "x", Board: "go"},
		func(string) bool { return false }, covered)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	// Every live hit names Acme, so nothing is hydrated: the listing is the only request.
	if fake.calls != 1 {
		t.Errorf("requests = %d, want 1 (listing only)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want the two live hits yielded body-less: %+v", len(jobs), jobs)
	}
	for _, j := range jobs {
		if j.Description != "" || j.SeenRefresh {
			t.Errorf("covered hit = %+v, want body-less and not a refresh", j)
		}
	}
}

func TestHiringCafeMarkers(t *testing.T) {
	src := NewHiringCafe(nil)
	if src.Provider() != "hiringcafe" {
		t.Errorf("Provider() = %q", src.Provider())
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("hiringcafe is not an aggregator; its copies of first-party postings would never yield")
	}
	if _, ok := src.(boardless); ok {
		t.Error("hiringcafe is boardless; the keyword is the board")
	}
	if _, ok := src.(HydratingSource); !ok {
		t.Error("hiringcafe is not a HydratingSource; every crawl would re-read every body")
	}
	if _, ok := src.(CoverageGated); !ok {
		t.Error("hiringcafe is not CoverageGated; it would spend its request budget on copies the gate discards")
	}
	if g, ok := src.(sweepGrace); !ok || g.sweepGrace() != 14*24*time.Hour {
		t.Error("hiringcafe does not declare the 14-day sweep grace its keyword slice needs")
	}
}

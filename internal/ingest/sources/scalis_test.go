package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// scalisPage1HTML is a trimmed but byte-faithful RSC-flight fixture modeled directly on a
// live-captured Scalis listing page (boldbusiness.scalis.ai/jobs): two postings, each
// carrying its description as a "$<id>" reference into the flight's own text rows.
const scalisPage1HTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":[{\"id\":\"aaaa1111-0000-4000-8000-000000000001\",\"title\":\"Senior Backend Engineer\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[{\"city\":\"Bogota\",\"country\":\"CO\"}],\"employment\":\"FULL_TIME\",\"workplace\":\"REMOTE\",\"payment\":\"SALARY\",\"skills\":[\"Go\",\"Postgres\"],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$28\",\"descriptionHtml\":\"$29\",\"createdAt\":\"2026-06-18T17:10:41.385Z\"},{\"id\":\"aaaa1111-0000-4000-8000-000000000002\",\"title\":\"Support Engineer\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[{\"city\":\"Lima\",\"country\":\"PE\"}],\"employment\":\"CONTRACTOR\",\"workplace\":\"HYBRID\",\"payment\":\"HOURLY\",\"skills\":[],\"salary\":{\"min\":20,\"max\":30,\"currency\":\"USD\"},\"description\":\"$2a\",\"descriptionHtml\":\"$2b\",\"createdAt\":\"2026-07-01T09:00:00.000Z\"}],\"count\":2,\"paginationCount\":2}}\n28:T15,Reports to: Team Lead\n29:T14,<p>Build things.</p>\n2a:Tf,Reports to: CTO\n2b:T15,<p>Ship features.</p>\n"])</script>
</body></html>`

// scalisEmptyPageHTML is a listing page past the last real page: an empty results array,
// the confirmed live termination signal (no redirect trap).
const scalisEmptyPageHTML = `<html><body>
<script>self.__next_f.push([1,"1:{\"initialData\":{\"results\":[],\"count\":2,\"paginationCount\":2}}\n"])</script>
</body></html>`

func scalisListingURLFor(page int) string {
	if page == 1 {
		return "https://boldbusiness.scalis.ai/jobs?page=1&limit=10&sortBy=SORT_BEST_MATCH"
	}
	return "https://boldbusiness.scalis.ai/jobs?page=2&limit=10&sortBy=SORT_BEST_MATCH"
}

func TestScalisProvider(t *testing.T) {
	if got := NewScalis(nil).Provider(); got != "scalis" {
		t.Errorf("Provider() = %q, want %q", got, "scalis")
	}
}

func TestScalisFetchSinglePageAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisPage1HTML).
		route(scalisListingURLFor(2), scalisEmptyPageHTML)

	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Fallback Co", Provider: "scalis", Board: "boldbusiness",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j1 := jobs[0]
	if j1.ExternalID != "aaaa1111-0000-4000-8000-000000000001" {
		t.Errorf("ExternalID = %q", j1.ExternalID)
	}
	if j1.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "BOLD Business" {
		t.Errorf("Company = %q", j1.Company)
	}
	if j1.Location != "Bogota, CO" {
		t.Errorf("Location = %q, want %q", j1.Location, "Bogota, CO")
	}
	if j1.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j1.EmploymentType)
	}
	if j1.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j1.WorkMode)
	}
	if !j1.Remote {
		t.Errorf("Remote = false, want true (WorkMode is remote)")
	}
	if strings.Join(j1.Skills, ",") != "Go,Postgres" {
		t.Errorf("Skills = %v", j1.Skills)
	}
	if j1.Description != "<p>Build things.</p>" {
		t.Errorf("Description = %q, want the resolved HTML row", j1.Description)
	}
	if j1.SalaryMin != nil || j1.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil (null in source)", j1.SalaryMin, j1.SalaryMax)
	}

	j2 := jobs[1]
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract (CONTRACTOR)", j2.EmploymentType)
	}
	if j2.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j2.WorkMode)
	}
	if j2.SalaryMin == nil || *j2.SalaryMin != 20 || j2.SalaryMax == nil || *j2.SalaryMax != 30 {
		t.Errorf("SalaryMin/Max = %v/%v, want 20/30", j2.SalaryMin, j2.SalaryMax)
	}
	if j2.SalaryCurrency != "USD" {
		t.Errorf("SalaryCurrency = %q, want USD", j2.SalaryCurrency)
	}
	if j2.SalaryPeriod != "hour" {
		t.Errorf("SalaryPeriod = %q, want hour (HOURLY payment)", j2.SalaryPeriod)
	}
	if j2.Description != "<p>Ship features.</p>" {
		t.Errorf("Description = %q", j2.Description)
	}
}

func TestScalisFetchPaginatesToExhaustion(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisPage1HTML).
		route(scalisListingURLFor(2), scalisEmptyPageHTML)

	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (page 2 must have been fetched and found empty)", len(jobs))
	}
}

func TestScalisEmptyFirstPageYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route(scalisListingURLFor(1), scalisEmptyPageHTML)
	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// A later-page failure must abort the whole Fetch, never return a partial result as
// success — the property the fullBoardListing marker rests on.
func TestScalisFetchFailsWholeBoardOnLaterPageError(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisPage1HTML).
		routeErr(scalisListingURLFor(2), errors.New("boom"))

	if _, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"}); err == nil {
		t.Fatal("Fetch succeeded despite a later-page listing error")
	}
}

func TestScalisRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["scalis"] {
		t.Error("FullBoardListingProviders(All(nil)) should include scalis")
	}
}

func TestScalisRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["scalis"]
	if !ok {
		t.Fatal("All() missing provider scalis")
	}
	if s.Provider() != "scalis" {
		t.Errorf("All()[scalis].Provider() = %q", s.Provider())
	}
}

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// scalis adapts Scalis, a multi-tenant ATS (<board>.scalis.ai). The board is the tenant
// subdomain. The listing page is a Next.js App Router app whose RSC flight inlines a
// complete, richly-structured job object per posting — title, company, locations,
// employment/workplace enums, skills, salary, and a full HTML description reachable as a
// "$<id>" reference into the flight's own text rows — so, unlike topco, no per-posting
// detail fetch is needed at all. The listing is paginated (confirmed live: 10 results per
// page, an empty result list past the last page, no redirect trap), so Fetch pages to
// exhaustion using the shared fetchFlight/bracketSlice/nextFlightTextRows primitives every
// other RSC-flight adapter (deel, topco, micro1) already uses.
type scalis struct {
	http HTMLGetter
}

// NewScalis builds the Scalis adapter over the given HTML client.
func NewScalis(c HTMLGetter) Source { return scalis{http: c} }

func (scalis) Provider() string { return "scalis" }

// scalisPageSize is the listing's fixed page size (confirmed live).
const scalisPageSize = 10

// scalisMaxPages bounds the page walk. The natural stop is a page whose result list is
// empty (confirmed live, including past the true last page), but a misbehaving tenant that
// never returns an empty page would otherwise loop forever.
const scalisMaxPages = 100

// fullBoardListing: a page-fetch or decode failure at any point fails the whole Fetch (see
// the loop below), so this marker's "whole listing or fail outright" guarantee holds even
// though the listing is paginated.
func (scalis) fullBoardListing() {}

func (s scalis) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= scalisMaxPages; page++ {
		url := fmt.Sprintf("https://%s.scalis.ai/jobs?page=%d&limit=%d&sortBy=SORT_BEST_MATCH",
			e.Board, page, scalisPageSize)
		flight, err := fetchFlight(ctx, s.http, url)
		if err != nil {
			return nil, fmt.Errorf("scalis: board %q page %d: %w", e.Board, page, err)
		}
		listing, err := extractScalisListing(flight)
		if err != nil {
			return nil, fmt.Errorf("scalis: board %q page %d: %w", e.Board, page, err)
		}
		if len(listing.Results) == 0 {
			break
		}
		rows := nextFlightTextRows(flight)
		for _, p := range listing.Results {
			if j, ok := scalisToJob(e, rows, p); ok {
				jobs = append(jobs, j)
			}
		}
	}
	return jobs, nil
}

// scalisListing is the "initialData" object a listing page's flight carries.
type scalisListing struct {
	Results []scalisPosting `json:"results"`
}

// scalisPosting is one listing result. description/descriptionHtml are each either a
// "$<id>" reference into the flight's text rows or, defensively, an inline string.
type scalisPosting struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	DescriptionHTML string   `json:"descriptionHtml"`
	Employment      string   `json:"employment"`
	Workplace       string   `json:"workplace"`
	Payment         string   `json:"payment"`
	Skills          []string `json:"skills"`
	Company         struct {
		Name string `json:"name"`
	} `json:"company"`
	Locations []struct {
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"locations"`
	Salary struct {
		Min      *int   `json:"min"`
		Max      *int   `json:"max"`
		Currency string `json:"currency"`
	} `json:"salary"`
	CreatedAt string `json:"createdAt"`
}

// extractScalisListing decodes the initialData object out of a listing page's flight. It
// anchors on the payload's own opening — `"initialData":{"results"` — rather than a bare
// `"initialData":`, so a stray unrelated match cannot pick the wrong object.
func extractScalisListing(flight string) (scalisListing, error) {
	raw, ok := bracketSlice(flight, `"initialData":{"results"`, '{', '}')
	if !ok {
		return scalisListing{}, fmt.Errorf("no initialData object found")
	}
	var l scalisListing
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		return scalisListing{}, fmt.Errorf("decode initialData: %w", err)
	}
	return l, nil
}

// scalisToJob maps one listing result to a Job. ok is false when the posting carries no
// id, which would collide on the (source, external_id) dedup key.
func scalisToJob(e CompanyEntry, rows map[string]string, p scalisPosting) (Job, bool) {
	if p.ID == "" {
		return Job{}, false
	}

	desc := p.DescriptionHTML
	if ref, isRef := strings.CutPrefix(desc, "$"); isRef {
		desc = rows[ref]
	}

	var locParts []string
	for _, l := range p.Locations {
		locParts = append(locParts, joinNonEmpty(l.City, l.Country))
	}
	location := joinNonEmpty(locParts...)

	workMode := scalisWorkMode(p.Workplace)
	salaryMin, salaryMax, salaryCurrency, salaryPeriod := scalisSalary(p)

	return Job{
		ExternalID:     p.ID,
		URL:            fmt.Sprintf("https://%s.scalis.ai/job/%s", e.Board, p.ID),
		Title:          strings.TrimSpace(p.Title),
		Company:        firstNonEmpty(p.Company.Name, e.Company),
		Location:       location,
		Description:    sanitizeHTML(desc),
		Remote:         workMode == "remote" || isRemote(location),
		WorkMode:       workMode,
		EmploymentType: scalisEmploymentType(p.Employment),
		Skills:         p.Skills,
		SalaryMin:      salaryMin,
		SalaryMax:      salaryMax,
		SalaryCurrency: salaryCurrency,
		SalaryPeriod:   salaryPeriod,
		PostedAt:       parseRFC3339(p.CreatedAt),
	}, true
}

// scalisWorkMode maps Scalis's workplace enum onto freehire's work-mode vocabulary,
// returning "" for an unrecognized value so the pipeline's location heuristic decides.
func scalisWorkMode(workplace string) string {
	switch strings.ToUpper(strings.TrimSpace(workplace)) {
	case "REMOTE":
		return "remote"
	case "HYBRID":
		return "hybrid"
	case "ON_SITE":
		return "onsite"
	}
	return ""
}

// scalisEmploymentType maps Scalis's employment enum onto vocab.EmploymentTypeValues,
// returning "" for an unrecognized value. TEMPORARY folds onto "contract", matching how
// this codebase's other adapters (e.g. talenthr) already read a temporary engagement.
func scalisEmploymentType(employment string) string {
	switch strings.ToUpper(strings.TrimSpace(employment)) {
	case "FULL_TIME":
		return "full_time"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	}
	return ""
}

// scalisSalary maps a posting's salary bounds and the payment enum's own unit into
// freehire's structured salary fields, returning all four empty/nil when neither bound is
// stated. The payment enum states the unit directly ("SALARY" is an annual figure,
// "HOURLY" an hourly one), so no bound is reported without a period to attach it to.
func scalisSalary(p scalisPosting) (min, max *int, currency, period string) {
	if p.Salary.Min == nil && p.Salary.Max == nil {
		return nil, nil, "", ""
	}
	switch strings.ToUpper(strings.TrimSpace(p.Payment)) {
	case "SALARY":
		period = "year"
	case "HOURLY":
		period = "hour"
	default:
		return nil, nil, "", "" // an unrecognized unit would misstate the figure
	}
	return p.Salary.Min, p.Salary.Max, p.Salary.Currency, period
}

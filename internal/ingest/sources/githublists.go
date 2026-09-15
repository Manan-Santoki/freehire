package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// githublistsHTTP is the transport surface the adapter needs: a streaming GET for the
// listings file and an HTML GET for the posting pages it hydrates from.
type githublistsHTTP interface {
	StreamGetter
	HTMLGetter
}

// githublists adapts the curated GitHub job-list repositories — SimplifyJobs/Summer20XX-
// Internships, SimplifyJobs/New-Grad-Positions and vanshb03/Summer20XX-Internships — whose
// README tables are rendered from one machine-readable file, `.github/scripts/listings.json`,
// on the repo's `dev` branch. That file is what this adapter reads; the README is never
// parsed. The lists are US-centric (internships and new-grad roles at US employers), which
// is the whole reason they are onboarded.
//
// # The board is a repository
//
// Each list is one board, spelled "owner/repo" (or "owner/repo@branch" when a list ever moves
// off `dev`). The repo NAME is read for what the list is about — an "Internships" list yields
// internship postings at intern seniority, a "New-Grad" one full-time junior postings — which
// is a fact the list states about every entry, not a guess from any one title.
//
// # What the file states, and what it does not
//
//   - An entry is a row of the list: id, company_name, title, url (the employer's own posting,
//     almost always a Greenhouse/Lever/Ashby/Workday page), locations, terms or season,
//     sponsorship, degrees, category, and two flags. Measured 2026-09-14 on New-Grad-Positions:
//     20,066 entries of which 3,041 are `active` — the lists keep every row they ever
//     carried and flip `active` off rather than deleting, and Simplify additionally
//     force-inactivates a row after two months while vanshb03 never does. So `active` and
//     `is_visible` are read, AND staleness is re-derived from date_updated against
//     githublistsMaxAge, because the two repos disagree on it.
//   - The file carries no body. The employer's page usually embeds a schema.org JobPosting
//     (the ATS platforms these link to all do), so a new posting's body is read from there
//     through the shared ld+json parser — hence HydratingSource. A page with no such block
//     keeps the list's own structured notes (terms, category, degrees, sponsorship) as its
//     body: the sponsorship line in particular is what a US job seeker most wants stated.
//   - Most of these employers run an ATS freehire already crawls first-party, and the
//     pipeline's aggregator gate discards such a copy before it is ever stored — so it is
//     never `seen`, and the plain hydrating crawl would buy its body every run for nothing.
//     CoverageGated exists for exactly this (remotedotcom's case), so the adapter asks which
//     employers are covered and skips their bodies.
//   - The two SimplifyJobs files are 11-13 MB. GetStream reads them under the long stream
//     timeout rather than GetJSON's 15 s.
//
// The file is read whole every run and any failure fails the board, so the provider is a
// fullCatalog: an entry the run did not list has been flipped inactive or aged out, and the
// source-scoped sweep may close it.
type githublists struct {
	http githublistsHTTP
	now  func() time.Time
}

// NewGithubLists builds the adapter over the given HTTP client.
func NewGithubLists(c githublistsHTTP) Source { return githublists{http: c, now: time.Now} }

func (githublists) Provider() string { return "githublists" }

// Every entry names its own employer, and most of those employers are also crawled
// first-party, so the copies must yield to them in the cross-source dedup.
func (githublists) aggregator() {}

// A list is read whole on every crawl and a failed read fails the board, so an entry a clean
// run did not list is genuinely gone from the list.
func (githublists) fullCatalog() {}

const (
	// githublistsRawURL is the listings file's raw URL: owner, repo, branch.
	githublistsRawURL = "https://raw.githubusercontent.com/%s/%s/%s/.github/scripts/listings.json"
	// githublistsDefaultBranch is where both maintainers keep the generated file.
	githublistsDefaultBranch = "dev"
	// githublistsMaxAge is how old an entry's last update may be before it is treated as
	// stale whatever its active flag says — Simplify's own force-inactivation horizon.
	githublistsMaxAge = 60 * 24 * time.Hour
)

// githublistsSegment is what a GitHub owner, repo or branch name may look like.
var githublistsSegment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// githublistsYear pulls the season's year out of a repo name ("Summer2027-Internships").
var githublistsYear = regexp.MustCompile(`20\d{2}`)

// githublistsSponsorship maps the list's sponsorship picklist onto a sentence for the body.
// The three phrasings are the lists' own; anything else ("Other") states nothing.
var githublistsSponsorship = map[string]string{
	"Does Not Offer Sponsorship":   "This position does not offer visa sponsorship.",
	"Offers Sponsorship":           "This position offers visa sponsorship.",
	"U.S. Citizenship is Required": "U.S. citizenship is required for this position.",
}

// githublistsRepo is a parsed board.
type githublistsRepo struct {
	Owner, Repo, Branch string
}

// githublistsBoard parses "owner/repo" or "owner/repo@branch".
func githublistsBoard(board string) (githublistsRepo, error) {
	b := strings.TrimSpace(board)
	branch := githublistsDefaultBranch
	if i := strings.LastIndex(b, "@"); i >= 0 {
		branch = b[i+1:]
		b = b[:i]
	}
	owner, repo, ok := strings.Cut(b, "/")
	if !ok || !githublistsSegment.MatchString(owner) || !githublistsSegment.MatchString(repo) ||
		!githublistsSegment.MatchString(branch) {
		return githublistsRepo{}, fmt.Errorf("githublists: board %q is not owner/repo[@branch]", board)
	}
	return githublistsRepo{Owner: owner, Repo: repo, Branch: branch}, nil
}

func (r githublistsRepo) url() string {
	return fmt.Sprintf(githublistsRawURL, r.Owner, r.Repo, r.Branch)
}

// shape reads the repo name for what every entry of the list is: an internship list yields
// internships at intern seniority, a new-grad list full-time junior roles. Any other list
// states neither and leaves both to the dictionaries.
func (r githublistsRepo) shape() (employmentType, seniority string) {
	name := strings.ToLower(r.Repo)
	switch {
	case strings.Contains(name, "intern"):
		return "internship", "intern"
	case strings.Contains(name, "new-grad"), strings.Contains(name, "newgrad"):
		return "full_time", "junior"
	}
	return "", ""
}

// githublistsEntry is one row of listings.json. The two flags are pointers because a missing
// flag and a false one mean different things (see live).
type githublistsEntry struct {
	ID          string   `json:"id"`
	CompanyName string   `json:"company_name"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Locations   []string `json:"locations"`
	Active      *bool    `json:"active"`
	IsVisible   *bool    `json:"is_visible"`
	DatePosted  float64  `json:"date_posted"`
	DateUpdated float64  `json:"date_updated"`
	Terms       []string `json:"terms"`
	Season      string   `json:"season"`
	Sponsorship string   `json:"sponsorship"`
	Degrees     []string `json:"degrees"`
	Category    string   `json:"category"`
}

// live reports whether an entry is a current, usable posting: identified, attributed, visible,
// active, linked to a real http(s) page, and updated inside githublistsMaxAge. A missing
// active flag is read as inactive — the lists always state it, so its absence is a row this
// adapter does not understand rather than one it may store.
func (e githublistsEntry) live(now time.Time) bool {
	if e.ID == "" || strings.TrimSpace(e.CompanyName) == "" || strings.TrimSpace(e.Title) == "" {
		return false
	}
	if e.IsVisible != nil && !*e.IsVisible {
		return false
	}
	if e.Active == nil || !*e.Active {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(e.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	updated := e.DateUpdated
	if updated == 0 {
		updated = e.DatePosted
	}
	if updated > 0 && now.Sub(time.Unix(int64(updated), 0)) > githublistsMaxAge {
		return false
	}
	return true
}

// notes is the list's own structured statement about an entry, one line each: the terms it
// hires for, its category, the degrees it asks for, and what it says about sponsorship.
func (e githublistsEntry) notes(repo githublistsRepo) string {
	var lines []string
	if terms := e.termsLine(repo); terms != "" {
		lines = append(lines, terms)
	}
	if c := strings.TrimSpace(e.Category); c != "" {
		lines = append(lines, "Category: "+c)
	}
	if d := githublistsJoin(e.Degrees); d != "" {
		lines = append(lines, "Degrees: "+d)
	}
	if s := githublistsSponsorship[strings.TrimSpace(e.Sponsorship)]; s != "" {
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}

// termsLine states the hiring term. A new-grad list has no term field and simply is one; an
// internship list states terms[] (Simplify) or a bare season whose year is in the repo name
// (vanshb03).
func (e githublistsEntry) termsLine(repo githublistsRepo) string {
	if employment, _ := repo.shape(); employment == "full_time" {
		return "Terms: New Grad"
	}
	if terms := githublistsJoin(e.Terms); terms != "" {
		return "Terms: " + terms
	}
	if season := strings.TrimSpace(e.Season); season != "" {
		return "Terms: " + strings.TrimSpace(season+" "+githublistsYear.FindString(repo.Repo))
	}
	return ""
}

// githublistsJoin joins the non-empty, trimmed values with ", ".
func githublistsJoin(values []string) string {
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}

// toJob maps a live entry to its list-only Job: everything the list states, with the notes as
// the body until a real one is read.
func (e githublistsEntry) toJob(repo githublistsRepo) Job {
	employment, seniority := repo.shape()
	return Job{
		ExternalID:     e.ID,
		URL:            strings.TrimSpace(e.URL),
		Title:          strings.TrimSpace(e.Title),
		Company:        strings.TrimSpace(e.CompanyName),
		Location:       githublistsLocation(e.Locations),
		Description:    sanitizeHTML(plainTextToHTML(e.notes(repo))),
		Remote:         isRemote(githublistsLocation(e.Locations)),
		PostedAt:       parseEpochSeconds(int64(e.DatePosted)),
		EmploymentType: employment,
		Seniority:      seniority,
	}
}

func githublistsLocation(locations []string) string {
	var out []string
	for _, l := range locations {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "; ")
}

// list reads the board's file and keeps its live entries. The file is decoded as a stream so
// a 13 MB list never sits in memory twice.
func (s githublists) list(ctx context.Context, e CompanyEntry) (githublistsRepo, []githublistsEntry, error) {
	repo, err := githublistsBoard(e.Board)
	if err != nil {
		return githublistsRepo{}, nil, err
	}
	now := s.now()
	var live []githublistsEntry
	seen := map[string]bool{}
	err = s.http.GetStream(ctx, repo.url(), "application/json", func(r io.Reader) error {
		dec := json.NewDecoder(r)
		if _, err := dec.Token(); err != nil { // the opening '['
			return fmt.Errorf("githublists: %s: stream open: %w", e.Board, err)
		}
		for dec.More() {
			var entry githublistsEntry
			if err := dec.Decode(&entry); err != nil {
				// A list is either whole or not read at all: a decode error mid-file is a
				// truncated or reshaped file, and a fullCatalog source must not report a
				// partial read as the catalogue.
				return fmt.Errorf("githublists: %s: decode entry: %w", e.Board, err)
			}
			if !entry.live(now) || seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			live = append(live, entry)
		}
		return nil
	})
	if err != nil {
		return githublistsRepo{}, nil, err
	}
	return repo, live, nil
}

func (s githublists) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// List-only fallback (no seen set): hydrate every entry.
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew is the hydrating crawl: the whole list every run, a posting page only for an entry
// the catalogue does not already hold.
func (s githublists) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	repo, entries, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, repo, entries, seen, nil), nil
}

// FetchNewGated is FetchNew told which employers the coverage gate will discard, so their
// bodies are never bought. The posting is still yielded, body-less, so the gate sees and counts
// it — dropping it here would be the adapter making the pipeline's decision.
func (s githublists) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool,
	covered func(companies []string) map[string]bool) ([]Job, error) {
	repo, entries, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, en := range entries {
		names = append(names, strings.TrimSpace(en.CompanyName))
	}
	return s.hydrate(ctx, repo, entries, seen, covered(names)), nil
}

// hydrate maps the live entries to Jobs, reading a body only where one is needed: not for a
// stored entry (a liveness refresh instead), not for a covered employer, and — when the page
// carries no JobPosting block — not at all, in which case the list's notes stay the body. The
// list is authoritative for the posting existing, so a failed read keeps it.
func (s githublists) hydrate(ctx context.Context, repo githublistsRepo, entries []githublistsEntry,
	seen func(externalID string) bool, skip map[string]bool) []Job {
	return fetchDetails(entries, defaultDetailWorkers, func(en githublistsEntry) (Job, bool) {
		base := en.toJob(repo)
		if seen != nil && seen(en.ID) {
			base.SeenRefresh = true
			return base, true
		}
		if skip[base.Company] {
			return base, true
		}
		body, ok := s.body(ctx, base.URL)
		if !ok {
			return base, true
		}
		base.Description = sanitizeHTML(plainTextToHTML(en.notes(repo)) + body)
		return base, true
	})
}

// body reads the employer's posting page and returns the description of the first schema.org
// JobPosting it embeds. ok is false when the page cannot be read, carries no such block, or
// the block has no description.
func (s githublists) body(ctx context.Context, link string) (string, bool) {
	root, err := s.http.GetHTML(ctx, link)
	if err != nil {
		log.Printf("githublists: posting page %s failed; keeping the list's notes: %v", link, err)
		return "", false
	}
	var ld struct {
		Description string `json:"description"`
	}
	if !LDJobPosting(root, &ld) || strings.TrimSpace(ld.Description) == "" {
		return "", false
	}
	return ld.Description, true
}

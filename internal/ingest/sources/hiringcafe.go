package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// hiringcafe adapts hiring.cafe (hiringcafe.com), an aggregator that indexes postings straight
// off employers' own career sites and ATS boards — every hit carries the employer's own
// apply_url, so a copy of a posting freehire already crawls first-party dedups against it the
// way the aggregator gate expects.
//
// # The board is a SEARCH KEYWORD and the region a country
//
// There is no per-employer board: one central index answers a search, so an entry's board is
// the keyword ("software engineer") and its region an alpha-2 country code that becomes the
// search's location filter — the whatjobs/jobleads shape. An empty region searches worldwide.
// The country filter is a Google-Places-shaped object the site's own frontend builds, and the
// only part of it this adapter cannot derive from the code is the country's display name, so
// hiringcafeCountryNames lists the ones onboarded; a region outside it fails the board loudly
// rather than searching the world under a US label.
//
// # What was measured (2026-09-14, residential egress)
//
//   - Every path answers Cloudflare's "Just a moment..." JS challenge (403) to Go's default
//     TLS fingerprint, curl included, and the old hiring.cafe host 308s here. The Chrome
//     fingerprint transport (fingerprintHTTP, the meta/bayt/jobleads cluster) is served 200 with
//     no challenge at all, paced at ~1 req/s. The JSON search endpoint the site's SPA uses
//     (/api/search-jobs) is refused outright, so the crawl reads the server-rendered page: the
//     search page embeds a `<script id="__NEXT_DATA__">` whose props.pageProps carries ssrHits.
//     The production datacenter IP has NOT been measured; if the edge refuses it, the one-line
//     answer is a firecrawlProviders entry, exactly as wellfound is wired.
//   - A hit carries NO body (`ssrUnpackJobCards` is false on the search page; 0 of 96 hits had a
//     description). The body is HTML on the posting's own page, /job/<requisition_id>, under
//     props.pageProps.job — hence HydratingSource, so a stored posting costs no second request.
//   - `sortBy: "date"` orders newest-first and is what makes a page cap meaningful; the default
//     relevance order surfaced 2015 postings on page 0. `dateFetchedPastNDays` did NOT narrow
//     the stated total (95,177 for "software engineer"/US at both 7 and 1 days), so the window
//     is the newest hiringcafeMaxPages pages of a keyword, not a date range — a slice, the
//     reason for the wide sweepGrace. `ssrPageSize` says 40 while a page carries ~100-125 hits
//     (the site groups a company's cards), so the walk counts what it collected, not the size.
//   - `ssrTotalCount` is huge and unverified as a pagination bound; the walk ends on
//     `ssrIsLastPage`, on a page that adds nothing new, or at the cap.
//   - `is_expired` is stated on both the hit and the posting page, and an expired posting page
//     titles itself "HiringCafe - Expired Job"; both are read.
type hiringcafe struct {
	http HTMLGetter
}

// NewHiringCafe builds the hiring.cafe adapter over the given HTML transport — the paced Chrome
// fingerprint client in the crawl registry, nil on the taxonomy path.
func NewHiringCafe(c HTMLGetter) Source { return hiringcafe{http: c} }

func (hiringcafe) Provider() string { return "hiringcafe" }

// hiringcafe aggregates postings from many employers, each named on its own hit, so it stays in
// the source facet and its copies yield to a first-party ATS posting. It is NOT boardless: the
// keyword is what selects the slice.
func (hiringcafe) aggregator() {}

// The crawl reads only the newest hiringcafeMaxPages pages of a keyword, so a live posting that
// drifts past that depth reads as unseen; on the 48h default it would be closed and reopened
// as newer postings arrive. Two weeks outlasts that drift — the whatjobs/jobleads reasoning.
func (hiringcafe) sweepGrace() time.Duration { return hiringcafeSweepGrace }

const (
	hiringcafeBaseURL = "https://hiringcafe.com/"
	// hiringcafeJobURL is a posting's own page, keyed by the hit's requisition_id (not its id,
	// which is the source-scoped composite the catalogue stores as external_id).
	hiringcafeJobURL = "https://hiringcafe.com/job/%s"
	// hiringcafeMaxPages bounds one keyword's walk. At ~100 hits a page this is roughly the
	// newest 500 postings of a keyword, which is far more than the hourly cadence adds to any
	// one keyword between runs — and what bounds a FIRST crawl: every hit is hydrated at the
	// shared ~1 req/s, the scheduler kills a provider's run at DefaultRunTimeout (50 min), and
	// a board whose walk is cut loses its buffered postings, so a handful of new keyword
	// boards must each fit inside one run with room to spare.
	hiringcafeMaxPages = 5
	// hiringcafeSortBy orders the index newest-first (see the type doc).
	hiringcafeSortBy = "date"
	// hiringcafeRecentDays is passed because the site's own frontend always sends it; it did
	// not narrow the total when measured, so nothing here relies on it.
	hiringcafeRecentDays = 30
	hiringcafeSweepGrace = 14 * 24 * time.Hour
	// hiringcafeRequestInterval paces every request on one shared limiter. From residential
	// egress 800 ms spacing (~1.25 req/s) was served clean over 15 listing pages and detail
	// pages alike. From the production datacenter address the edge blocks after roughly FIFTY
	// requests in a five-minute window and then answers 429 to everything for as long as the
	// crawl keeps asking: 65 requests in 50 s at 800 ms (2026-09-15 06:29 UTC) and 47 in
	// 185 s at 4 s (06:55 UTC) were both refused at about that count, while 20 listing pages
	// at 4 s were always served. Eight seconds keeps a run under 40 requests per five minutes;
	// the budget and the breaker below bound what one run can spend if the window is tighter
	// still, and board_health is where to read whether it is.
	hiringcafeRequestInterval = 8 * time.Second
	hiringcafeRequestBurst    = 1
	// hiringcafeDetailWorkers bounds the detail pool. The limiter sets the pace, not the pool;
	// a narrow pool only keeps the number of retry ladders in flight small when the edge
	// starts refusing, so the breaker trips after a handful of requests rather than dozens.
	hiringcafeDetailWorkers = 2
	// hiringcafeExpiredTitle is the fragment of the page title an expired posting renders.
	hiringcafeExpiredTitle = "Expired Job"
)

// errHiringcafeGone is a posting page that answered but carries no live posting: expired by
// the site's own flag or title, gone (404/410), or without a body. It is not a transport
// failure and never trips the breaker.
var errHiringcafeGone = errors.New("hiringcafe: posting gone or empty")

// hiringcafeRetryDelays is the back-off ladder for a request the edge refused as a burst
// (429, or the 403 a challenge arrives under): one retry after each delay, then give up. It is
// a var so a test can shorten it.
var hiringcafeRetryDelays = []time.Duration{5 * time.Second, 15 * time.Second}

// hiringcafeMaxNewPerRun caps how many NEW, uncovered hits one board hydrates per run. The
// listing is newest-first, so the budget always buys the freshest postings; what it leaves
// stays new and is bought on a later run. Four boards × (5 listing pages + 50 detail pages)
// at 8 s is ~30 minutes, inside the scheduler's 50-minute run, and steady state (only what
// an hour adds, minus the covered employers the gate discards) is a fraction of that. A var
// so a test can narrow it.
var hiringcafeMaxNewPerRun int64 = 50

// hiringcafeCountryNames maps an entry's region (alpha-2) onto the display name the search's
// location filter carries. Onboarding a market is one row here plus its boards.
var hiringcafeCountryNames = map[string]string{
	"US": "United States",
	"CA": "Canada",
	"GB": "United Kingdom",
	"IE": "Ireland",
	"DE": "Germany",
	"NL": "Netherlands",
	"FR": "France",
	"ES": "Spain",
	"PL": "Poland",
	"IN": "India",
	"AU": "Australia",
	"SG": "Singapore",
	"BR": "Brazil",
}

// hiringcafeSearchState is the site's own searchState query parameter, reduced to the fields
// that select a slice. The frontend sends many more (seniority, commitment, clearance lists);
// omitting them is answered exactly like sending their all-inclusive defaults, verified live.
type hiringcafeSearchState struct {
	SearchQuery           string               `json:"searchQuery"`
	SortBy                string               `json:"sortBy"`
	DateFetchedPastNDays  int                  `json:"dateFetchedPastNDays"`
	WorkplaceTypes        []string             `json:"workplaceTypes"`
	DefaultToUserLocation bool                 `json:"defaultToUserLocation"`
	UserLocation          *string              `json:"userLocation"`
	Locations             []hiringcafeLocation `json:"locations"`
}

// hiringcafeLocation is the country filter in the Google-Places shape the frontend builds.
type hiringcafeLocation struct {
	FormattedAddress  string                       `json:"formatted_address"`
	Types             []string                     `json:"types"`
	ID                string                       `json:"id"`
	AddressComponents []hiringcafeAddressComponent `json:"address_components"`
	Options           hiringcafeLocationOptions    `json:"options"`
}

type hiringcafeAddressComponent struct {
	LongName  string   `json:"long_name"`
	ShortName string   `json:"short_name"`
	Types     []string `json:"types"`
}

type hiringcafeLocationOptions struct {
	FlexibleRegions []string `json:"flexible_regions"`
}

// hiringcafeState builds the serialized search state for an entry: the board is the keyword,
// the region (optional) the country. An unknown region is an error rather than a worldwide
// search, so a mistyped code cannot file the world's postings under one country's board.
func hiringcafeState(e CompanyEntry) (string, error) {
	keyword := strings.TrimSpace(e.Board)
	if keyword == "" {
		return "", fmt.Errorf("hiringcafe: company %q has no search keyword (board)", e.Company)
	}
	state := hiringcafeSearchState{
		SearchQuery:          keyword,
		SortBy:               hiringcafeSortBy,
		DateFetchedPastNDays: hiringcafeRecentDays,
		WorkplaceTypes:       []string{"Remote", "Hybrid", "Onsite"},
		// An empty filter is what the site's own frontend sends for a worldwide search; nil
		// would serialize as null, which is not a shape it has been seen to accept.
		Locations: []hiringcafeLocation{},
	}
	if region := strings.ToUpper(strings.TrimSpace(e.Region)); region != "" {
		name, ok := hiringcafeCountryNames[region]
		if !ok {
			return "", fmt.Errorf("hiringcafe: region %q is not in hiringcafeCountryNames", e.Region)
		}
		state.Locations = []hiringcafeLocation{{
			FormattedAddress: name,
			Types:            []string{"country"},
			ID:               "user_country",
			AddressComponents: []hiringcafeAddressComponent{{
				LongName: name, ShortName: region, Types: []string{"country"},
			}},
			Options: hiringcafeLocationOptions{
				FlexibleRegions: []string{"anywhere_in_continent", "anywhere_in_world"},
			},
		}}
	}
	b, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("hiringcafe: encode search state: %w", err)
	}
	return string(b), nil
}

// hiringcafeSearchURL is one page of a search. Page 0 is the bare search; the site counts
// pages from 0 and puts a later page in ?page=N.
func hiringcafeSearchURL(state string, page int) string {
	u := hiringcafeBaseURL + "?searchState=" + url.QueryEscape(state)
	if page > 0 {
		u += fmt.Sprintf("&page=%d", page)
	}
	return u
}

// hiringcafeNextData is the shape of the embedded __NEXT_DATA__ this adapter reads.
type hiringcafeNextData struct {
	Props struct {
		PageProps hiringcafePage `json:"pageProps"`
	} `json:"props"`
}

// hiringcafePage is props.pageProps on either page: the search page fills Hits/IsLastPage,
// the posting page fills Job. Error is the search's own server-side failure, null when fine.
type hiringcafePage struct {
	Hits       []hiringcafeHit `json:"ssrHits"`
	IsLastPage bool            `json:"ssrIsLastPage"`
	Error      any             `json:"ssrError"`
	Job        *hiringcafeHit  `json:"job"`
}

// hiringcafeHit is one indexed posting. job_information carries the title and — on the
// posting page only — the HTML body; v5_processed_job_data is the site's structured reading
// of the posting, which is where every facet this adapter maps comes from.
type hiringcafeHit struct {
	ID              string              `json:"id"`
	RequisitionID   string              `json:"requisition_id"`
	ApplyURL        string              `json:"apply_url"`
	BoardToken      string              `json:"board_token"`
	IsExpired       bool                `json:"is_expired"`
	JobInformation  hiringcafeJobInfo   `json:"job_information"`
	Processed       hiringcafeProcessed `json:"v5_processed_job_data"`
	EnrichedCompany hiringcafeCompany   `json:"enriched_company_data"`
}

type hiringcafeJobInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type hiringcafeCompany struct {
	Name string `json:"name"`
}

// hiringcafeProcessed is the structured slice of v5_processed_job_data. Compensation comes as
// one pair per frequency with the stated frequency naming which pair the posting filled.
type hiringcafeProcessed struct {
	CoreJobTitle               string   `json:"core_job_title"`
	CompanyName                string   `json:"company_name"`
	FormattedWorkplaceLocation string   `json:"formatted_workplace_location"`
	WorkplaceCountries         []string `json:"workplace_countries"`
	WorkplaceType              string   `json:"workplace_type"`
	Commitment                 []string `json:"commitment"`
	SeniorityLevel             string   `json:"seniority_level"`
	TechnicalTools             []string `json:"technical_tools"`
	EstimatedPublishDate       string   `json:"estimated_publish_date"`
	MinYearsOfExperience       *float64 `json:"min_industry_and_role_yoe"`
	CompensationCurrency       string   `json:"listed_compensation_currency"`
	CompensationFrequency      string   `json:"listed_compensation_frequency"`
	YearlyMin                  *float64 `json:"yearly_min_compensation"`
	YearlyMax                  *float64 `json:"yearly_max_compensation"`
	MonthlyMin                 *float64 `json:"monthly_min_compensation"`
	MonthlyMax                 *float64 `json:"monthly_max_compensation"`
	DailyMin                   *float64 `json:"daily_min_compensation"`
	DailyMax                   *float64 `json:"daily_max_compensation"`
	HourlyMin                  *float64 `json:"hourly_min_compensation"`
	HourlyMax                  *float64 `json:"hourly_max_compensation"`
}

// hiringcafeParse reads a fetched page's embedded payload. A page with no __NEXT_DATA__ is an
// error rather than an empty page: that is exactly what a challenge interstitial looks like,
// and reading it as "no postings" is the silent outage the browser-tier note warns about.
func hiringcafeParse(root *html.Node) (hiringcafePage, error) {
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return hiringcafePage{}, fmt.Errorf("hiringcafe: __NEXT_DATA__ script not found (a challenge page carries none)")
	}
	var nd hiringcafeNextData
	if err := json.Unmarshal([]byte(raw), &nd); err != nil {
		return hiringcafePage{}, fmt.Errorf("hiringcafe: decode __NEXT_DATA__: %w", err)
	}
	if nd.Props.PageProps.Error != nil {
		return hiringcafePage{}, fmt.Errorf("hiringcafe: server-side search failed: %v", nd.Props.PageProps.Error)
	}
	return nd.Props.PageProps, nil
}

// get fetches one page, retrying a refusal the edge answers to a burst — 429, or the 403 a
// managed challenge arrives under — once per rung of hiringcafeRetryDelays. The retry sits
// beneath the shared limiter, which is why the ladder is short: a refused burst is what the
// pace exists to avoid, and hammering through it only extends the penalty window.
func (s hiringcafe) get(ctx context.Context, u string) (*html.Node, error) {
	root, err := s.http.GetHTML(ctx, u)
	for _, delay := range hiringcafeRetryDelays {
		if err == nil || !isRateLimited(err) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		root, err = s.http.GetHTML(ctx, u)
	}
	return root, err
}

// page fetches and parses one search page.
func (s hiringcafe) page(ctx context.Context, state string, page int) (hiringcafePage, error) {
	root, err := s.get(ctx, hiringcafeSearchURL(state, page))
	if err != nil {
		return hiringcafePage{}, err
	}
	return hiringcafeParse(root)
}

// list walks a keyword's newest pages, deduplicating hits across pages (a company's grouped
// cards can straddle a boundary). The first page failing is a board-level error; a later one
// ends the walk with what it gathered, and says so, since a truncated slice reads downstream
// as a keyword that shrank.
func (s hiringcafe) list(ctx context.Context, e CompanyEntry) ([]hiringcafeHit, error) {
	state, err := hiringcafeState(e)
	if err != nil {
		return nil, err
	}
	var out []hiringcafeHit
	collected := map[string]bool{}
	for page := 0; page < hiringcafeMaxPages; page++ {
		pp, err := s.page(ctx, state, page)
		if err != nil {
			if page == 0 {
				return nil, fmt.Errorf("hiringcafe: keyword %q page 0: %w", e.Board, err)
			}
			log.Printf("hiringcafe: keyword %q truncated at page %d with %d postings: %v",
				e.Board, page, len(out), err)
			return out, nil
		}
		added := 0
		for _, h := range pp.Hits {
			if h.ID == "" || collected[h.ID] {
				continue
			}
			collected[h.ID] = true
			out = append(out, h)
			added++
		}
		if added == 0 || pp.IsLastPage {
			return out, nil
		}
	}
	log.Printf("hiringcafe: keyword %q still had new postings at the %d-page cap (%d collected); "+
		"the slice is the keyword's newest postings, not the whole keyword", e.Board, hiringcafeMaxPages, len(out))
	return out, nil
}

func (s hiringcafe) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// List-only fallback (no seen set): hydrate every posting.
	hits, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, e, hits, nil, nil)
}

// FetchNew is the hydrating crawl: the keyword's newest pages are listed every run, but a
// posting page is fetched only for a hit the catalogue does not already hold. A seen hit is
// re-listed as a liveness refresh with no request and no content rewrite.
func (s hiringcafe) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	hits, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, e, hits, seen, nil)
}

// FetchNewGated is FetchNew told which employers the coverage gate will discard, so their
// bodies are never bought. It matters more here than on any other aggregator: hiring.cafe
// indexes the very ATS boards freehire crawls first-party, so most hits are copies the gate
// discards after ingest (2 of 2, 3 of 4 and 1 of 2 on the first bounded production run), and
// the edge's request budget is the scarcest thing this adapter has. A covered hit is still
// yielded, body-less, so the gate sees and counts it.
func (s hiringcafe) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool,
	covered func(companies []string) map[string]bool) ([]Job, error) {
	hits, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		if job, ok := h.toJob(); ok {
			names = append(names, job.Company)
		}
	}
	return s.hydrate(ctx, e, hits, seen, covered(names))
}

// hydrate maps the listed hits to Jobs through a narrow pool (the limiter, not the pool, sets
// the pace), buying a body only where one is needed: not for a stored hit (a liveness refresh
// instead) and not for a covered employer (yielded body-less). A hit whose body could not be
// read is DROPPED rather than stored list-only: a stored row is re-offered for hydration only
// inside pipeline.HydrationRetryWindow, after which it is `seen` forever with no body, while a
// dropped one stays new and costs one request on the next crawl — the seek asymmetry, and the
// same cause (refusals arrive in bursts).
//
// Two things bound what a run spends. The budget: only the first hiringcafeMaxNewPerRun new
// hits of a board are attempted, newest first. The breaker: once a refusal has survived the
// retry ladder, no further detail request is made this run — every remaining new hit is
// dropped without a request, since retrying a refused request is exactly what holds the
// wall up (the production run of 2026-09-15 kept re-earning its 429 for six minutes that way).
// A board that read at least one body still succeeds with what it read; a crawl that listed
// new postings and read none of them is a failure, not an empty source — that is what a wall
// looks like from the outside.
func (s hiringcafe) hydrate(ctx context.Context, e CompanyEntry, hits []hiringcafeHit,
	seen func(externalID string) bool, skip map[string]bool) ([]Job, error) {
	var attempted, read atomic.Int64
	var walled atomic.Bool
	jobs := fetchDetails(hits, hiringcafeDetailWorkers, func(h hiringcafeHit) (Job, bool) {
		base, ok := h.toJob()
		if !ok {
			return Job{}, false
		}
		if seen != nil && seen(h.ID) {
			base.SeenRefresh = true
			return base, true
		}
		if skip[base.Company] {
			return base, true // covered by the employer's own ATS; a body would be pure loss
		}
		if walled.Load() || attempted.Add(1) > hiringcafeMaxNewPerRun {
			return Job{}, false // stays new; a later run buys it
		}
		body, err := s.detail(ctx, h)
		if err != nil {
			if isRateLimited(err) && walled.CompareAndSwap(false, true) {
				log.Printf("hiringcafe: keyword %q: the edge refused a detail page past the retry ladder; "+
					"no further detail requests this run: %v", e.Board, err)
			} else if !errors.Is(err, errHiringcafeGone) && !isRateLimited(err) {
				log.Printf("hiringcafe: detail %s failed; deferring to the next crawl: %v", h.ID, err)
			}
			return Job{}, false
		}
		read.Add(1)
		base.Description = body
		return base, true
	})
	if n := min(attempted.Load(), hiringcafeMaxNewPerRun); n > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("hiringcafe: keyword %q attempted %d new postings and read none of their bodies (walled=%v)",
			e.Board, n, walled.Load())
	}
	return jobs, nil
}

// detail reads one posting's page and returns its sanitized HTML body. It returns
// errHiringcafeGone for a page that answered without a live posting — expired by flag or title,
// 404/410, no body — and the transport's own error otherwise, so the caller can tell a refusal
// (which trips the breaker) from a posting that is simply not there.
func (s hiringcafe) detail(ctx context.Context, h hiringcafeHit) (string, error) {
	if h.RequisitionID == "" {
		return "", errHiringcafeGone
	}
	root, err := s.get(ctx, fmt.Sprintf(hiringcafeJobURL, url.PathEscape(h.RequisitionID)))
	if err != nil {
		if !detailUnreadable(err) {
			return "", errHiringcafeGone // 404/410: the platform's own answer
		}
		return "", err
	}
	if strings.Contains(titleText(root), hiringcafeExpiredTitle) {
		return "", errHiringcafeGone
	}
	pp, err := hiringcafeParse(root)
	if err != nil {
		return "", err
	}
	if pp.Job == nil || pp.Job.IsExpired {
		return "", errHiringcafeGone
	}
	body := sanitizeHTML(pp.Job.JobInformation.Description)
	if strings.TrimSpace(body) == "" {
		return "", errHiringcafeGone
	}
	return body, nil
}

// toJob maps a hit's listing fields to a Job without its body. ok is false for an expired hit
// or one the catalogue cannot key, attribute or link: no id, no employer name anywhere, no
// title, or neither an apply link nor a requisition id to point at.
func (h hiringcafeHit) toJob() (Job, bool) {
	if h.ID == "" || h.IsExpired {
		return Job{}, false
	}
	p := h.Processed
	company := strings.TrimSpace(firstNonEmpty(p.CompanyName, h.EnrichedCompany.Name, h.BoardToken))
	title := strings.TrimSpace(firstNonEmpty(h.JobInformation.Title, p.CoreJobTitle))
	if company == "" || title == "" {
		return Job{}, false
	}
	link := strings.TrimSpace(h.ApplyURL)
	if link == "" {
		if h.RequisitionID == "" {
			return Job{}, false
		}
		link = fmt.Sprintf(hiringcafeJobURL, url.PathEscape(h.RequisitionID))
	}
	workMode := hiringcafeWorkMode(p.WorkplaceType)
	job := Job{
		ExternalID:         h.ID,
		URL:                link,
		Title:              title,
		Company:            company,
		Location:           strings.TrimSpace(p.FormattedWorkplaceLocation),
		Remote:             workMode == "remote",
		WorkMode:           workMode,
		PostedAt:           NotFuture(parseRFC3339(p.EstimatedPublishDate)),
		Countries:          hiringcafeCountries(p.WorkplaceCountries),
		Seniority:          hiringcafeSeniority(p.SeniorityLevel),
		EmploymentType:     hiringcafeEmploymentType(p.Commitment),
		Skills:             skilltag.Canonicalize(p.TechnicalTools),
		ExperienceYearsMin: hiringcafeYears(p.MinYearsOfExperience),
	}
	p.applySalary(&job)
	return job, true
}

// hiringcafeWorkMode maps the site's closed workplace_type enum (Remote/Hybrid/Onsite) onto
// freehire's work-mode vocabulary; anything else yields "" so the location heuristic decides.
func hiringcafeWorkMode(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "onsite":
		return "onsite"
	}
	return ""
}

// hiringcafeCountries resolves the posting's structured alpha-2 codes, deduplicated in order.
func hiringcafeCountries(codes []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, code := range codes {
		for _, c := range countryFromCode(code) {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// hiringcafeSeniority maps the site's four-value seniority picklist onto freehire's. "No Prior
// Experience Required" and "Entry Level" are both junior; there is no lead/staff rung to map.
func hiringcafeSeniority(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "no prior experience required", "entry level":
		return "junior"
	case "mid level":
		return "middle"
	case "senior level":
		return "senior"
	}
	return ""
}

// hiringcafeEmploymentType maps a single stated commitment onto freehire's vocabulary. A
// posting stating several names no one type and yields "" (the edjoin rule); Temporary,
// Seasonal and Volunteer have no freehire value.
func hiringcafeEmploymentType(commitment []string) string {
	if len(commitment) != 1 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(commitment[0])) {
	case "full time":
		return "full_time"
	case "part time":
		return "part_time"
	case "contract":
		return "contract"
	case "internship":
		return "internship"
	}
	return ""
}

// hiringcafeYears reads the stated minimum years of experience, nil when unstated.
func hiringcafeYears(v *float64) *int {
	if v == nil || *v < 0 {
		return nil
	}
	n := int(*v)
	return &n
}

// applySalary copies the posting's structured compensation onto the job, but only as a
// complete statement: the frequency names which of the per-frequency pairs the posting filled,
// the currency must be stated, and at least one bound must be positive. Weekly and bi-weekly
// pairs have no vocab.SalaryPeriodValues member and are dropped rather than published
// half-qualified.
func (p hiringcafeProcessed) applySalary(job *Job) {
	var period string
	var lo, hi *float64
	switch strings.ToLower(strings.TrimSpace(p.CompensationFrequency)) {
	case "yearly":
		period, lo, hi = "year", p.YearlyMin, p.YearlyMax
	case "monthly":
		period, lo, hi = "month", p.MonthlyMin, p.MonthlyMax
	case "daily":
		period, lo, hi = "day", p.DailyMin, p.DailyMax
	case "hourly":
		period, lo, hi = "hour", p.HourlyMin, p.HourlyMax
	default:
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(p.CompensationCurrency))
	if currency == "" {
		return
	}
	min, max := hiringcafeAmount(lo), hiringcafeAmount(hi)
	if min == nil && max == nil {
		return
	}
	job.SalaryMin, job.SalaryMax = min, max
	job.SalaryCurrency, job.SalaryPeriod = currency, period
}

func hiringcafeAmount(v *float64) *int {
	if v == nil {
		return nil
	}
	return roundSalaryPart(*v)
}

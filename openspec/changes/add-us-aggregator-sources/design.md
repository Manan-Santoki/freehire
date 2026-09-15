## Context

Three sources, three transports, three lifecycle shapes — and the point of the design is
that each lands on a shape this package already has rather than inventing one.

| source | board | transport | body | lifecycle |
|---|---|---|---|---|
| hiring.cafe | keyword + country | `fingerprintHTTP`, paced 800 ms | posting page (`HydratingSource`) | slice of a deep index → `sweepGrace` 14 d |
| GitHub lists | `owner/repo` | plain client, `GetStream` | employer's page ld+json (`HydratingSource`, `CoverageGated`) | whole list each run → `fullCatalog` |
| Hacker News | boardless | plain client | the comment itself | two whole threads each run → `fullCatalog` |

## Decisions

### hiring.cafe reads the SSR page over the Chrome fingerprint, not the SPA's JSON endpoint

Measured 2026-09-14: `hiring.cafe/api/search-jobs` answers 405 and the same path on
`hiringcafe.com` the Cloudflare challenge; curl gets the challenge on every path. The
Chrome-fingerprint transport (`tls-client`, Chrome_144 profile) is served the real page,
keyless, at ~1 req/s with no challenge across a 15-page walk plus detail pages. Job-Ops
reaches the same conclusion with a Firefox fingerprint (impit) and 800 ms spacing. So the
adapter takes an `HTMLGetter`, is registered in `All`'s fingerprint block, and is NOT added
to `firecrawlProviders` today: the hosted tier is billed per page and a first keyword crawl
hydrates ~500 of them, while nothing measured says the fingerprint path is refused. The
prod datacenter IP is the unmeasured variable; the fallback is one line.

### The production address is served on a small budget, so a run is bounded three ways

The first production run (2026-09-15) was served ~65 requests in 50 s at 1.25 req/s and
then refused every request for as long as it kept asking; a bounded 4 s run was refused after
~47 requests in three minutes. A gated 8 s run was refused after 47 requests in 380 s, and a run started
eleven minutes after the previous one was served its 47 again: roughly 50 requests per
ten-minute window per address, a count rather than a rate. So: 20 s spacing on one limiter
shared by listing and detail, 3 listing pages a keyword; `CoverageGated`, so a
hit whose employer freehire already crawls first-party (most of them) costs no request; `hiringcafeMaxNewPerRun` new detail pages per board per run
(newest first — the rest stay new for a later run); and a breaker that stops all detail
requests for the run once a refusal has survived the retry ladder, because a retried
refusal is what keeps the block alive. A board keeps what it read; a run that read nothing
reports the wall.

### The keyword slice is newest-first and capped, and that is why the sweep waits 14 days

`sortBy: "date"` orders the index newest-first (the default order surfaced 2015 postings on
page 0). `dateFetchedPastNDays` did not change the stated total at 7 vs 1 days, so it is not a
window the crawl can rely on; `hiringcafeMaxPages` (5 pages, ~500 hits) is — sized so a first crawl of a few keyword
boards, every hit hydrated at ~1 req/s, fits inside the scheduler's 50-minute run. A posting
drifts past that depth as newer ones arrive, so on the 48 h default it would be closed and
reopened — the whatjobs/jobleads reasoning, and the same 14-day answer.

### A hiring.cafe hit whose page could not be read is dropped, not stored list-only

The seek asymmetry: refusals arrive in bursts, and a stored body-less row is re-offered for
hydration only inside `HydrationRetryWindow`, after which it is `seen` forever with no body,
while a dropped hit stays new and costs one request on the next crawl. A crawl that listed
new hits and read none of them fails, per the "reads nothing of what it listed" rule.

### GitHub lists are `CoverageGated`

Most list entries link to Greenhouse/Lever/Ashby/Workday pages of employers freehire crawls
first-party. The pipeline's aggregator gate discards such a copy before it is stored, so it
is never `seen`, so a plain hydrating crawl would buy its body every run — remotedotcom's
measured 71 % waste. `FetchNewGated` yields a covered entry body-less (the gate still counts
it) and reads a body only for the rest.

### The list's own notes are the body when the employer's page has no JobPosting

Terms, category, degrees and the sponsorship line are what the list states; the sponsorship
line ("U.S. citizenship is required for this position.") is what a US job seeker most needs.
`plainTextToHTML` + `sanitizeHTML`, prepended to the ld+json body when one is read.

### Hacker News finds the thread by author tag and reads two threads

`tags=story,author_whoishiring` lists the account's own stories newest-first; free-text
search is ranked and drifts (career-ops hit exactly this). The account posts three threads a
month, so the title is regex-checked. Two threads cover the hand-over at month start. Both
are read whole and any failure fails the crawl, which is what makes `fullCatalog` sound and
is the only thing that closes a boardless feed's aged-out posts.

### A post that does not state an employer is dropped

The header convention is "Company | Role | ...". Fewer than two segments means nothing
separates an employer from a role, and a catalogue row needs both. Segment order does vary
in practice (some posts lead with the location); that noise is accepted and left to the
title dictionaries rather than guessed at.

## Risks / Trade-offs

- **hiring.cafe from the prod IP** is unmeasured. Watch `board_health` on the first runs;
  the `firecrawlProviders` fallback is documented in the adapter and in AGENTS.md.
- **Yield**: over 97% of hits are employers freehire already crawls first-party and the
  gate discards them (the first gated run ingested 15 of ~1,300 listed). hiring.cafe is a
  supplement for employers outside the first-party fleet, not a volume source.
- **Run cost**: 3 listing pages × 4 boards plus the uncovered details at 20 s ≈ 5-10 minutes
  an hour, inside the edge's ~50-per-ten-minutes budget. The scheduler kills a run at 50
  minutes and a board cut mid-walk saves nothing, so add keyword boards a few at a time.
- **GitHub list duplicates**: the two intern lists overlap heavily; both are boards under one
  provider, so the same posting can be stored under two external-id namespaces. The
  duplicate markers collapse the pair in search, as for schoolspring's keyword overlap.

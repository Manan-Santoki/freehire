## 1. Live probes

- [x] 1.1 hiring.cafe: confirm the challenge on plain/curl, the 200 through `fingerprintHTTP`,
      the `__NEXT_DATA__` shape (`ssrHits`, `ssrIsLastPage`, `pageProps.job`), that hits carry
      no body, that `sortBy: "date"` orders newest-first, and the per-page hit count
- [x] 1.2 Hacker News: confirm the author-tag search lists the sibling threads, and the items
      API's top-level `children` shape (null `text` on a deleted comment)
- [x] 1.3 GitHub lists: confirm `listings.json` on `dev`, its size, the entry shape, and the
      active/visible counts

## 2. hiring.cafe adapter

- [x] 2.1 Search state from board + region (`hiringcafeCountryNames`; unknown region fails)
- [x] 2.2 Page walk newest-first, deduplicated, ending on `ssrIsLastPage` / nothing added / cap
- [x] 2.3 Challenge page (no `__NEXT_DATA__`) on page 0 fails the board
- [x] 2.4 `HydratingSource`: seen → `SeenRefresh`; detail from `/job/<requisition_id>`;
      expired (flag or title) and unreadable hits dropped; listed-but-read-none fails
- [x] 2.5 Structured facets: work mode, countries, seniority, single commitment, per-frequency
      salary, skills, years of experience
- [x] 2.6 Typed refusals from `fingerprintHTTP`; 429/403 retried on `hiringcafeRetryDelays`
- [x] 2.7 Register in the fingerprint block (taxonomy path with nil), 14-day `sweepGrace`

## 3. GitHub lists adapter

- [x] 3.1 Board parsing `owner/repo[@branch]`, default `dev`
- [x] 3.2 Streaming decode of `listings.json`; a decode error fails the board
- [x] 3.3 Live filter: active, visible, http(s) URL, updated inside 60 days
- [x] 3.4 Notes body (terms / category / degrees / sponsorship); ld+json body prepended with it
- [x] 3.5 `FetchNew` and `FetchNewGated`; repo-name shape (internship / new-grad)
- [x] 3.6 Register; `aggregator`, `fullCatalog`, `CoverageGated`

## 4. Hacker News adapter

- [x] 4.1 Thread discovery by author tag, title-checked, two newest
- [x] 4.2 Header parse (company | role | location, commitment and salary segments skipped),
      first-link URL with permalink fallback, whole comment as body
- [x] 4.3 Any thread failure fails the crawl; register as boardless `aggregator` + `fullCatalog`

## 5. Wiring and docs

- [x] 5.1 `make gen-contracts` (source facet carries the three keys)
- [x] 5.2 AGENTS.md trap entries for all three
- [x] 5.3 After deploy, seed boards — done 2026-09-15 06:27 UTC through the Dokploy schedule
      `seed-us-aggregator-boards` (one boardless `hackernews` row, the three GitHub repos, and
      four `hiringcafe` US keyword boards: software engineer, software engineer intern,
      machine learning engineer, data engineer; ids 157555-157562). Further keywords: edit
      that schedule's command and run it again, a few at a time.
- [x] 5.4 Watch the first production runs: `hackernews` listed 417 posts, ingested 244, 171
      skipped as first-party-covered, 0 failed. `hiringcafe` from the production IP: the
      listings were served (all four keywords walked to the 5-page cap in ~16 s, no challenge),
      but detail pages were refused with 429 after ~65 requests in 50 s at 1.25 req/s and the
      refusal held while the run kept retrying — fixed by the 2 s pace, the 100-per-board
      per-run budget and the breaker (2.8). No `firecrawlProviders` entry is needed.
- [x] 2.8 Pace 8 s (the edge allows ~50 requests per five minutes from the production
      address), per-run new-detail budget, two-worker pool, the refusal breaker, and
      `CoverageGated` so covered employers' bodies are never bought

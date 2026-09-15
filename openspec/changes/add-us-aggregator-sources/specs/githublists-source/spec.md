## Purpose

Crawl the curated GitHub job-list repositories (SimplifyJobs, vanshb03) from the
machine-readable `listings.json` each renders its README from, hydrating bodies from the
employer's own posting page and never buying a body for an employer the catalogue already
covers first-party.

## ADDED Requirements

### Requirement: Repository-board crawl of listings.json

The system SHALL provide a `githublists` source adapter whose board is `owner/repo` (optionally
`owner/repo@branch`, default branch `dev`) and which reads
`.github/scripts/listings.json` as a stream. An entry SHALL be yielded only when it is
identified, attributed, visible, active, linked to an http(s) page, and updated within the
adapter's staleness window. A decode failure anywhere in the file SHALL fail the board.

#### Scenario: Only live entries are yielded

- **WHEN** the file carries an active entry, an inactive one, a hidden one and a stale one
- **THEN** exactly the active, visible, fresh entry is yielded

#### Scenario: A truncated file fails the board

- **WHEN** the file ends mid-entry
- **THEN** the crawl fails rather than reporting the entries before the cut

### Requirement: Bodies from the employer's page, gated by coverage

The adapter SHALL be a `HydratingSource` and `CoverageGated`: a new entry's body is read from
its URL's embedded schema.org JobPosting, prefixed by the list's own notes (terms, category,
degrees, sponsorship); an entry whose page carries no JobPosting keeps the notes as its body;
an entry the catalogue already holds is a liveness refresh; an entry whose employer the
coverage gate will discard is yielded body-less without a page request.

#### Scenario: A covered employer's page is never read

- **WHEN** the coverage resolver reports an entry's employer as covered
- **THEN** the entry is yielded with the notes as its body and its page is not fetched

### Requirement: The list states the role shape

An entry from an internship list SHALL carry employment type `internship` and seniority
`intern`; one from a new-grad list `full_time` and `junior`; any other list neither.

### Requirement: Whole-list lifecycle

The adapter SHALL be marked an aggregator and a `fullCatalog` source: a list is read whole
every run, so an entry a clean run did not yield has been flipped inactive or aged out.

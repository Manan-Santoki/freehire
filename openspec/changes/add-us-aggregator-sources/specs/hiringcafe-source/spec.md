## Purpose

Crawl hiring.cafe's keyword search — an index of postings taken from employers' own career
sites — into the catalogue through the Chrome-fingerprint transport, newest-first, reading
each new posting's body from its own page.

## ADDED Requirements

### Requirement: hiring.cafe keyword-slice crawl

The system SHALL provide a `hiringcafe` source adapter whose board is a search keyword and
whose optional region is an alpha-2 country code selecting the search's location filter. The
adapter SHALL read the server-rendered search page's embedded `__NEXT_DATA__` payload, ordered
newest-first, walking pages until the site states the last page, until a page adds no new
hit, or until the adapter's page cap.

#### Scenario: A keyword board yields its newest postings

- **WHEN** the adapter crawls a keyword board with region `US`
- **THEN** it returns one `Job` per distinct hit, keyed on the hit's own `id`, linked to the
  employer's `apply_url`, with the employer, title, location, work mode, countries, seniority,
  employment type, skills, years of experience and structured salary read from the hit's
  processed data

#### Scenario: An unknown region fails the board

- **WHEN** a board's region is not one the adapter knows a display name for
- **THEN** the crawl fails with an error naming the table to extend, and no request is made

#### Scenario: A challenge page fails the board

- **WHEN** the first search page carries no `__NEXT_DATA__` script
- **THEN** the crawl fails rather than reporting an empty keyword

### Requirement: Bodies are hydrated per new posting

The adapter SHALL be a `HydratingSource`: a hit the catalogue already holds is re-listed as a
liveness refresh with no request, and a new hit's body is read from its own posting page. A
hit whose page cannot be read, is expired, or carries no body SHALL be dropped for that crawl
rather than stored body-less; a crawl that listed new hits and read none of them SHALL fail.

#### Scenario: A seen posting costs no request

- **WHEN** the seen predicate reports a hit's id as ingested
- **THEN** the hit is yielded as `SeenRefresh` and its posting page is not fetched

#### Scenario: An expired posting is dropped

- **WHEN** a hit or its posting page states the posting has expired
- **THEN** no `Job` is yielded for it

### Requirement: Paced, retrying transport

The adapter SHALL be served through the Chrome-fingerprint transport on a limiter of its own
shared by listing and detail requests, and SHALL retry a request refused as a burst (429, or
the 403 a challenge arrives under) once per rung of a short back-off ladder before failing.

#### Scenario: A refused burst is retried

- **WHEN** a request is answered 429 and the next attempt is served
- **THEN** the crawl continues with the served response

### Requirement: Aggregator with a wide sweep grace

The adapter SHALL be marked an aggregator (not boardless) and SHALL declare a 14-day sweep
grace, because a crawl reads only the newest pages of a keyword and a live posting drifts past
that depth.

## Purpose

Crawl Hacker News' monthly "Ask HN: Who is hiring?" threads into the catalogue through the
Algolia HN Search API, one posting per top-level comment that follows the thread's header
convention.

## ADDED Requirements

### Requirement: Thread discovery by the posting account

The system SHALL provide a boardless `hackernews` source adapter that finds hiring threads by
listing the `whoishiring` account's newest stories and keeping those whose title matches
"Who is hiring", newest first, up to two. A crawl with no such thread SHALL fail.

#### Scenario: Sibling threads are skipped

- **WHEN** the account's newest stories include "Who wants to be hired?" and "Freelancer?"
  threads beside two hiring threads
- **THEN** exactly the two hiring threads are read

### Requirement: One posting per conforming top-level comment

For each thread the adapter SHALL read the thread item whole and yield one `Job` per
top-level comment whose first paragraph carries at least two pipe-delimited segments — the
employer, then the role — with the location taken from the first later segment that is not a
commitment word, a URL or a salary, the first anchor as the link (falling back to the
comment's permalink), and the whole comment as the body. Deleted comments, comments with no
pipe-delimited header, and replies SHALL NOT be yielded.

#### Scenario: A canonical post is mapped

- **WHEN** a comment reads "Modash.io | Senior Product Engineer | Remote (Europe) | ... | <link>"
- **THEN** the job's company is Modash.io, its title Senior Product Engineer, its location
  Remote (Europe), it is flagged remote, and its URL is the link

#### Scenario: A post with no employer is dropped

- **WHEN** a comment's first paragraph has no pipe
- **THEN** no `Job` is yielded for it

### Requirement: Whole-thread lifecycle

The adapter SHALL be marked an aggregator and a `fullCatalog` source, and any thread that
cannot be read SHALL fail the crawl, so a post from a thread outside the two-thread window is
closed by the source-scoped sweep.

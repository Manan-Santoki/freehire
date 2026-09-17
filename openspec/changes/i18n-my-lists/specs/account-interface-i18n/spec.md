## Purpose

Ensures the `/my/**` account section renders in the signed-in user's
resolved interface locale — English or Russian today — rather than always
in English.

## ADDED Requirements

### Requirement: The job-lists page is translated

`/my/lists` SHALL render every literal string it owns — including its
plural job-count line, its `window.prompt()` message text, and its
`aria-label`/`title`/confirm-dialog text interpolated with a list's own
name — and its browser title, in the resolved account-section locale. A
server-supplied error message (`ApiError.message`) is exempt: it is shown
verbatim in whatever language the server produced it, taking priority over
the page's own generic fallback for that action.

#### Scenario: A Russian-language account sees the job-lists page in Russian

- **WHEN** a signed-in user whose account `language` is `ru` opens
  `/my/lists`
- **THEN** every literal string on the page, its `<title>`, and every
  prompt/dialog it opens render in Russian, with the job count using the
  correct Russian plural form for its value

#### Scenario: A generic action failure renders in the resolved locale

- **WHEN** an action on this page (create, rename, edit description, share,
  unshare, copy link, or delete) fails without the server supplying its own
  message
- **THEN** the page's own fallback message for that action renders in the
  resolved locale

#### Scenario: A server-supplied error message is never translated

- **WHEN** an action on this page fails and the server's response carries
  its own message
- **THEN** that message is shown exactly as the server sent it, regardless
  of the resolved locale

#### Scenario: An English-language account is unaffected

- **WHEN** a signed-in user whose account `language` is `en` (or any
  supported-but-not-yet-translated locale) opens `/my/lists`
- **THEN** the page renders exactly as it did before this change

## Purpose

Ensures every date and relative-time string the frontend renders follows the
same resolved locale as the surrounding page's UI text, instead of the
visitor's browser/runtime locale — so a translated page never shows an
untranslated date, and an untranslated page never shows a translated one.

## ADDED Requirements

### Requirement: Date and relative-time strings follow the page's resolved locale

Every date or relative-time string rendered by the application SHALL be
formatted using the same locale the surrounding page's UI text resolves to,
never the visitor's browser or runtime default locale.

#### Scenario: A translated account page formats its dates in the same language

- **WHEN** a signed-in user whose account `language` is `ru` opens a `/my/**`
  page that shows a relative time (e.g. "created", "last used") or an
  absolute date
- **THEN** that string renders entirely in Russian, with no English words
  spliced into it, regardless of the visitor's browser or OS locale

#### Scenario: An untranslated page formats its dates in English

- **WHEN** a visitor whose browser reports a non-English locale opens a page
  whose UI text is English (a public page, or a `/my/**` page whose account
  `language` resolves to English)
- **THEN** every date or relative-time string on that page renders in
  English, regardless of the visitor's browser or OS locale

#### Scenario: The header notification list follows the same rule as the rest of the page

- **WHEN** the header notification list renders on any route, public or
  `/my/**`
- **THEN** its dates follow that route's resolved locale, the same rule
  applied everywhere else — not the visitor's browser locale

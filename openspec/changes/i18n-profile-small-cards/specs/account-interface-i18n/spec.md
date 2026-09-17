## Purpose

Ensures the `/my/**` account section renders in the signed-in user's
resolved interface locale — English or Russian today — rather than always
in English, so a Russian-speaking user reads their own account pages in
Russian.

## ADDED Requirements

### Requirement: The profile's account-settings and contact/location/skills cards are translated

The `/my/profile` page's account-settings cards (timezone, language) and its
location-preferences, skills, and contacts cards SHALL render every literal
string — labels, placeholders, section headings, save-state messages, and
error messages — in the resolved account-section locale.

Facet-dictionary values (work-mode/region/country option labels, skill
names) and IANA timezone identifiers rendered inside these cards are exempt:
they are dictionary/wire tokens, not this component's own prose, and follow
whatever the dictionary itself is written in until a separate change
localizes dictionaries.

#### Scenario: A Russian-language account sees the timezone and language cards in Russian

- **WHEN** a signed-in user whose account `language` is `ru` opens
  `/my/profile` and views the account-settings section
- **THEN** the timezone card's heading, description, save states
  ("Saving…"/"Saved"/an error message), and the language card's own heading,
  description, save states, and the six language names in its picker all
  render in Russian

#### Scenario: A Russian-language account sees the location, skills, and contacts cards in Russian

- **WHEN** a signed-in user whose account `language` is `ru` opens
  `/my/profile` and views the location, skills, or contacts card
- **THEN** every label, placeholder, section heading, and error/empty-state
  message in that card renders in Russian, while facet option labels (work
  mode, region, country), skill names, and any raw wire token continue to
  render exactly as the dictionary or API supplies them

#### Scenario: An English-language account is unaffected

- **WHEN** a signed-in user whose account `language` is `en` (or any
  supported-but-not-yet-translated locale) opens `/my/profile`
- **THEN** these cards render exactly as they did before this change

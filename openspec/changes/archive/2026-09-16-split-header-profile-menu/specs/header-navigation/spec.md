## ADDED Requirements

### Requirement: Desktop profile menu

On a wide viewport, a signed-in user's profile icon SHALL act as a second,
independent menu trigger, opening a dropdown scoped to that user's own
account: Profile (targeting `/my/profile`), Activity, Tracking, Inbox, Agent,
Tailor, Search alerts, API keys, My submissions, then Submit a job, then
(moderators only) Moderation, then Log out. Opening the profile menu SHALL
close the consolidated menu and the notification bell dropdown if either is
open, and opening either of those SHALL close the profile menu. The profile
menu SHALL close after an item is selected, on `Escape`, on outside click, and
on navigation. For a signed-out user on a wide viewport the profile icon SHALL
remain a direct Sign in action (no dropdown).

#### Scenario: Profile menu lists the account items

- **WHEN** a signed-in non-moderator user opens the profile menu on a wide
  viewport
- **THEN** it shows Profile, Activity, Tracking, Inbox, Agent, Tailor, Search
  alerts, API keys, My submissions, Submit a job, and Log out, but not
  Moderation

#### Scenario: Moderator sees moderation in the profile menu

- **WHEN** a signed-in moderator opens the profile menu on a wide viewport
- **THEN** the profile menu additionally shows the Moderation item

#### Scenario: Log out is immediately visible

- **WHEN** a signed-in user opens the profile menu on a wide viewport
- **THEN** the Log out action is visible in the panel without needing to
  scroll

#### Scenario: Profile item targets the single profile

- **WHEN** a signed-in user selects the Profile item in the profile menu
- **THEN** the app navigates to `/my/profile`

#### Scenario: Opening the profile menu closes the consolidated menu

- **WHEN** the consolidated menu is open on a wide viewport and the user opens
  the profile menu
- **THEN** the consolidated menu closes and the profile menu opens

#### Scenario: Opening the consolidated menu closes the profile menu

- **WHEN** the profile menu is open on a wide viewport and the user opens the
  consolidated menu
- **THEN** the profile menu closes and the consolidated menu opens

#### Scenario: Signed-out profile icon has no dropdown

- **WHEN** a signed-out user is on a wide viewport
- **THEN** the profile icon position shows a direct Sign in action and
  activating it does not show a dropdown panel

## MODIFIED Requirements

### Requirement: Unified header layout

The site-wide header SHALL present three slots in a single row — the logo
(left, linking to `/`), a large search field (center, growing to fill
available width), and a controls cluster (right) — using the same structure
on every viewport. The header SHALL NOT render inline nav links outside the
controls cluster. On a wide viewport, the controls cluster additionally hosts
a profile menu trigger for a signed-in user (see Requirement: Desktop profile
menu); on a narrow viewport it does not, and the consolidated menu trigger is
the cluster's only menu control.

#### Scenario: Header renders three slots on desktop

- **WHEN** a user loads any page on a wide viewport
- **THEN** the header shows the logo, a centered search field, and the
  controls cluster (the consolidated menu trigger, plus a profile menu
  trigger when signed in), with no inline nav links outside that cluster

#### Scenario: Header renders three slots on mobile

- **WHEN** a user loads any page on a narrow viewport
- **THEN** the header shows the logo, the search field, and the controls
  cluster with a single menu trigger button, in the same arrangement as
  desktop

### Requirement: Consolidated menu

The menu SHALL contain the site nav links (Jobs, Companies, Collections, the
site's feature pages, About, and Open) and the theme toggle. On a narrow viewport
the menu SHALL additionally contain, for a signed-in user, the account items
(Profile, Activity, Tracking, Inbox, Agent, Tailor, Search alerts, API keys,
My submissions), Submit a job, and a Log out action; the Moderation item
SHALL appear only for a moderator. For a signed-out user on a narrow viewport
the menu SHALL show a Sign in action instead of the account items. On a wide
viewport the menu SHALL NOT contain the account items, Submit a job,
Moderation, or a Sign in/Log out action — those are reached through the
profile menu trigger (signed in) or the existing direct sign-in icon (signed
out); see Requirement: Desktop profile menu. The Profile item, where present
in the menu, SHALL target `/my/profile`. The menu SHALL close after an item
is selected, on `Escape`, and on outside click.

#### Scenario: Signed-out menu

- **WHEN** a signed-out user opens the menu on a narrow viewport
- **THEN** it shows the nav links, the theme toggle, and a Sign in action, and
  no account items

#### Scenario: Signed-in menu

- **WHEN** a signed-in non-moderator user opens the menu on a narrow viewport
- **THEN** it shows the nav links, the account items (Profile, Activity,
  Tracking, Inbox, Agent, Tailor, Search alerts, API keys, My submissions),
  Submit a job, the theme toggle, and Log out, but not Moderation

#### Scenario: Profile item targets the single profile

- **WHEN** a signed-in user on a narrow viewport selects the Profile account
  item
- **THEN** the app navigates to `/my/profile`

#### Scenario: Desktop menu excludes account items

- **WHEN** any user opens the menu on a wide viewport
- **THEN** it shows only the nav links and the theme toggle, with no account
  items, no Submit a job, no Moderation, and no Sign in/Log out action

#### Scenario: Moderator sees moderation

- **WHEN** a signed-in moderator opens the menu on a narrow viewport
- **THEN** the menu additionally shows the Moderation item

#### Scenario: Open link sits next to About

- **WHEN** any user opens the menu, on either viewport
- **THEN** the nav links include an Open item (targeting `/open`) immediately
  next to the About item

#### Scenario: Selecting an item closes the menu

- **WHEN** the menu is open and the user selects any link or action
- **THEN** the menu closes

## Purpose

Ensures the `/my/**` account section renders in the signed-in user's
resolved interface locale — English or Russian today — rather than always
in English.

## ADDED Requirements

### Requirement: The notifications page is translated

`/my/notifications` SHALL render its browser title, its "mark all read"
action, and its empty-state message in the resolved account-section
locale. The notification rows themselves are server-supplied content, not
this page's own prose, and are exempt.

#### Scenario: A Russian-language account sees the notifications page in Russian

- **WHEN** a signed-in user whose account `language` is `ru` opens
  `/my/notifications`
- **THEN** the page's `<title>`, its "mark all read" action (when unread
  notifications exist), and its empty-state message (when there are no
  notifications) all render in Russian

#### Scenario: An English-language account is unaffected

- **WHEN** a signed-in user whose account `language` is `en` (or any
  supported-but-not-yet-translated locale) opens `/my/notifications`
- **THEN** the page renders exactly as it did before this change

## Purpose

Ensures the `/my/**` account section renders in the signed-in user's
resolved interface locale — English or Russian today — rather than always
in English.

## ADDED Requirements

### Requirement: The webhook settings page is translated

`/my/webhook` SHALL render every literal string it owns — the sign-in
prompt, heading, description, form controls, status line, action buttons,
and the delete-confirmation dialog — and its browser title, in the resolved
account-section locale.

#### Scenario: A Russian-language account sees the webhook page in Russian

- **WHEN** a signed-in user whose account `language` is `ru` opens
  `/my/webhook`
- **THEN** every literal string on the page, and the page's `<title>`,
  render in Russian

#### Scenario: An English-language account is unaffected

- **WHEN** a signed-in user whose account `language` is `en` (or any
  supported-but-not-yet-translated locale) opens `/my/webhook`
- **THEN** the page renders exactly as it did before this change

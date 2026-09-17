## ADDED Requirements

### Requirement: An employment's achievements are collapsed behind an expandable summary

The experience view SHALL show each employment's achievement list collapsed by default,
summarized as a count ("N achievements", or a message that none are recorded yet). Expanding
an employment SHALL reveal its achievements in place, without navigating away from the
experience view or changing the page's URL. An employment SHALL start expanded when it is the
only reason the view is showing something the candidate needs to act on (see the unconfirmed
banner requirement below).

#### Scenario: A collapsed employment shows a count

- **WHEN** a signed-in user opens the experience view and an employment holds achievements
- **THEN** that employment renders collapsed, showing how many achievements it holds, and no
  achievement bullet text

#### Scenario: Expanding reveals the achievements in place

- **WHEN** the candidate activates a collapsed employment's summary
- **THEN** its achievements render beneath it in the same view, and the browser URL does not
  change

#### Scenario: An employment with no achievements says so

- **WHEN** an employment holds no achievements
- **THEN** its summary says none are recorded yet, rather than showing a count of zero

### Requirement: Achievement selection for Merge and Tailor works across collapsed employments

Selecting an achievement for merging or for "Tailor with assistant" SHALL NOT require its
employment to be expanded, and SHALL NOT be cleared by collapsing or expanding any employment.
A selection SHALL be able to span achievements under different employments, consistent with
"Tailor with assistant" accepting achievements from more than one place.

#### Scenario: Collapsing an employment keeps its selected achievements selected

- **WHEN** the candidate has selected an achievement under an expanded employment and then
  collapses that employment
- **THEN** the achievement remains selected and is counted in the selection toolbar

#### Scenario: A cross-employment selection remains available for Tailor with assistant

- **WHEN** the candidate selects one achievement from one employment and another from a
  different employment, with both employments left collapsed afterward
- **THEN** "Tailor with assistant" is available for the two-item selection

### Requirement: The unconfirmed-achievements banner links to what it is about

When the bank holds one or more achievements the assistant recorded but the candidate has not
confirmed, the banner reporting that count SHALL be an activatable control. Activating it
SHALL expand the employment (or the unplaced section) holding the first unconfirmed
achievement, in bank order, and SHALL bring that achievement into view.

#### Scenario: Activating the banner reaches the first unconfirmed achievement

- **WHEN** the bank holds unconfirmed achievements and the candidate activates the banner
- **THEN** the employment holding the first unconfirmed achievement (in bank order) expands
  if it was collapsed, and that achievement is scrolled into view

#### Scenario: An unconfirmed achievement with no employment is reachable too

- **WHEN** the first unconfirmed achievement in bank order has no employment attached
- **THEN** activating the banner reveals it in the "Not tied to a role" section

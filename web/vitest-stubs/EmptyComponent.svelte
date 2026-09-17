<!-- Test-only stub: renders nothing. Stands in (via `vi.mock`) for a heavy child component
     whose own onMount side effects (a real network call, e.g. AssistantChat's boot()) a
     test has no reason to exercise and does not want to mock the transitive dependencies
     of. -->

import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { canonicalQuery, filtersFromParams, type FilterStore } from '$lib/filters';
import type { SavedSearch } from '$lib/types';
import SavedFilters from './SavedFilters.svelte';

// The card reads two module-level singletons — the session and the saved-search store —
// so both are mocked; `store` is a live-store stand-in that records `apply`, since the
// card's whole contract is "one click → one canonical write".
const { auth, saved } = vi.hoisted(() => ({
  auth: { signedIn: true },
  saved: { items: [] as SavedSearch[], ensureLoaded: vi.fn() },
}));

vi.mock('$lib/auth.svelte', () => ({ isAuthenticated: () => auth.signedIn }));
vi.mock('$lib/savedSearches.svelte', () => ({ savedSearches: saved }));

const search = (id: number, name: string, query: string): SavedSearch => ({
  id,
  name,
  query,
  derived_from_profile: false,
  created_at: null,
  updated_at: null,
});

/** A stand-in for the live FilterStore: the card only reads `value` and calls `apply`. */
function storeWith(query: string) {
  const apply = vi.fn();
  const store = {
    value: filtersFromParams(new URLSearchParams(query)),
    apply,
  } as unknown as FilterStore;
  return { store, apply };
}

const devopsQuery = 'work_mode=remote&countries=us&category=devops&seniority=intern&sort=match';

beforeEach(() => {
  auth.signedIn = true;
  saved.ensureLoaded.mockReset();
  saved.items = [
    search(1, 'DevOps internships', devopsQuery),
    search(2, 'Broad software roles', 'work_mode=remote&countries=us&category=software_engineering'),
  ];
});

describe('SavedFilters', () => {
  it('applies a saved set on click, dropping the stored sort (ordering is not part of the set)', async () => {
    const { store, apply } = storeWith('');
    render(SavedFilters, { props: { store } });

    await fireEvent.click(screen.getByRole('button', { name: 'DevOps internships' }));

    expect(apply).toHaveBeenCalledTimes(1);
    // The written query: clicked set's facets, `us` for the country, and no `sort` —
    // the stored sort is dropped because the ordering is not part of the set.
    const written = new URLSearchParams(apply.mock.calls[0]?.[0]);
    expect(written.get('countries')).toBe('us');
    expect(written.get('category')).toBe('devops');
    expect(written.get('seniority')).toBe('intern');
    expect(written.get('sort')).toBeNull();
  });

  it('marks the set matching the applied filters as current and leaves the rest plain', () => {
    const { store } = storeWith('work_mode=remote&countries=us&category=software_engineering');
    render(SavedFilters, { props: { store } });

    expect(screen.getByRole('button', { name: 'Broad software roles' }).getAttribute('aria-current')).toBe('true');
    expect(screen.getByRole('button', { name: 'DevOps internships' }).getAttribute('aria-current')).toBeNull();
  });

  it('matches the current set even when the stored query still carries a sort', () => {
    // The stored set carries `sort=match`; the applied filters carry none. They are the
    // same set (see canonicalQuery), so the row must light up rather than read as unapplied.
    const { store } = storeWith(canonicalQuery(devopsQuery));
    render(SavedFilters, { props: { store } });

    expect(screen.getByRole('button', { name: 'DevOps internships' }).getAttribute('aria-current')).toBe('true');
  });

  it('does not re-apply the set that already matches the filters', async () => {
    const { store, apply } = storeWith('work_mode=remote&countries=us&category=software_engineering');
    render(SavedFilters, { props: { store } });

    await fireEvent.click(screen.getByRole('button', { name: 'Broad software roles' }));

    expect(apply).not.toHaveBeenCalled();
  });

  it('renders nothing for a signed-out visitor', () => {
    auth.signedIn = false;
    const { store } = storeWith('');
    render(SavedFilters, { props: { store } });

    expect(screen.queryByText('Saved filters')).toBeNull();
  });

  it('renders nothing for an account with no saved sets', () => {
    saved.items = [];
    const { store } = storeWith('');
    render(SavedFilters, { props: { store } });

    expect(screen.queryByText('Saved filters')).toBeNull();
  });

  it('loads the saved sets once the session is confirmed', async () => {
    const { store } = storeWith('');
    render(SavedFilters, { props: { store } });

    await vi.waitFor(() => expect(saved.ensureLoaded).toHaveBeenCalled());
  });
});

import { describe, it, expect } from 'vitest';
import {
  parseForYouVerdict,
  forYouVerdictHref,
  parseForYouMin,
  forYouJobCard,
  FOR_YOU_VERDICTS,
} from './forYouFilters';

describe('parseForYouVerdict', () => {
  it('reads a recognised verdict from the query string', () => {
    for (const v of FOR_YOU_VERDICTS) {
      expect(parseForYouVerdict(new URLSearchParams({ verdict: v }))).toBe(v);
    }
  });

  it('is "" (all) when the param is absent', () => {
    expect(parseForYouVerdict(new URLSearchParams())).toBe('');
  });

  it('is "" (all) for an unrecognised value, never a thrown error', () => {
    expect(parseForYouVerdict(new URLSearchParams({ verdict: 'MAYBE_NOT' }))).toBe('');
    expect(parseForYouVerdict(new URLSearchParams({ verdict: '' }))).toBe('');
  });
});

describe('forYouVerdictHref', () => {
  it('sets ?verdict= for a chip that is not yet active', () => {
    expect(forYouVerdictHref('/jobs/for-you', new URLSearchParams(), 'APPLY')).toBe(
      '/jobs/for-you?verdict=APPLY',
    );
  });

  it('toggles the active chip back to "all" by clearing the param', () => {
    expect(
      forYouVerdictHref('/jobs/for-you', new URLSearchParams({ verdict: 'APPLY' }), 'APPLY'),
    ).toBe('/jobs/for-you');
  });

  it('switching to a different verdict replaces the previous one, not adds to it', () => {
    expect(
      forYouVerdictHref('/jobs/for-you', new URLSearchParams({ verdict: 'APPLY' }), 'SKIP'),
    ).toBe('/jobs/for-you?verdict=SKIP');
  });

  it('keeps an unrelated param (min) and drops a stale page', () => {
    const href = forYouVerdictHref(
      '/jobs/for-you',
      new URLSearchParams({ min: '70', page: '3' }),
      'MAYBE',
    );
    expect(href).toBe('/jobs/for-you?min=70&verdict=MAYBE');
  });
});

describe('parseForYouMin', () => {
  it('reads a valid 1-100 floor', () => {
    expect(parseForYouMin(new URLSearchParams({ min: '70' }))).toBe(70);
    expect(parseForYouMin(new URLSearchParams({ min: '100' }))).toBe(100);
    expect(parseForYouMin(new URLSearchParams({ min: '1' }))).toBe(1);
  });

  it('is undefined when absent', () => {
    expect(parseForYouMin(new URLSearchParams())).toBeUndefined();
  });

  it('is undefined for non-numeric, zero, negative, or out-of-range values', () => {
    expect(parseForYouMin(new URLSearchParams({ min: 'abc' }))).toBeUndefined();
    expect(parseForYouMin(new URLSearchParams({ min: '0' }))).toBeUndefined();
    expect(parseForYouMin(new URLSearchParams({ min: '-5' }))).toBeUndefined();
    expect(parseForYouMin(new URLSearchParams({ min: '101' }))).toBeUndefined();
  });
});

describe('forYouJobCard', () => {
  const base = {
    slug: 'acme-engineer',
    title: 'Software Engineer',
    company: 'Acme',
    work_mode: 'remote',
    skills: ['go', 'sql'],
  };

  it('maps `slug` to the `public_slug` JobRow keys and links on', () => {
    expect(forYouJobCard(base).public_slug).toBe('acme-engineer');
  });

  it('carries the fields JobCard actually draws', () => {
    const card = forYouJobCard({ ...base, posted_at: '2026-09-01T00:00:00Z', closed_at: undefined });
    expect(card).toMatchObject({
      title: 'Software Engineer',
      company: 'Acme',
      work_mode: 'remote',
      skills: ['go', 'sql'],
      posted_at: '2026-09-01T00:00:00Z',
    });
  });

  it('turns an empty work_mode into undefined rather than an empty facet chip', () => {
    expect(forYouJobCard({ ...base, work_mode: '' }).work_mode).toBeUndefined();
  });
});

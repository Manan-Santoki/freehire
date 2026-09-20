// The pure view-model behind the "For You" feed's verdict filter chips (see
// routes/jobs/for-you). No SvelteKit or Svelte runes here, so this is unit-testable
// in plain Node, the same discipline `pagination.ts` follows for the /jobs feed's own
// URL-driven controls.
//
// The backend (`handler.ForYou`) takes a single `verdict` query param — "", APPLY,
// MAYBE or SKIP — never a set, so the three chips are mutually exclusive toggles, not
// independent checkboxes: choosing one clears any other, and choosing the active one
// again clears back to "all".

import type { Card as JobCard } from './generated/contracts';

/** The three Jev verdicts the feed can be narrowed to. Mirrors the wire values
 *  `jevscore` emits (see contracts.ts's `ForYouJob.verdict`) — not a local vocabulary
 *  that could drift from what the server actually serves. */
export type ForYouVerdict = 'APPLY' | 'MAYBE' | 'SKIP';

/** The chip row's fixed order — best fit first, so the toggles read as ranked. */
export const FOR_YOU_VERDICTS: ForYouVerdict[] = ['APPLY', 'MAYBE', 'SKIP'];

/** Parse the `?verdict=` query param into what `api.forYouFeed` expects: one of the
 *  three verdicts, or `''` for "show all" (the default — SKIP is never hidden unless
 *  chosen). A missing or unrecognised value reads as "all" rather than as a request
 *  that fails, the same leniency `parsePage` shows a malformed `?page=`. */
export function parseForYouVerdict(params: URLSearchParams): ForYouVerdict | '' {
  const raw = params.get('verdict');
  return raw != null && (FOR_YOU_VERDICTS as string[]).includes(raw) ? (raw as ForYouVerdict) : '';
}

/** The `?verdict=` href one chip should link to: selecting the verdict already active
 *  clears the filter back to "all" (a toggle-off), matching how facet chips behave
 *  elsewhere. `params` carries whatever else is on the URL (e.g. `min`) unchanged —
 *  the two filters are independent — and a stale `page` is dropped, since a narrower
 *  or wider feed is a different result set to page from the top of. */
export function forYouVerdictHref(
  pathname: string,
  params: URLSearchParams,
  verdict: ForYouVerdict,
): string {
  const next = new URLSearchParams(params);
  const alreadyActive = parseForYouVerdict(params) === verdict;
  if (alreadyActive) next.delete('verdict');
  else next.set('verdict', verdict);
  next.delete('page');
  const qs = next.toString();
  return qs ? `${pathname}?${qs}` : pathname;
}

/** Parse the optional `?min=` match-percent floor (0-100). Absent, non-numeric, or
 *  out-of-range reads as "no floor" rather than failing the page — the same
 *  malformed-input-never-breaks-a-page rule `pagination.ts`'s `parsePage` follows. */
export function parseForYouMin(params: URLSearchParams): number | undefined {
  const raw = params.get('min');
  if (!raw || !/^\d+$/.test(raw)) return undefined;
  const n = Number(raw);
  return n > 0 && n <= 100 ? n : undefined;
}

/** A `ForYouJob` row shaped into a `JobCard` so `JobRow` can render it: the two wire
 *  shapes disagree on the slug field (`slug` vs `public_slug`) and `ForYouJob` carries
 *  none of `JobCard`'s other optional facets (countries, collections, ghost, ...), so a
 *  straight cast would either miss the identity JobRow keys and links on, or claim facets
 *  the personalized feed never computed. Everything `JobCard` leaves optional is simply
 *  omitted here rather than guessed. The Jev verdict/match_pct are NOT folded in — the
 *  route passes those to `JobRow` as its own `verdict`/`matchPct` props instead, so this
 *  function only has to answer "what does the card look like", not "what does it score". */
export function forYouJobCard(job: {
  slug: string;
  title: string;
  company: string;
  work_mode: string;
  posted_at?: string;
  closed_at?: string;
  skills: string[];
}): JobCard {
  return {
    public_slug: job.slug,
    title: job.title,
    company: job.company,
    work_mode: job.work_mode || undefined,
    posted_at: job.posted_at,
    closed_at: job.closed_at,
    skills: job.skills,
  };
}

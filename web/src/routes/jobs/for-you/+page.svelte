<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import { verdictTone } from '$lib/jobMatch';
  import { FOR_YOU_VERDICTS, forYouVerdictHref, forYouJobCard } from '$lib/forYouFilters';
  import type { ForYouJob } from '$lib/types';
  import JobRow from '$lib/components/JobRow.svelte';
  import States from '$lib/components/States.svelte';
  import { LoadMore } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // The active verdict/min filters, read straight off `data` (the server `load` already
  // parsed and validated the URL) rather than re-parsed from `page.url` here — one place
  // decides what a malformed query param means.
  const verdict = $derived(data.verdict);
  const min = $derived(data.min);

  const makePaginator = () =>
    new Paginator<ForYouJob>(
      (limit, offset) => api.forYouFeed({ verdict: verdict || undefined, min }, limit, offset),
      { keyOf: (job) => job.slug },
    );

  // Seeded once from the server-rendered first page. `filterKey` is the (verdict, min)
  // pair that page was fetched for; a chip click is a real navigation (a new `?verdict=`
  // href), so SvelteKit re-runs `+page.server.ts` and hands this component a fresh
  // `data.initial` without remounting it — the effect below re-seeds a fresh paginator
  // whenever that pair changes, the same "page link" pattern JobsView.svelte uses to
  // notice a `?page=N` navigation and re-seed from the `initial` its own `load` just
  // fetched, rather than issuing a second, redundant client-side request.
  let filterKey = $state(untrack(() => `${verdict}|${min}`));
  let jobs = $state.raw(
    untrack(() => {
      const p = makePaginator();
      p.seed(data.initial, 0);
      return p;
    }),
  );

  $effect(() => {
    const key = `${verdict}|${min}`;
    const slice = data.initial;
    untrack(() => {
      if (key === filterKey) return;
      filterKey = key;
      const next = makePaginator();
      next.seed(slice, 0);
      jobs = next;
    });
  });
</script>

<svelte:head>
  <title>For You — freehire</title>
  <!-- Personal, ranked against this viewer's own profile: nothing here is the same page
       for two people, so it must never be indexed or offered as a canonical destination. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <h1 class="text-xl font-semibold tracking-tight">For You</h1>
  <p class="mt-1 text-sm text-muted-foreground">
    Open jobs ranked by how well they fit your profile, best match first.
  </p>

  <!-- Verdict filter chips: mutually exclusive toggles over the ONE `verdict` param the
       backend takes (see handler.ForYou) — never a multi-select, since the API has no
       "any of these three" query. Real <a href> links (not client-only state) so the
       filtered feed is itself a shareable, back-button-safe URL, matching how every other
       facet control in the app behaves. SKIP is never hidden by default: "all" is every
       verdict, this row only NARROWS it. -->
  <!-- eslint-disable svelte/no-navigation-without-resolve -- both hrefs are built from
       `page.url.pathname`, which SvelteKit has already resolved (this route takes no
       params); `forYouVerdictHref` only appends/removes the `verdict`/`page` query
       params, the same pattern Pagination.svelte's `pageHref` follows. -->
  <div class="mt-4 flex flex-wrap items-center gap-2" role="group" aria-label="Filter by Jev verdict">
    <a
      href={page.url.pathname}
      aria-current={!verdict ? 'true' : undefined}
      class={[
        'rounded-full border px-3 py-1 text-xs font-medium transition',
        !verdict
          ? 'border-brand/30 bg-brand-muted text-brand-strong'
          : 'border-border bg-secondary text-muted-foreground hover:bg-accent',
      ]}
    >
      All
    </a>
    {#each FOR_YOU_VERDICTS as v (v)}
      {@const active = verdict === v}
      <a
        href={forYouVerdictHref(page.url.pathname, page.url.searchParams, v)}
        aria-current={active ? 'true' : undefined}
        class={[
          'rounded-full border px-3 py-1 text-xs font-medium transition',
          active ? verdictTone(v) : 'border-border bg-secondary text-muted-foreground hover:bg-accent',
        ]}
      >
        {v}
      </a>
    {/each}
  </div>
  <!-- eslint-enable svelte/no-navigation-without-resolve -->

  <div class="mt-4">
    {#if jobs.status === 'loading'}
      <States state="loading" />
    {:else if jobs.status === 'error'}
      <States state="error" message="Failed to load your feed." />
    {:else if jobs.items.length === 0}
      <States
        state="empty"
        message={verdict
          ? `No ${verdict.toLowerCase()} matches yet — try "All" or check back once more jobs are scored.`
          : 'Nothing scored for you yet — add skills or a résumé to your profile and check back soon.'}
      />
    {:else}
      <div class="flex flex-col gap-3">
        {#each jobs.items as job (job.slug)}
          <JobRow job={forYouJobCard(job)} verdict={job.verdict} matchPct={job.match_pct} />
        {/each}
      </div>
      {#if jobs.hasMore}
        <LoadMore loading={jobs.loadingMore} error={jobs.loadMoreError} onclick={() => jobs.loadMore()} />
      {/if}
    {/if}
  </div>
</div>

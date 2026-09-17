<script lang="ts">
  import { Bookmark } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { canonicalQuery, savedSearchQuery, type FilterStore } from '$lib/filters';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import type { SavedSearch } from '$lib/types';

  // The jobs sidebar's saved-filter card: the account's saved sets, one click to apply.
  // The same list the filter modal's "My filters" tab carries, without the modal's
  // staging step — this sidebar edits the live store directly, so a click filters the
  // list immediately instead of the All filters → My filters → Show results detour.
  //
  // Read-and-apply only; rename and delete stay in the modal, which owns that editing
  // flow. Renders nothing when signed out or with no saved sets yet, so the sidebar
  // never grows a second empty state beside the shell's own "No filters yet".
  let { store }: { store: FilterStore } = $props();

  const items = $derived(savedSearches.items);

  // Which set is exactly the applied filters — canonically, so a set whose stored query
  // still carries a `sort=` lights up once applied (the ordering is not part of what a
  // saved search IS; see canonicalQuery).
  const current = $derived(savedSearchQuery(store.value));
  const activeId = $derived(items.find((s) => canonicalQuery(s.query) === current)?.id ?? null);

  // Load once the session is confirmed, the same trigger the modal's list uses; the
  // sign-out cache reset is owned centrally by +layout.svelte.
  $effect(() => {
    if (isAuthenticated()) void savedSearches.ensureLoaded();
  });

  function apply(set: SavedSearch) {
    // Already exactly the applied filters: a second write would only re-trigger the
    // list reload for an identical result set.
    if (set.id === activeId) return;
    store.apply(canonicalQuery(set.query));
  }
</script>

{#if isAuthenticated() && items.length > 0}
  <div class="rounded-xl border border-border bg-card p-4">
    <span class="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
      <Bookmark class="size-3.5" aria-hidden="true" /> Saved filters
    </span>
    <ul class="mt-2 flex flex-col gap-0.5">
      {#each items as set (set.id)}
        {@const active = set.id === activeId}
        <li>
          <button
            type="button"
            onclick={() => apply(set)}
            title="Apply this filter"
            aria-current={active ? 'true' : undefined}
            class={[
              'flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-accent',
              active && 'bg-accent',
            ]}
          >
            <span class={['size-1.5 shrink-0 rounded-full transition-colors', active ? 'bg-brand' : 'bg-transparent']}></span>
            <span class={['min-w-0 flex-1 truncate text-sm', active && 'font-medium']}>{set.name}</span>
          </button>
        </li>
      {/each}
    </ul>
  </div>
{/if}

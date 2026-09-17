// See app-state.ts — inert unless a test forgets to `vi.mock('$app/environment', ...)`.
// `browser` is what `$lib/urlSynced.svelte` reads (and its importers, e.g. `$lib/filters`),
// so the stub exists to let those modules LOAD in the components project at all; false is
// the honest value there, since jsdom is not the SvelteKit browser runtime.
export const browser = false;

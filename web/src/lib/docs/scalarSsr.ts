// Shared by both /docs/api and /docs/api/internal's +page.server.ts: plain JSON in
// (a generated OpenAPI document), Scalar SSR HTML fragment out. The client hydrates
// the same fragment with the same config (see scalarConfig.ts), so the two never
// disagree about what the spec says.
import { scalarConfigFromContent } from './scalarConfig';

// @scalar/server-side-rendering is imported dynamically, inside the returned `load`,
// not at module top level: one of its transitive dependencies (@scalar/snippetz's
// stringify-object -> is-identifier -> super-regex -> make-asynchronous) spins up a
// worker_threads.Worker as an import-time side effect. SvelteKit's build-time route
// analysis (`analyse.js`) imports every route module inside its OWN worker thread to
// inspect exports like `prerender` — a nested Worker created during that pass crashes
// with "Cannot destructure property 'mod' of 'threads.workerData'" because the outer
// worker's workerData isn't there for it to read. A dynamic import here means
// analyse.js's static import of a route's +page.server.ts never touches that
// dependency chain at all; `load()` itself still only runs at actual request time (or
// once, at prerender, on real routes), where there is no such nested-worker context.
// Confirmed by reproducing the crash with `pnpm run build` and clearing it with this
// change.

/** Builds a `load` for a Scalar reference page: renders `spec` to an HTML fragment
 *  once per process and memoizes it as a Promise (not just its resolved value) so
 *  concurrent first requests share one in-flight render instead of each re-running
 *  Scalar's Vue SSR renderer. `spec` depends on nothing request-specific — it's a
 *  static import at the call site — so the same fragment serves every visitor until
 *  the next deploy restarts this process anyway. */
export function createScalarPageLoad(spec: Record<string, unknown>) {
  let cachedScalarHtml: Promise<string> | undefined;
  return async () => {
    cachedScalarHtml ??= (async () => {
      const { renderApiReferenceToString } = await import('@scalar/server-side-rendering');
      return renderApiReferenceToString(scalarConfigFromContent(spec));
    })();
    return { scalarHtml: await cachedScalarHtml };
  };
}

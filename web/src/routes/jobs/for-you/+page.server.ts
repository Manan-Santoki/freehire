import { redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import { serverApi } from '$lib/server/api';
import { parseForYouVerdict, parseForYouMin } from '$lib/forYouFilters';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// /jobs/for-you is personal — it's the caller's own Jev-ranked feed, computed against
// their own profile/résumé — so, like /my/tracking, a signed-out visitor has nothing to
// show and is bounced to sign-in rather than shown an empty state. `user` is resolved
// once in the root layout load; reuse it via parent() instead of a second /me round trip.
export const load: PageServerLoad = async ({ parent, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }

  const verdict = parseForYouVerdict(url.searchParams);
  const min = parseForYouMin(url.searchParams);
  // Server-render the first page so the ranked rows are in the initial HTML — the same
  // reason /jobs seeds `initial` from `searchJobs` rather than leaving the whole feed to
  // a client-side fetch on mount. Cookie forwarded explicitly: `API_INTERNAL_URL` (set in
  // production) makes this an absolute cross-origin request, which `event.fetch` alone
  // does not carry the session cookie on.
  const initial = await serverApi(fetch, request.headers.get('cookie')).forYouFeed(
    { verdict: verdict || undefined, min },
    LIMIT,
    0,
  );

  return { initial, verdict, min };
};

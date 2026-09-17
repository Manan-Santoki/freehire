<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import JobRelated from '$lib/components/JobRelated.svelte';
  import JobSeeAlso from '$lib/components/JobSeeAlso.svelte';
  import JobView from '$lib/components/JobView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { categoryLandingLink } from '$lib/roleLandings';
  import {
    breadcrumbJsonLd,
    jobPageTitle,
    jobPostingJsonLd,
    jsonLdScript,
    metaDescription,
  } from '$lib/seo';
  import { Breadcrumbs } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const marketLink = $derived(categoryLandingLink(data.job.enrichment.category));
  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/jobs/${data.job.public_slug}`);
  // The per-job OG preview lives beside the canonical URL; og:image must be absolute.
  const ogImage = $derived(`${canonical}/og.png`);
  // A blank job body strips to "", which would otherwise suppress the
  // <meta name="description"> tag entirely (Seo.svelte omits it when empty).
  const description = $derived(
    metaDescription(data.job.description) ||
      (data.job.company
        ? `${data.job.title} at ${data.job.company} — apply on freehire.`
        : `${data.job.title} — apply on freehire.`)
  );
  // The one breadcrumb trail feeding both the visible nav and the structured data below —
  // a single array so the two can never disagree about what the trail is. There used to
  // be no `Jobs` level here: `/jobs` was a 301 to `/`, and a trail step naming a redirect
  // is a step Google resolves away. That is now backwards — `/jobs` is the real feed and
  // `/` is the one that redirects (jobs/+page.server.ts's own comment: "The feed used to
  // live at `/`, which is now the landing page") — so the level belongs back.
  const breadcrumbItems = $derived([{ name: 'Jobs', href: '/jobs' }, { name: data.job.title }]);
  const jsonLd = $derived(
    jsonLdScript([
      jobPostingJsonLd(data.job, origin),
      breadcrumbJsonLd(
        breadcrumbItems.map((item) => ({
          name: item.name,
          url: item.href ? `${origin}${item.href}` : canonical,
        })),
      ),
    ])
  );
</script>

<Seo title={jobPageTitle(data.job)} {description} {canonical} image={ogImage} />

<svelte:head>
  <!-- JobPosting structured data — eligible for Google Jobs. -->
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<!-- Slightly wider mobile gutter than the site-wide px-4: the description is dense
     raw text with no card wrapper, so 16px reads tight against the edge; sm+ falls
     back to the shared px-4 rhythm. -->
<div class="mx-auto w-full max-w-6xl px-5 py-6 sm:px-4">
  <Breadcrumbs items={breadcrumbItems} class="mb-4" />

  <JobView job={data.job} applyForm={data.applyForm} />

  <JobRelated
    similar={data.similar}
    copies={data.copies}
    copiesTotal={data.copiesTotal}
    slug={data.job.public_slug}
  />

  <JobSeeAlso cards={data.seeAlso} />

  <!-- The bridge from a posting to the market pages. Costs nothing to render: the
       category is already on the job, and the category table needs no gate check
       (see categoryLandingLink). Without this the /roles tree is reachable only from
       the footer and the sitemap, which is how a page ends up crawled but not
       weighted. -->
  {#if marketLink}
    <p class="mt-8 text-sm">
      <a
        href={resolve('/roles/[category]', { category: marketLink.slug })}
        class="text-brand-strong hover:underline"
      >
        {marketLink.label} jobs by country — openings, pay and top skills →
      </a>
    </p>
  {/if}
</div>

import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: no Svelte
// plugin, no DOM in vitest.config.ts).
//
// The account shell (/my/*) needed a footer, but not the marketing one: the four
// link columns, the popular-collections strip and the Product Hunt badge are noise
// next to an app surface with its own sidebar nav. `compact` keeps everything below
// the bottom bar (copyright, cookie settings, social links, open-source note) and
// drops the rest, reusing the bottom bar's own markup rather than forking a second
// component.
const SOURCE = readFileSync(join(import.meta.dirname, 'Footer.svelte'), 'utf8');

describe('Footer compact mode', () => {
  it('declares a compact prop defaulting to false', () => {
    expect(SOURCE).toMatch(/let\s*\{\s*compact\s*=\s*false\s*\}/);
  });

  it('skips the link groups, popular collections, and Product Hunt badge when compact', () => {
    const groupsAt = SOURCE.indexOf('{#each groups as group');
    const guardAt = SOURCE.lastIndexOf('{#if !compact}', groupsAt);
    expect(guardAt, 'expected an {#if !compact} guard before the link groups').toBeGreaterThan(-1);

    const producthuntAt = SOURCE.indexOf('productHunt.href');
    const guardEndAt = SOURCE.indexOf('{/if}', producthuntAt);
    expect(guardEndAt, 'expected the {#if !compact} guard to still be open at the Product Hunt badge').toBeGreaterThan(producthuntAt);
  });

  it('still renders the bottom bar (copyright, cookie settings, social, open-source note) unconditionally', () => {
    const guardEndAt = SOURCE.indexOf('{/if}', SOURCE.indexOf('productHunt.href'));
    const bottomBar = SOURCE.slice(guardEndAt);
    expect(bottomBar).toContain('©');
    expect(bottomBar).toContain('Cookie settings');
    expect(bottomBar).toContain('SOCIAL_LINKS');
    expect(bottomBar).toContain('View source on GitHub');
  });

  it("only borders the bottom bar's own top when compact (no divider from a section that isn't rendered)", () => {
    const commentAt = SOURCE.indexOf('Bottom bar:');
    const innerDivAt = SOURCE.indexOf('mx-auto flex max-w-6xl flex-col');
    const wrapperOpenTag = SOURCE.slice(commentAt, innerDivAt);
    expect(wrapperOpenTag).toContain('compact ?');
  });
});

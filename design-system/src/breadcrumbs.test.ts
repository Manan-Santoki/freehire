import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Breadcrumbs from './breadcrumbs.svelte';

describe('Breadcrumbs', () => {
  it('renders every hub item as a link and the current item as unlinked text', () => {
    const { getByText } = render(Breadcrumbs, {
      items: [
        { name: 'Talent Network', href: '/talent' },
        { name: 'DevOps Engineer' },
      ],
    });

    const hub = getByText('Talent Network');
    expect(hub.tagName).toBe('A');
    expect(hub.getAttribute('href')).toBe('/talent');

    const current = getByText('DevOps Engineer');
    expect(current.tagName).toBe('SPAN');
    expect(current.getAttribute('aria-current')).toBe('page');
  });

  it('separates items with a chevron but carries none before the first', () => {
    const { container } = render(Breadcrumbs, {
      items: [{ name: 'Jobs', href: '/jobs' }, { name: 'Senior Backend Engineer' }],
    });

    expect(container.querySelectorAll('svg')).toHaveLength(1);
  });

  it('names the nav for assistive tech', () => {
    const { getByRole } = render(Breadcrumbs, { items: [{ name: 'Companies', href: '/companies' }, { name: 'Acme' }] });

    expect(getByRole('navigation', { name: 'Breadcrumb' })).toBeTruthy();
  });
});

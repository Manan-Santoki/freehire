import type { Meta, StoryObj } from '@storybook/svelte';
import Breadcrumbs from './breadcrumbs.svelte';

const meta = {
  title: 'Primitives/Breadcrumbs',
  component: Breadcrumbs,
  tags: ['autodocs'],
} satisfies Meta<typeof Breadcrumbs>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TwoLevels: Story = {
  args: {
    items: [{ name: 'Talent Network', href: '/talent' }, { name: 'DevOps Engineer' }],
  },
};

export const ThreeLevels: Story = {
  args: {
    items: [
      { name: 'Companies', href: '/companies' },
      { name: 'Acme', href: '/companies/acme' },
      { name: 'Senior Backend Engineer' },
    ],
  },
};

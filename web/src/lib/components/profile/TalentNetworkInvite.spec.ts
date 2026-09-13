import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { TalentNetworkSetting } from '$lib/types';
import TalentNetworkInvite from './TalentNetworkInvite.svelte';

const { getTalentNetwork } = vi.hoisted(() => ({
  getTalentNetwork: vi.fn<() => Promise<TalentNetworkSetting>>(),
}));

vi.mock('$lib/api', () => ({ api: { getTalentNetwork } }));

let user: { beta_tester: boolean } | null;
vi.mock('$lib/auth.svelte', () => ({ currentUser: () => user }));

beforeEach(() => {
  user = { beta_tester: false };
  getTalentNetwork.mockReset().mockResolvedValue({
    talent_network_visibility: 'off',
    talent_handle: '',
    listed: false,
  });
});

describe('TalentNetworkInvite', () => {
  it('shows the invitation to a non-beta-tester account', async () => {
    render(TalentNetworkInvite);

    await expect(screen.findByText('Get found without applying')).resolves.toBeTruthy();
  });

  it('shows the invitation to a beta-tester account too', async () => {
    user = { beta_tester: true };
    render(TalentNetworkInvite);

    await expect(screen.findByText('Get found without applying')).resolves.toBeTruthy();
  });
});

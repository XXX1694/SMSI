import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '@/lib/api';

const apiMock = vi.hoisted(() => ({ posts: { create: vi.fn(), publish: vi.fn() } }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/lib/composer', async () => ({ ...(await vi.importActual<typeof import('@/lib/composer')>('@/lib/composer')), validateComposer: () => [] }));
const push = vi.hoisted(() => vi.fn());
vi.mock('next/navigation', () => ({ useRouter: () => ({ push }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock('@/components/toast', () => ({ useToast: () => toast }));

import { CreateActions } from '@/components/composer/create-actions';
import type { ComposerState } from '@/lib/composer';

const state = { content: 'Hello', accountIds: ['a1'], overrides: {}, media: [], scheduledAtUtc: null } as unknown as ComposerState;

function renderActions(onLeave = vi.fn()) {
  render(<CreateActions state={state} title="" accounts={[]} providers={[]} selected={[]} onLeave={onLeave} />);
  return onLeave;
}

async function publishNow() {
  await userEvent.click(screen.getByRole('button', { name: 'Publish now' }));
  const dialog = await screen.findByRole('dialog');
  await userEvent.click(within(dialog).getByRole('button', { name: 'Publish now' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  apiMock.posts.create.mockResolvedValue({ id: 'p1' });
});

describe('CreateActions publish now', () => {
  it('creates once and publishes', async () => {
    apiMock.posts.publish.mockResolvedValue({ id: 'p1' });
    const onLeave = renderActions();
    await publishNow();
    await waitFor(() => expect(push).toHaveBeenCalledWith(expect.stringContaining('p1')));
    expect(apiMock.posts.create).toHaveBeenCalledTimes(1);
    expect(onLeave).toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith('Publishing started');
  });

  // #127: the post was created, publishing failed; trying again must not create a second post.
  it('goes to the saved draft when publishing fails after the post was created', async () => {
    apiMock.posts.publish.mockRejectedValue(new ApiError(422, 'SOCIAL_ACCOUNT_EXPIRED', 'This account needs reconnecting. Reconnect it in Accounts.'));
    const onLeave = renderActions();
    await publishNow();
    await waitFor(() => expect(push).toHaveBeenCalledWith(expect.stringContaining('p1')));
    expect(onLeave).toHaveBeenCalled();
    expect(apiMock.posts.publish).toHaveBeenCalledWith('p1');
    expect(toast.error).toHaveBeenCalledWith(
      'The post was saved, but publishing failed: This account needs reconnecting. Reconnect it in Accounts. Publish it again here once that is fixed.',
    );
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(toast.success).not.toHaveBeenCalled();
    expect(apiMock.posts.create).toHaveBeenCalledTimes(1);
  });

  it('stays in the composer when creating the post fails, so nothing exists to duplicate', async () => {
    apiMock.posts.create.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'bad'));
    const onLeave = renderActions();
    await publishNow();
    await waitFor(() => expect(apiMock.posts.create).toHaveBeenCalledTimes(1));
    expect(push).not.toHaveBeenCalled();
    expect(onLeave).not.toHaveBeenCalled();
    expect(apiMock.posts.publish).not.toHaveBeenCalled();
  });
});

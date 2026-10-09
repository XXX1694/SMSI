import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { ScopePicker } from '@/components/developer/scope-picker';
import { CapabilityBadges } from '@/components/capability-badges';
import { PostStatusBadge } from '@/components/status-badge';
import { ErrorState, InlineError } from '@/components/states';
import { defaultScopes } from '@/lib/scopes';
import { normalizeCapabilities } from '@/lib/normalize';

function Harness() {
  const [v, setV] = useState<string[]>(defaultScopes());
  return <ScopePicker value={v} onChange={setV} />;
}

describe('ScopePicker', () => {
  it('shows Safe / Medium / Dangerous groups with dangerous unchecked and no warning', () => {
    render(<Harness />);
    expect(screen.getByText('Safe')).toBeInTheDocument();
    expect(screen.getByText('Medium')).toBeInTheDocument();
    expect(screen.getByText('Dangerous')).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /Publish immediately/ })).not.toBeChecked();
    expect(screen.getByRole('checkbox', { name: /Read posts/ })).toBeChecked();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('warns when a dangerous scope is ticked', async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole('checkbox', { name: /Delete posts/ }));
    expect(screen.getByRole('alert')).toHaveTextContent(/Dangerous scopes/);
  });
});

describe('badges', () => {
  it('renders post status label', () => {
    render(<PostStatusBadge status="partially_published" />);
    expect(screen.getByText('Partially published')).toBeInTheDocument();
  });
  it('capability badges expose support to screen readers', () => {
    render(<CapabilityBadges caps={normalizeCapabilities({ CanPublishText: true, MaxTextLength: 280 })} />);
    expect(screen.getByText('Text').parentElement).toHaveTextContent('Supports Text');
    expect(screen.getByText('Video').parentElement).toHaveTextContent('No Video');
    expect(screen.getByText('280 chars')).toBeInTheDocument();
  });
});

const apiMock = vi.hoisted(() => ({
  social: {
    providers: vi.fn(),
    accounts: vi.fn(),
  },
  posts: { create: vi.fn(), publish: vi.fn() },
  media: { upload: vi.fn(), list: vi.fn() },
}));
vi.mock('@/lib/api', () => ({ api: apiMock, ApiError: class extends Error {} }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }));

describe('ComposerView', () => {
  beforeEach(() => {
    apiMock.social.providers.mockResolvedValue([
      { id: 'mock', name: 'Mock', configured: true, unsupported: false, available: true, capabilities: normalizeCapabilities({ CanPublishText: true, MaxTextLength: 10, MaxMediaCount: 1 }) },
    ]);
    apiMock.social.accounts.mockResolvedValue([
      { id: 'a1', provider: 'mock', username: 'm', display_name: 'Mock Account', avatar_url: null, status: 'active', scopes: [], connected_at: '' },
    ]);
    apiMock.posts.create.mockReset();
  });

  it('blocks submission and shows issues when content exceeds the platform limit', async () => {
    const { ComposerView } = await import('@/components/composer/composer-view');
    const { PrefsProvider } = await import('@/components/prefs-provider');
    const { ToastProvider } = await import('@/components/toast');
    render(
      <PrefsProvider>
        <ToastProvider>
          <ComposerView />
        </ToastProvider>
      </PrefsProvider>,
    );
    await userEvent.click(await screen.findByRole('button', { name: /Mock Account/ }));
    await userEvent.type(screen.getByLabelText('Post content'), 'this is far too long');
    await userEvent.click(screen.getByRole('button', { name: 'Save draft' }));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/over the Mock limit/));
    expect(apiMock.posts.create).not.toHaveBeenCalled();
  });
});

describe('InlineError and ErrorState title', () => {
  it('renders an alert with a retry button', async () => {
    const onRetry = vi.fn();
    render(<InlineError onRetry={onRetry}>Could not save.</InlineError>);
    expect(screen.getByRole('alert')).toHaveTextContent('Could not save.');
    screen.getByRole('button', { name: 'Try again' }).click();
    expect(onRetry).toHaveBeenCalled();
  });
  it('uses a custom ErrorState title', () => {
    render(<ErrorState title="Could not load posts" error={new Error('Offline.')} />);
    expect(screen.getByText('Could not load posts')).toBeInTheDocument();
  });
});

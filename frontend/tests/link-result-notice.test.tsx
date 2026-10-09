import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const router = vi.hoisted(() => ({ replace: vi.fn() }));
const search = vi.hoisted(() => ({ value: '' }));
vi.mock('next/navigation', () => ({ useRouter: () => router, usePathname: () => '/settings', useSearchParams: () => new URLSearchParams(search.value) }));

import { LinkResultNotice } from '@/components/link-result-notice';

beforeEach(() => router.replace.mockReset());

describe('LinkResultNotice', () => {
  it('shows nothing and leaves the address alone after a successful connect (plain /settings)', () => {
    search.value = '';
    const { container } = render(<LinkResultNotice />);
    expect(container).toBeEmptyDOMElement();
    expect(router.replace).not.toHaveBeenCalled();
  });

  it('names the provider for a known code and clears the query parameters', async () => {
    search.value = 'error=oauth_provider_error&provider=github';
    render(<LinkResultNotice />);
    expect(await screen.findByRole('alert')).toHaveTextContent('GitHub reported a problem');
    expect(router.replace).toHaveBeenCalledWith('/settings');
  });

  it('explains an account that belongs to someone else', async () => {
    search.value = 'error=identity_in_use&provider=google';
    render(<LinkResultNotice />);
    expect(await screen.findByRole('alert')).toHaveTextContent('That Google account is already connected to another Steerpost account');
  });

  it('answers an unknown code with the generic sentence and never echoes it', async () => {
    search.value = 'error=%3Cb%3Eboom%3C%2Fb%3E&provider=evil';
    render(<LinkResultNotice />);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Connecting did not work. Try again.');
    expect(alert).not.toHaveTextContent('boom');
    expect(alert).not.toHaveTextContent('evil');
  });

  it('never echoes an unknown provider of a known code: it reads as "the provider"', async () => {
    search.value = 'error=oauth_provider_error&provider=Evil%20Corp';
    render(<LinkResultNotice />);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('the provider reported a problem');
    expect(alert).not.toHaveTextContent('Evil');
  });

  it('accepts a code without a provider', async () => {
    search.value = 'error=oauth_cancelled';
    render(<LinkResultNotice />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Connecting was canceled');
  });
});

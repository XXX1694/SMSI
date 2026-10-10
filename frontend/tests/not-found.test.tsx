import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import NotFound from '@/app/not-found';
import { PrefsProvider } from '@/components/prefs-provider';
import { I18nRoot } from '@/i18n/i18n-root';

const ENABLED = ['en'] as const;

describe('not-found page', () => {
  it('says what happened and offers the dashboard, from the root scope alone', () => {
    render(
      <PrefsProvider>
        <I18nRoot enabled={ENABLED}>
          <NotFound />
        </I18nRoot>
      </PrefsProvider>,
    );
    expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument();
    expect(screen.getByText(/The link may be old or mistyped/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Go to the dashboard' })).toHaveAttribute('href', '/dashboard');
    expect(screen.getByRole('main')).toHaveAttribute('id', 'main');
  });
});

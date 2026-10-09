import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PrivacyContent } from '@/components/legal/privacy-content';
import { TermsContent } from '@/components/legal/terms-content';
import { LEGAL_VERSION, OPERATOR_CONTACT_PLACEHOLDER, readOperator } from '@/lib/legal';

describe('readOperator', () => {
  it('falls back to placeholders that say the operator must set them', () => {
    const op = readOperator({});
    expect(op.configured).toBe(false);
    expect(op.contact).toBe(OPERATOR_CONTACT_PLACEHOLDER);
    expect(op.name).toMatch(/operator sets OPERATOR_NAME/);
  });

  it('uses the environment when both values are set', () => {
    expect(readOperator({ OPERATOR_NAME: ' Acme ', OPERATOR_CONTACT: 'privacy@acme.example' })).toEqual({
      name: 'Acme',
      contact: 'privacy@acme.example',
      configured: true,
    });
  });
});

describe('legal pages', () => {
  const op = readOperator({ OPERATOR_NAME: 'Acme', OPERATOR_CONTACT: 'privacy@acme.example' });

  it('renders the Privacy Policy with the data it stores, the networks, the operator and the version', () => {
    render(<PrivacyContent operator={op} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Privacy Policy' })).toBeInTheDocument();
    expect(screen.getByText(/Version .* Effective/)).toHaveTextContent(LEGAL_VERSION);
    expect(screen.getByRole('note')).toHaveTextContent('not legal advice');
    for (const word of ['LinkedIn', 'Telegram', 'Discord', 'Mastodon', 'Bluesky', 'encrypted at rest', 'trackers']) {
      expect(document.body).toHaveTextContent(word);
    }
    expect(screen.getByRole('heading', { name: 'Export and deletion' })).toBeInTheDocument();
    expect(document.body).toHaveTextContent('privacy@acme.example');
    expect(screen.getByRole('link', { name: 'Terms of Service' })).toHaveAttribute('href', '/terms');
  });

  it('renders the Terms of Service and says plainly when the operator is not set', () => {
    render(<TermsContent operator={readOperator({})} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Terms of Service' })).toBeInTheDocument();
    expect(screen.getByRole('note')).toHaveTextContent('has not set their name and contact');
    expect(document.body).toHaveTextContent(OPERATOR_CONTACT_PLACEHOLDER);
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toHaveAttribute('href', '/privacy');
  });
});

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Badge } from '@/components/ui/badge';
import { PostStatusBadge } from '@/components/status-badge';

describe('Badge', () => {
  it('shows a status as a hidden glyph plus the label in the text colour', () => {
    const { container } = render(<PostStatusBadge status="failed" />);
    const tag = screen.getByText('Failed');
    expect(tag).toHaveClass('text-foreground', 'rounded-[5px]', 'border-danger/35');
    expect(tag).not.toHaveClass('rounded-full');
    const glyph = container.querySelector('svg[data-glyph="failed"]');
    expect(glyph).toHaveAttribute('aria-hidden');
    expect(glyph).toHaveClass('text-danger');
  });

  it('puts the tone on the label when there is no glyph', () => {
    render(<Badge tone="warning">Trusted</Badge>);
    const tag = screen.getByText('Trusted');
    expect(tag).toHaveClass('text-warning');
    expect(tag.querySelector('svg')).toBeNull();
  });

  it('shows an unknown status as plain neutral text', () => {
    const { container } = render(<PostStatusBadge status="weird" />);
    expect(screen.getByText('weird')).toHaveClass('text-muted-foreground');
    expect(container.querySelector('svg')).toBeNull();
  });
});

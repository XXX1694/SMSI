import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Badge } from '@/components/ui/badge';
import { PostStatusBadge } from '@/components/status-badge';

describe('Badge', () => {
  it('shows a status as a hidden glyph plus the label in the text colour', () => {
    const { container } = render(<PostStatusBadge status="failed" />);
    expect(screen.getByText('Failed')).toHaveClass('text-foreground');
    const tag = container.firstElementChild;
    expect(tag).toHaveClass('rounded-tag', 'border-danger/35', 'text-danger');
    expect(tag).not.toHaveClass('rounded-full');
    expect(container.querySelector('svg[data-glyph="failed"]')).toHaveAttribute('aria-hidden');
  });

  it('puts the tone on the label when there is no glyph', () => {
    render(<Badge tone="warning">Trusted</Badge>);
    const tag = screen.getByText('Trusted');
    expect(tag).toHaveClass('text-warning');
    expect(tag.querySelector('svg')).toBeNull();
  });

  it('keeps quiet statuses muted and drops the tinted border for the others', () => {
    const { container } = render(<PostStatusBadge status="draft" />);
    expect(container.firstElementChild).toHaveClass('text-muted-foreground');
    const scheduled = render(<PostStatusBadge status="scheduled" />).container.firstElementChild;
    expect(scheduled).toHaveClass('border-border');
  });

  it('shows an unknown status as plain neutral text', () => {
    const { container } = render(<PostStatusBadge status="weird" />);
    expect(screen.getByText('weird')).toHaveClass('text-muted-foreground');
    expect(container.querySelector('svg')).toBeNull();
  });
});

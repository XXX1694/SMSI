import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Logo, LogoMark } from '@/components/brand/logo';

describe('Logo', () => {
  it('shows the name as text and keeps the mark out of the accessibility tree', () => {
    const { container } = render(<Logo />);
    expect(screen.getByText('Steerpost')).toBeTruthy();
    const svg = container.querySelector('svg');
    expect(svg?.getAttribute('aria-hidden')).toBe('true');
    expect(svg?.getAttribute('focusable')).toBe('false');
  });

  it('draws the mark in the accent colour and only animates when asked', () => {
    const { container, rerender } = render(<Logo />);
    expect(container.querySelector('svg')?.getAttribute('class')).toContain('text-accent');
    expect(container.querySelector('svg')?.getAttribute('class')).not.toContain('logo-draw');
    rerender(<Logo animate />);
    expect(container.querySelector('svg')?.getAttribute('class')).toContain('logo-draw');
  });

  it('uses currentColor so the mark follows the theme', () => {
    const { container } = render(<LogoMark />);
    for (const el of container.querySelectorAll('path')) expect(el.getAttribute('fill') === 'currentColor' || el.getAttribute('stroke') === 'currentColor').toBe(true);
  });
});

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { CapabilityBadges } from '@/components/capability-badges';
import type { Capabilities } from '@/lib/types';

const caps = { canPublishText: true, canPublishImage: false, canPublishVideo: false, canSchedule: true, canDelete: false, canAnalytics: false, maxTextLength: 280 } as Capabilities;

describe('CapabilityBadges', () => {
  it('tells screen readers what is not supported and strikes unsupported items without fading them (contrast)', () => {
    render(<CapabilityBadges caps={caps} />);
    const off = screen.getByText('Image').parentElement as HTMLElement;
    expect(screen.getAllByText(/^Does not support /).length).toBe(4);
    expect(screen.getAllByText(/^Supports /).length).toBe(2);
    expect(off.className).toContain('line-through');
    expect(off.className).not.toMatch(/opacity-/);
  });
});

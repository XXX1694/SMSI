import { describe, expect, it } from 'vitest';
import { normalizeProvider, unwrapList } from '@/lib/normalize';

describe('normalizeProvider', () => {
  it('accepts PascalCase capabilities from the contract', () => {
    const p = normalizeProvider({ id: 'linkedin', configured: true, capabilities: { CanPublishText: true, CanPublishImage: true, MaxTextLength: 3000, RequiresApproval: true, Notes: 'x' } });
    expect(p.available).toBe(true);
    expect(p.capabilities).toMatchObject({ canPublishText: true, canPublishImage: true, canPublishVideo: false, maxTextLength: 3000, requiresApproval: true, notes: 'x' });
    expect(p.name).toBe('LinkedIn');
  });
  it('accepts snake_case', () => {
    expect(normalizeProvider({ provider: 'telegram', capabilities: { can_publish_text: true, max_text_length: 4096 } }).capabilities.maxTextLength).toBe(4096);
  });
  it('marks unsupported stubs and unconfigured providers as not available', () => {
    expect(normalizeProvider({ provider: 'instagram', status: 'unsupported', capabilities: { RequiresApproval: true } }).available).toBe(false);
    expect(normalizeProvider({ provider: 'x', configured: false, capabilities: { CanPublishText: true } }).available).toBe(false);
    expect(normalizeProvider({ provider: 'tiktok', capabilities: {} }).available).toBe(false);
  });
  it('unwrapList accepts arrays and pages', () => {
    expect(unwrapList([1])).toEqual([1]);
    expect(unwrapList({ items: [2] })).toEqual([2]);
    expect(unwrapList(null)).toEqual([]);
  });
});

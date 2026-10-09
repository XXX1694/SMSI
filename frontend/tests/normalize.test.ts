import { describe, expect, it } from 'vitest';
import { normalizeMe, normalizeProvider, unwrapList } from '@/lib/normalize';

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

describe('normalizeProvider connect fields', () => {
  it('reads connect_method and connect_fields, treating kind secret as a secret and dropping nameless fields', () => {
    const p = normalizeProvider({
      provider: 'mastodon',
      capabilities: {
        can_publish_text: true,
        connect_method: 'token',
        connect_fields: [{ name: 'access_token', label: 'Access token', kind: 'secret', required: true }, { label: 'no name' }, { name: 'u', kind: 'weird' }],
      },
    });
    expect(p.capabilities.connectMethod).toBe('token');
    expect(p.capabilities.connectFields).toEqual([
      { name: 'access_token', label: 'Access token', help: '', placeholder: '', kind: 'secret', required: true, secret: true },
      { name: 'u', label: 'u', help: '', placeholder: '', kind: 'text', required: false, secret: false },
    ]);
  });

  it('has no fields for providers that do not use a token', () => {
    const p = normalizeProvider({ provider: 'linkedin', capabilities: { can_publish_text: true, connect_method: 'oauth' } });
    expect(p.capabilities).toMatchObject({ connectMethod: 'oauth', connectFields: [] });
  });
});

describe('normalizeMe deletion fields', () => {
  it('reads the schedule and the grace period, with safe defaults for older servers', () => {
    const now = normalizeMe({ id: 'u', email: 'a@b.c', user: { deletion_scheduled_at: '2026-10-16T12:00:00Z' }, deletion_grace_days: 14 });
    expect(now.deletion_scheduled_at).toBe('2026-10-16T12:00:00Z');
    expect(now.deletion_grace_days).toBe(14);
    const old = normalizeMe({ id: 'u', email: 'a@b.c', user: {} });
    expect(old.deletion_scheduled_at).toBeNull();
    expect(old.deletion_grace_days).toBe(7);
  });
});

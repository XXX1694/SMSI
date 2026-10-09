import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, setCsrfToken } from '@/lib/api';

const media = { id: 'm1', kind: 'video', mime_type: 'video/mp4', size_bytes: 3, original_name: 'a.mp4' };

function stubFetch() {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(media), { status: 201 }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('media upload target (D-015)', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    setCsrfToken(null);
  });

  it('goes straight to the API host with credentials and the CSRF header, not through the 10 MB proxy', async () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', 'https://api.example.test/');
    setCsrfToken('csrf-1');
    const fetchMock = stubFetch();
    await api.media.upload(new File(['abc'], 'a.mp4', { type: 'video/mp4' }));
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('https://api.example.test/api/v1/media');
    expect(init.credentials).toBe('include');
    expect((init.headers as Record<string, string>)['X-CSRF-Token']).toBe('csrf-1');
    expect(init.body).toBeInstanceOf(FormData);
  });

  it('falls back to the same-origin proxy when no API URL is configured', async () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', '');
    const fetchMock = stubFetch();
    await api.media.upload(new File(['abc'], 'a.mp4', { type: 'video/mp4' }));
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/v1/media');
    expect(init.credentials).toBe('same-origin');
  });

  it('keeps every other call on the proxy', async () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', 'https://api.example.test');
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ items: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await api.media.list();
    expect((fetchMock.mock.calls[0] as unknown as [string])[0]).toBe('/api/v1/media');
  });
});

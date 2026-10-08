import type { WireProvider } from './model';

const caps = (o: Partial<WireProvider['capabilities']>): WireProvider['capabilities'] => ({
  can_publish_text: false,
  can_publish_image: false,
  can_publish_video: false,
  can_schedule: false,
  can_delete: false,
  can_analytics: false,
  max_text_length: 0,
  max_media_count: 0,
  requires_approval: false,
  notes: '',
  ...o,
});

/** Mirrors `GET /social/providers` of the real backend (docs/ARCHITECTURE.md section 2). */
export const PROVIDERS: readonly WireProvider[] = [
  {
    provider: 'linkedin',
    configured: true,
    status: 'supported',
    capabilities: caps({
      can_publish_text: true,
      can_publish_image: true,
      max_text_length: 3000,
      max_media_count: 9,
      requires_approval: true,
      notes: 'Company pages need Marketing Developer Platform approval. Video is not supported yet.',
    }),
  },
  {
    provider: 'telegram',
    configured: true,
    status: 'supported',
    capabilities: caps({
      can_publish_text: true,
      can_publish_image: true,
      can_publish_video: true,
      can_delete: true,
      max_text_length: 4096,
      max_media_count: 10,
      notes:
        "Add the SocialOS bot as an admin with 'Post messages' to your channel or group, then post the one-time code SocialOS gives you there to prove you control it.",
    }),
  },
  {
    provider: 'mock',
    configured: true,
    status: 'supported',
    capabilities: caps({
      can_publish_text: true,
      can_publish_image: true,
      can_publish_video: true,
      can_schedule: true,
      can_delete: true,
      can_analytics: true,
      max_text_length: 280,
      max_media_count: 4,
      notes: 'Deterministic mock provider for testing. In this demo, any post containing FAIL fails to publish, on every network.',
    }),
  },
  ...['instagram', 'facebook', 'tiktok', 'youtube', 'x', 'threads', 'pinterest'].map(
    (provider): WireProvider => ({
      provider,
      configured: false,
      status: 'unsupported',
      capabilities: caps({ requires_approval: true, notes: 'Registered stub: returns PROVIDER_NOT_AVAILABLE.' }),
    }),
  ),
];

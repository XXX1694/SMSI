import type { WireConnectField, WireProvider } from './model';

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

const tokenProvider = (
  provider: string,
  c: Partial<WireProvider['capabilities']>,
  connect_fields: WireConnectField[],
): WireProvider => ({
  provider,
  configured: true,
  status: 'supported',
  capabilities: caps({ ...c, connect_method: 'token', connect_fields }),
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
        "Add the Steerpost bot as an admin with 'Post messages' to your channel or group, then post the one-time code Steerpost gives you there to prove you control it.",
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
  tokenProvider('discord', {
    can_publish_text: true,
    can_publish_image: true,
    can_delete: true,
    max_text_length: 2000,
    max_media_count: 10,
    notes:
      "Posts into one channel through its webhook, as the webhook's name. Text up to 2000 characters and up to 10 images; mentions are not pinged. No titles, video, threads or scheduling on Discord's side.",
  }, [
    {
      name: 'webhook_url', label: 'Webhook URL', kind: 'url', secret: true, required: true,
      placeholder: 'https://discord.com/api/webhooks/...',
      help: 'Channel settings > Integrations > Webhooks > New Webhook > Copy Webhook URL. The URL is a password: anyone who has it can post in that channel.',
    },
  ]),
  tokenProvider('mastodon', {
    can_publish_text: true,
    can_publish_image: true,
    max_text_length: 500,
    max_media_count: 4,
    notes: 'Mastodon and compatible servers. Posts are public; images only (no video). Limits are read from your instance when you connect.',
  }, [
    {
      name: 'instance_url', label: 'Instance URL', kind: 'url', required: true, placeholder: 'https://mastodon.social',
      help: 'The https address of your server. Servers on private networks cannot be connected.',
    },
    {
      name: 'access_token', label: 'Access token', kind: 'secret', secret: true, required: true,
      help: 'On your server: Preferences > Development > New application. Tick write:statuses, write:media and read:accounts, then copy "Your access token".',
    },
  ]),
  tokenProvider('bluesky', {
    can_publish_text: true,
    can_publish_image: true,
    can_delete: true,
    max_text_length: 300,
    max_media_count: 4,
    notes: 'Text up to 300 characters, up to 4 images of 2 MB each without alt text. Links and hashtags become clickable; mentions are not linked. Uses an app password. Steerpost schedules; Bluesky has no native scheduling.',
  }, [
    { name: 'handle', label: 'Handle', kind: 'text', required: true, placeholder: 'name.bsky.social', help: 'Your Bluesky handle, for example name.bsky.social.' },
    {
      name: 'app_password', label: 'App password', kind: 'secret', secret: true, required: true, placeholder: 'xxxx-xxxx-xxxx-xxxx',
      help: 'Create one in Settings > Privacy and security > App passwords. Never use your main password.',
    },
    { name: 'pds', label: 'Server (optional)', kind: 'url', required: false, placeholder: 'https://bsky.social', help: 'Only if you host your own PDS. Leave empty for bsky.social.' },
  ]),
  ...['instagram', 'facebook', 'tiktok', 'youtube', 'x', 'threads', 'pinterest'].map(
    (provider): WireProvider => ({
      provider,
      configured: false,
      status: 'unsupported',
      capabilities: caps({ requires_approval: true, notes: 'Registered stub: returns PROVIDER_NOT_AVAILABLE.' }),
    }),
  ),
];

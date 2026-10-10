import type { Messages } from '@/i18n/catalog';

/**
 * The messages a component sees with no `MessagesScope` above it. Empty in the app: every route gets its English from a
 * scope, so this module keeps the whole catalog out of the shared chunk. Vitest aliases it to the full English catalog
 * (tests/helpers/full-english.ts), so a component rendered on its own in a test still reads English.
 */
export const FALLBACK_BUNDLES: Partial<Messages> = {};

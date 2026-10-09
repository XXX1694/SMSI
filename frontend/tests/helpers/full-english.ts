import type { Messages } from '@/i18n/catalog';
import en from '@/i18n/en-all';

/** Stands in for src/i18n/fallback-bundles.ts under Vitest (see vitest.config.ts): a component alone reads full English. */
export const FALLBACK_BUNDLES: Partial<Messages> = en;

import { formatNumber } from '@/i18n/format';
import type { AppT } from '@/i18n/translate';

export const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
export const MAX_VIDEO_BYTES = 100 * 1024 * 1024;
export const IMAGE_TYPES = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'] as const;
export const VIDEO_TYPES = ['video/mp4', 'video/quicktime'] as const;
export const ACCEPT_ATTR = [...IMAGE_TYPES, ...VIDEO_TYPES].join(',');

export function mediaKind(mime: string): 'image' | 'video' | null {
  if ((IMAGE_TYPES as readonly string[]).includes(mime)) return 'image';
  if ((VIDEO_TYPES as readonly string[]).includes(mime)) return 'video';
  return null;
}

/** Returns an error message, or null if the file is acceptable. */
export function validateMediaFile(file: { name: string; type: string; size: number }, t: AppT): string | null {
  const kind = mediaKind(file.type);
  if (!kind) return t('media.unsupportedType', { name: file.name });
  const limit = kind === 'image' ? MAX_IMAGE_BYTES : MAX_VIDEO_BYTES;
  if (file.size > limit) {
    return t(kind === 'image' ? 'media.imageTooLarge' : 'media.videoTooLarge', { name: file.name, size: formatBytes(file.size, t), limit: formatBytes(limit, t) });
  }
  if (file.size === 0) return t('media.emptyFile', { name: file.name });
  return null;
}

/** "512 B", "340 KB", "2.5 MB". The number follows the locale; the units come from the catalog. */
export function formatBytes(n: number, t: AppT): string {
  const num = (v: number, digits: number) => formatNumber(v, t.locale, { minimumFractionDigits: digits, maximumFractionDigits: digits });
  if (n < 1024) return t('common.bytes', { size: num(n, 0) });
  if (n < 1024 * 1024) return t('common.kilobytes', { size: num(n / 1024, 0) });
  return t('common.megabytes', { size: num(n / 1024 / 1024, n < 10 * 1024 * 1024 ? 1 : 0) });
}

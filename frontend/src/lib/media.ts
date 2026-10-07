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
export function validateMediaFile(file: { name: string; type: string; size: number }): string | null {
  const kind = mediaKind(file.type);
  if (!kind) return `${file.name}: unsupported type. Use JPEG, PNG, WebP, GIF, MP4 or MOV.`;
  const limit = kind === 'image' ? MAX_IMAGE_BYTES : MAX_VIDEO_BYTES;
  if (file.size > limit) {
    return `${file.name}: too large (${formatBytes(file.size)}). ${kind === 'image' ? 'Images' : 'Videos'} are limited to ${formatBytes(limit)}.`;
  }
  if (file.size === 0) return `${file.name}: file is empty.`;
  return null;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / 1024 / 1024).toFixed(n < 10 * 1024 * 1024 ? 1 : 0)} MB`;
}

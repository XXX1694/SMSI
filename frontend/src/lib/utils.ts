import { clsx, type ClassValue } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

// Teach the merger the custom token names from tailwind.config.ts; otherwise `text-compact` is read as a text colour
// and no longer replaces the `text-sm` in a component's base classes.
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [{ text: ['2xs', 'compact'] }],
      shadow: [{ shadow: ['pop'] }],
      z: [{ z: ['banner', 'overlay', 'toast'] }],
    },
  },
});

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}

import react from '@vitejs/plugin-react';
import path from 'node:path';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: [
      // Components rendered on their own read full English; the app's fallback is empty (see src/i18n/fallback-bundles.ts).
      { find: '@/i18n/fallback-bundles', replacement: path.resolve(__dirname, 'tests/helpers/full-english.ts') },
      { find: '@', replacement: path.resolve(__dirname, 'src') },
    ],
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./tests/setup.ts'],
    include: ['tests/**/*.test.{ts,tsx}'],
    globals: true,
  },
});

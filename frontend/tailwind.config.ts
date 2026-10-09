import type { Config } from 'tailwindcss';

const config: Config = {
  darkMode: 'class',
  content: ['./src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Inter Variable"', 'Inter', 'ui-sans-serif', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      colors: {
        background: 'hsl(var(--background) / <alpha-value>)',
        foreground: 'hsl(var(--foreground) / <alpha-value>)',
        muted: { DEFAULT: 'hsl(var(--muted) / <alpha-value>)', foreground: 'hsl(var(--muted-foreground) / <alpha-value>)' },
        border: 'hsl(var(--border) / <alpha-value>)',
        input: 'hsl(var(--input) / <alpha-value>)',
        ring: 'hsl(var(--ring) / <alpha-value>)',
        surface: 'hsl(var(--surface) / <alpha-value>)',
        accent: { DEFAULT: 'hsl(var(--accent) / <alpha-value>)', foreground: 'hsl(var(--accent-foreground) / <alpha-value>)', soft: 'hsl(var(--accent-soft) / <alpha-value>)' },
        success: { DEFAULT: 'hsl(var(--success) / <alpha-value>)', soft: 'hsl(var(--success-soft) / <alpha-value>)' },
        warning: { DEFAULT: 'hsl(var(--warning) / <alpha-value>)', soft: 'hsl(var(--warning-soft) / <alpha-value>)' },
        danger: { DEFAULT: 'hsl(var(--danger) / <alpha-value>)', soft: 'hsl(var(--danger-soft) / <alpha-value>)' },
      },
      fontSize: {
        '2xs': ['var(--text-2xs)', 'var(--text-2xs-leading)'],
        xs: ['var(--text-xs)', 'var(--text-xs-leading)'],
        compact: ['var(--text-compact)', 'var(--text-compact-leading)'],
        sm: ['var(--text-sm)', 'var(--text-sm-leading)'],
        base: ['var(--text-base)', 'var(--text-base-leading)'],
        lg: ['var(--text-lg)', 'var(--text-lg-leading)'],
        xl: ['var(--text-xl)', 'var(--text-xl-leading)'],
        '2xl': ['var(--text-2xl)', 'var(--text-2xl-leading)'],
        '3xl': ['var(--text-3xl)', 'var(--text-3xl-leading)'],
      },
      spacing: { gutter: 'var(--space-gutter)', stack: 'var(--space-stack)', section: 'var(--space-section)' },
      borderRadius: { sm: 'var(--radius-sm)', md: 'var(--radius-md)', lg: 'var(--radius-lg)', xl: 'var(--radius-xl)' },
      boxShadow: { md: 'var(--shadow-md)', lg: 'var(--shadow-lg)', pop: 'var(--shadow-pop)' },
      zIndex: { banner: 'var(--z-banner)', overlay: 'var(--z-overlay)', toast: 'var(--z-toast)' },
      transitionDuration: { DEFAULT: 'var(--duration-fast)', fast: 'var(--duration-fast)', base: 'var(--duration-base)' },
      transitionTimingFunction: { DEFAULT: 'var(--ease-standard)', standard: 'var(--ease-standard)' },
    },
  },
  plugins: [],
};

export default config;

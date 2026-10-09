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
      transitionDuration: { DEFAULT: 'var(--duration-fast)', fast: 'var(--duration-fast)', base: 'var(--duration-base)', slow: 'var(--duration-slow)' },
      transitionTimingFunction: { DEFAULT: 'var(--ease-standard)', standard: 'var(--ease-standard)', out: 'var(--ease-out)', in: 'var(--ease-in)' },
      // transform and opacity only; durations and easings come from tokens.css. Reduced motion is handled in globals.css.
      keyframes: {
        'fade-in': { from: { opacity: '0' }, to: { opacity: '1' } },
        'fade-out': { from: { opacity: '1' }, to: { opacity: '0' } },
        'rise-in': { from: { opacity: '0', transform: 'translateY(var(--distance-enter))' }, to: { opacity: '1', transform: 'none' } },
        'pop-in': { from: { opacity: '0', transform: 'translate(-50%, -48%) scale(0.97)' }, to: { opacity: '1', transform: 'translate(-50%, -50%) scale(1)' } },
        'pop-out': { from: { opacity: '1', transform: 'translate(-50%, -50%) scale(1)' }, to: { opacity: '0', transform: 'translate(-50%, -48%) scale(0.97)' } },
        'toast-in': { from: { opacity: '0', transform: 'translateX(1.5rem)' }, to: { opacity: '1', transform: 'none' } },
        'toast-out': { from: { opacity: '1', transform: 'none' }, to: { opacity: '0', transform: 'translateX(1.5rem)' } },
        shimmer: { '100%': { transform: 'translateX(100%)' } },
      },
      animation: {
        'fade-in': 'fade-in var(--duration-base) var(--ease-out) both',
        'fade-out': 'fade-out var(--duration-fast) var(--ease-in) both',
        'rise-in': 'rise-in var(--duration-slow) var(--ease-out) both',
        'pop-in': 'pop-in var(--duration-base) var(--ease-out) both',
        'pop-out': 'pop-out var(--duration-fast) var(--ease-in) both',
        'toast-in': 'toast-in var(--duration-slow) var(--ease-out) both',
        'toast-out': 'toast-out var(--duration-fast) var(--ease-in) both',
        shimmer: 'shimmer 1.4s var(--ease-standard) infinite',
      },
    },
  },
  plugins: [],
};

export default config;

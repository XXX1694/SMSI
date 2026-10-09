import type { Config } from 'tailwindcss';

// Tailwind accepts an array of fallback values at runtime (one declaration each) but its types only allow a string.
const SCREEN_HEIGHT = [
  'calc(100vh - env(safe-area-inset-top) - env(safe-area-inset-bottom))',
  'calc(100dvh - env(safe-area-inset-top) - env(safe-area-inset-bottom))',
] as unknown as string;

const config: Config = {
  darkMode: 'class',
  content: ['./src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        // Onest for Latin and Cyrillic; --font-script is Noto Sans Arabic / JP / SC on those locales only (globals.css, D-024).
        sans: ['"Onest Variable"', 'var(--font-script)', 'ui-sans-serif', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
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
        info: { DEFAULT: 'hsl(var(--info) / <alpha-value>)', soft: 'hsl(var(--info-soft) / <alpha-value>)' },
        secondary: {
          DEFAULT: 'hsl(var(--secondary) / <alpha-value>)',
          foreground: 'hsl(var(--secondary-foreground) / <alpha-value>)',
          strong: 'hsl(var(--secondary-strong) / <alpha-value>)',
        },
        canvas: 'hsl(var(--canvas) / <alpha-value>)',
        // A complete colour with its alpha built in: the dim behind dialogs.
        scrim: 'var(--scrim)',
        chart: {
          1: 'hsl(var(--chart-1) / <alpha-value>)',
          2: 'hsl(var(--chart-2) / <alpha-value>)',
          3: 'hsl(var(--chart-3) / <alpha-value>)',
          4: 'hsl(var(--chart-4) / <alpha-value>)',
        },
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
      // Screen-height layouts: the body is padded by the safe-area insets (globals.css), so subtract them (vh first as a fallback, then dvh for mobile browser bars). Zero insets give plain 100vh/100dvh.
      height: { screen: SCREEN_HEIGHT, tag: 'var(--size-tag)', count: 'var(--size-count)' },
      minHeight: { screen: SCREEN_HEIGHT },
      minWidth: { count: 'var(--size-count)' },
      spacing: { gutter: 'var(--space-gutter)', stack: 'var(--space-stack)', section: 'var(--space-section)' },
      borderRadius: { sm: 'var(--radius-sm)', md: 'var(--radius-md)', lg: 'var(--radius-lg)', xl: 'var(--radius-xl)', tag: 'var(--radius-tag)' },
      // Glass surfaces are the .glass-* classes in globals.css (D-024), not utilities: blur, tint, edge and fallbacks travel together.
      boxShadow: { md: 'var(--shadow-md)', lg: 'var(--shadow-lg)', pop: 'var(--shadow-pop)' },
      zIndex: { banner: 'var(--z-banner)', overlay: 'var(--z-overlay)', toast: 'var(--z-toast)' },
      transitionDuration: { DEFAULT: 'var(--duration-fast)', fast: 'var(--duration-fast)', base: 'var(--duration-base)', slow: 'var(--duration-slow)', hero: 'var(--duration-hero)' },
      transitionTimingFunction: {
        DEFAULT: 'var(--ease-standard)',
        standard: 'var(--ease-standard)',
        enter: 'var(--ease-enter)',
        exit: 'var(--ease-exit)',
        fill: 'var(--ease-fill)',
        steer: 'var(--ease-steer)',
      },
      // transform and opacity only; durations and easings come from tokens.css. Reduced motion is handled in globals.css.
      keyframes: {
        'fade-in': { from: { opacity: '0' }, to: { opacity: '1' } },
        'pop-in': { from: { opacity: '0', transform: 'translate(-50%, -48%) scale(0.97)' }, to: { opacity: '1', transform: 'translate(-50%, -50%) scale(1)' } },
        'pop-out': { from: { opacity: '1', transform: 'translate(-50%, -50%) scale(1)' }, to: { opacity: '0', transform: 'translate(-50%, -48%) scale(0.97)' } },
        'toast-in': { from: { opacity: '0', transform: 'translateX(1.5rem)' }, to: { opacity: '1', transform: 'none' } },
        'toast-out': { from: { opacity: '1', transform: 'none' }, to: { opacity: '0', transform: 'translateX(1.5rem)' } },
      },
      animation: {
        'fade-in': 'fade-in var(--duration-base) var(--ease-enter) both',
        'fade-out': 'fade-out var(--duration-fast) var(--ease-exit) both',
        'pop-in': 'pop-in var(--duration-base) var(--ease-enter) both',
        'pop-out': 'pop-out var(--duration-fast) var(--ease-exit) both',
        'toast-in': 'toast-in var(--duration-slow) var(--ease-enter) both',
        'toast-out': 'toast-out var(--duration-fast) var(--ease-exit) both',
      },
    },
  },
  plugins: [],
};

export default config;

'use client';
import {
  BarChart3,
  CalendarDays,
  Code2,
  FileText,
  Image as ImageIcon,
  LayoutDashboard,
  Link2,
  LogOut,
  Menu,
  PenSquare,
  Settings,
  ShieldCheck,
  X,
} from 'lucide-react';
import { useTranslations } from '@/i18n/use-translations';
import { usePathname, useRouter } from 'next/navigation';
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ComponentType, type ReactNode, type RefObject } from 'react';
import { useAuth } from '@/components/auth-provider';
import { usePendingApprovals } from '@/components/approvals/use-pending-approvals';
import { Logo } from '@/components/brand/logo';
import { EmailBanner } from '@/components/email-banner';
import { PageTransition } from '@/components/page-transition';
import { TransitionLink } from '@/components/transition-link';
import { LegalLinks } from '@/components/legal/legal-links';
import { Button } from '@/components/ui/button';
import { LanguageSelect } from '@/i18n/language-select';
import { useLocaleSettings } from '@/i18n/locale-provider';
import { StatusGlyph } from '@/components/ui/status-glyph';
import { cn } from '@/lib/utils';
import type en from '../../messages/en.json';

interface NavItem {
  href: string;
  labelKey: keyof typeof en.nav;
  icon: ComponentType<{ className?: string }>;
}

const NAV: NavItem[] = [
  { href: '/dashboard', labelKey: 'dashboard', icon: LayoutDashboard },
  { href: '/compose', labelKey: 'compose', icon: PenSquare },
  { href: '/posts', labelKey: 'posts', icon: FileText },
  { href: '/calendar', labelKey: 'calendar', icon: CalendarDays },
  { href: '/media', labelKey: 'media', icon: ImageIcon },
  { href: '/analytics', labelKey: 'analytics', icon: BarChart3 },
  { href: '/accounts', labelKey: 'accounts', icon: Link2 },
  { href: '/approvals', labelKey: 'approvals', icon: ShieldCheck },
];
const NAV_BOTTOM: NavItem[] = [
  { href: '/developer', labelKey: 'developer', icon: Code2 },
  { href: '/settings', labelKey: 'settings', icon: Settings },
];

function NavLink({ item, onNavigate, badge }: { item: NavItem; onNavigate: () => void; badge?: number | null }) {
  const pathname = usePathname();
  const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
  const Icon = item.icon;
  const t = useTranslations('nav');
  const ts = useTranslations('shell');
  return (
    <TransitionLink
      href={item.href}
      onClick={onNavigate}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'relative z-10 flex items-center gap-2.5 rounded-md px-2.5 py-3 text-sm transition-colors md:py-1.5',
        // Forced colours drop the sliding highlight, so the active row also gets an outline there.
        active
          ? 'font-medium text-secondary-foreground group-data-[indicator=off]/nav:bg-secondary forced-colors:outline forced-colors:outline-1 forced-colors:-outline-offset-1'
          : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground',
      )}
    >
      <Icon className="h-4 w-4" aria-hidden />
      {t(item.labelKey)}
      {badge ? (
        <span className="ml-auto inline-grid h-count min-w-count place-items-center rounded-tag bg-foreground/10 px-1 text-2xs font-semibold leading-none tabular-nums text-foreground">
          <span aria-hidden>{ts('navBadge', { count: badge, over: String(badge > 99) })}</span>
          <span className="sr-only">{ts('navWaiting', { count: badge })}</span>
        </span>
      ) : null}
    </TransitionLink>
  );
}

/** The highlight behind the active item. It is measured from the DOM and slides with a transform. */
function useActiveIndicator(pathname: string, open: boolean) {
  const nav = useRef<HTMLElement>(null);
  const [box, setBox] = useState<{ y: number; h: number } | null>(null);
  const [settled, setSettled] = useState(false);
  const measure = useCallback(() => {
    const el = nav.current?.querySelector<HTMLElement>('[aria-current="page"]');
    // A hidden sidebar (mobile, closed) measures 0; show nothing until it is visible.
    const next = el && el.offsetHeight > 0 ? { y: el.offsetTop, h: el.offsetHeight } : null;
    setBox((cur) => (cur?.y === next?.y && cur?.h === next?.h ? cur : next));
  }, []);
  useLayoutEffect(measure, [measure, pathname, open]);
  // Re-measure when the layout changes without a navigation: viewport resize (mobile <-> desktop), web font load.
  useEffect(() => {
    const el = nav.current;
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure);
    if (el) observer?.observe(el);
    void document.fonts?.ready.then(measure);
    window.addEventListener('resize', measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', measure);
    };
  }, [measure]);
  // No slide on the very first placement, only on later moves.
  useLayoutEffect(() => {
    if (box) setSettled(true);
  }, [box]);
  return { nav, box, settled };
}

/** Escape closes the mobile menu and hands focus back to the button that opened it. */
function useEscapeToClose(open: boolean, setOpen: (v: boolean) => void, returnFocusTo: RefObject<HTMLElement | null>) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      setOpen(false);
      returnFocusTo.current?.focus();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, setOpen, returnFocusTo]);
}

/** First tab stop: jumps over the sidebar to the page content. */
function SkipLink() {
  const t = useTranslations('shell');
  return (
    <a
      href="#main"
      className="sr-only focus-visible:not-sr-only focus-visible:fixed focus-visible:left-3 focus-visible:top-10 focus-visible:z-toast focus-visible:rounded-md focus-visible:border focus-visible:bg-background focus-visible:px-4 focus-visible:py-2.5 focus-visible:text-sm focus-visible:font-medium"
    >
      {t('skipToContent')}
    </a>
  );
}

function SidebarFooter({ email, name, onSignOut }: { email?: string; name?: string; onSignOut: () => void }) {
  const { available } = useLocaleSettings();
  const t = useTranslations('shell');
  return (
    <>
      <LegalLinks className="mt-3 px-2.5" />
      {available.length > 1 ? (
        <div className="mt-3 px-2.5">
          <LanguageSelect compact />
        </div>
      ) : null}
      <div className="mt-3 flex items-center justify-between gap-2 border-t px-2.5 pt-3">
        <span className="truncate text-xs text-muted-foreground" title={email}>
          {name || email}
        </span>
        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onSignOut} aria-label={t('signOut')}>
          <LogOut className="h-4 w-4" aria-hidden />
        </Button>
      </div>
    </>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const t = useTranslations('shell');
  const [open, setOpen] = useState(false);
  const { user, logout } = useAuth();
  const router = useRouter();
  const toggle = useRef<HTMLButtonElement>(null);
  const close = () => setOpen(false);
  const pending = usePendingApprovals();
  const pathname = usePathname();
  const { nav, box, settled } = useActiveIndicator(pathname, open);

  useEscapeToClose(open, setOpen, toggle);

  async function signOut() {
    await logout();
    router.replace('/login');
  }

  return (
    <div className="min-h-screen md:flex">
      <SkipLink />
      <header data-app-header className="glass-chrome sticky top-0 z-30 flex h-14 items-center justify-between border-b px-4 md:hidden">
        <Logo animate />
        {pending ? (
          <TransitionLink href="/approvals" className="ml-auto mr-2 inline-flex min-h-11 items-center whitespace-nowrap rounded-md px-3 text-xs font-medium tabular-nums text-foreground hover:bg-secondary/60">
            <span aria-hidden className="inline-flex items-center gap-1.5 sm:hidden">
              <StatusGlyph name="waiting" className="text-warning" />
              {pending}
            </span>
            <span className="sr-only sm:not-sr-only">{t('requestsWaiting', { count: pending })}</span>
          </TransitionLink>
        ) : null}
        <Button ref={toggle} variant="ghost" size="icon" onClick={() => setOpen((o) => !o)} aria-label={open ? t('closeMenu') : t('openMenu')} aria-expanded={open} aria-controls="sidebar">
          {open ? <X className="h-4 w-4" aria-hidden /> : <Menu className="h-4 w-4" aria-hidden />}
        </Button>
      </header>
      <aside
        id="sidebar"
        className={cn(
          // The open mobile menu sits in the scrolling page, and blur is never applied to anything that scrolls (BRAND.md section 4).
          'glass-chrome flex-col border-b p-3 max-md:backdrop-filter-none md:sticky md:top-0 md:flex md:h-screen md:w-56 md:shrink-0 md:border-b-0 md:border-r',
          open ? 'flex' : 'hidden',
        )}
      >
        <div className="mb-4 hidden px-2.5 pt-1 md:block">
          <Logo animate />
        </div>
        <nav ref={nav} aria-label={t('mainNav')} data-indicator={box ? 'on' : 'off'} className="group/nav relative flex flex-1 flex-col gap-0.5">
          <span
            aria-hidden
            data-testid="nav-indicator"
            className={cn(
              'pointer-events-none absolute inset-x-0 top-0 rounded-md bg-secondary before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-accent',
              settled && 'transition-transform duration-base ease-enter',
              !box && 'hidden',
            )}
            style={box ? { height: box.h, transform: `translateY(${box.y}px)` } : undefined}
          />
          {NAV.map((i) => (
            <NavLink key={i.href} item={i} onNavigate={close} badge={i.href === '/approvals' ? pending : null} />
          ))}
          <div className="mt-auto flex flex-col gap-0.5 pt-4">
            {NAV_BOTTOM.map((i) => (
              <NavLink key={i.href} item={i} onNavigate={close} />
            ))}
          </div>
        </nav>
        <SidebarFooter email={user?.email} name={user?.display_name} onSignOut={signOut} />
      </aside>
      <main id="main" tabIndex={-1} className="min-w-0 flex-1 focus:outline-none">
        <EmailBanner />
        <div className="mx-auto w-full max-w-6xl px-4 py-8 md:px-10 md:py-12">
          <PageTransition>{children}</PageTransition>
        </div>
      </main>
    </div>
  );
}

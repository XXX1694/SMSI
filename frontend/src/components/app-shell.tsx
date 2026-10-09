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
import { usePathname, useRouter } from 'next/navigation';
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ComponentType, type ReactNode } from 'react';
import { useAuth } from '@/components/auth-provider';
import { usePendingApprovals } from '@/components/approvals/use-pending-approvals';
import { DeletionBanner } from '@/components/deletion-banner';
import { Logo } from '@/components/brand/logo';
import { EmailBanner } from '@/components/email-banner';
import { PageTransition } from '@/components/page-transition';
import { TransitionLink } from '@/components/transition-link';
import { LegalLinks } from '@/components/legal/legal-links';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface NavItem {
  href: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
}

const NAV: NavItem[] = [
  { href: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { href: '/compose', label: 'Compose', icon: PenSquare },
  { href: '/posts', label: 'Posts', icon: FileText },
  { href: '/calendar', label: 'Calendar', icon: CalendarDays },
  { href: '/media', label: 'Media', icon: ImageIcon },
  { href: '/analytics', label: 'Analytics', icon: BarChart3 },
  { href: '/accounts', label: 'Accounts', icon: Link2 },
  { href: '/approvals', label: 'Approvals', icon: ShieldCheck },
];
const NAV_BOTTOM: NavItem[] = [
  { href: '/developer', label: 'Developer', icon: Code2 },
  { href: '/settings', label: 'Settings', icon: Settings },
];

function NavLink({ item, onNavigate, badge }: { item: NavItem; onNavigate: () => void; badge?: number | null }) {
  const pathname = usePathname();
  const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
  const Icon = item.icon;
  return (
    <TransitionLink
      href={item.href}
      onClick={onNavigate}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'relative z-10 flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors',
        active ? 'font-medium text-foreground group-data-[indicator=off]/nav:bg-muted' : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground',
      )}
    >
      <Icon className="h-4 w-4" aria-hidden />
      {item.label}
      {badge ? (
        <span className="ml-auto rounded-full bg-warning-soft px-1.5 text-xs font-medium text-warning">
          {badge > 99 ? '99+' : badge}
          <span className="sr-only"> waiting</span>
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

export function AppShell({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const { user, logout } = useAuth();
  const router = useRouter();
  const close = () => setOpen(false);
  const pending = usePendingApprovals();
  const pathname = usePathname();
  const { nav, box, settled } = useActiveIndicator(pathname, open);

  async function signOut() {
    await logout();
    router.replace('/login');
  }

  return (
    <div className="min-h-screen md:flex">
      <header className="flex h-12 items-center justify-between border-b px-4 md:hidden">
        <Logo animate />
        {pending ? (
          <TransitionLink href="/approvals" className="ml-auto mr-2 rounded-full bg-warning-soft px-2 py-0.5 text-xs font-medium text-warning">
            {pending} waiting for approval
          </TransitionLink>
        ) : null}
        <Button variant="ghost" size="icon" onClick={() => setOpen((o) => !o)} aria-label={open ? 'Close menu' : 'Open menu'} aria-expanded={open} aria-controls="sidebar">
          {open ? <X className="h-4 w-4" aria-hidden /> : <Menu className="h-4 w-4" aria-hidden />}
        </Button>
      </header>
      <aside
        id="sidebar"
        className={cn(
          'flex-col border-b bg-surface p-3 md:sticky md:top-0 md:flex md:h-screen md:w-56 md:shrink-0 md:border-b-0 md:border-r',
          open ? 'flex' : 'hidden',
        )}
      >
        <div className="mb-4 hidden px-2.5 pt-1 md:block">
          <Logo animate />
        </div>
        <nav ref={nav} aria-label="Main" data-indicator={box ? 'on' : 'off'} className="group/nav relative flex flex-1 flex-col gap-0.5">
          <span
            aria-hidden
            data-testid="nav-indicator"
            className={cn(
              'pointer-events-none absolute inset-x-0 top-0 rounded-md bg-muted before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-accent',
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
        <LegalLinks className="mt-3 px-2.5" />
        <div className="mt-3 flex items-center justify-between gap-2 border-t px-2.5 pt-3">
          <span className="truncate text-xs text-muted-foreground" title={user?.email}>
            {user?.display_name || user?.email}
          </span>
          <Button variant="ghost" size="icon" className="h-7 w-7" onClick={signOut} aria-label="Sign out">
            <LogOut className="h-4 w-4" aria-hidden />
          </Button>
        </div>
      </aside>
      <main className="min-w-0 flex-1">
        <DeletionBanner />
        <EmailBanner />
        <div className="mx-auto w-full max-w-6xl px-4 py-8 md:px-10 md:py-12">
          <PageTransition>{children}</PageTransition>
        </div>
      </main>
    </div>
  );
}

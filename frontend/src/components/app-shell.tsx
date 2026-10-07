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
  X,
} from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useState, type ComponentType, type ReactNode } from 'react';
import { useAuth } from '@/components/auth-provider';
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
];
const NAV_BOTTOM: NavItem[] = [
  { href: '/developer', label: 'Developer', icon: Code2 },
  { href: '/settings', label: 'Settings', icon: Settings },
];

function NavLink({ item, onNavigate }: { item: NavItem; onNavigate: () => void }) {
  const pathname = usePathname();
  const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      onClick={onNavigate}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm',
        active ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
      )}
    >
      <Icon className="h-4 w-4" aria-hidden />
      {item.label}
    </Link>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const { user, logout } = useAuth();
  const router = useRouter();
  const close = () => setOpen(false);

  async function signOut() {
    await logout();
    router.replace('/login');
  }

  return (
    <div className="min-h-screen md:flex">
      <header className="flex h-12 items-center justify-between border-b px-4 md:hidden">
        <span className="text-sm font-semibold">SocialOS</span>
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
        <div className="mb-4 hidden px-2.5 pt-1 text-sm font-semibold md:block">SocialOS</div>
        <nav aria-label="Main" className="flex flex-1 flex-col gap-0.5">
          {NAV.map((i) => (
            <NavLink key={i.href} item={i} onNavigate={close} />
          ))}
          <div className="mt-auto flex flex-col gap-0.5 pt-4">
            {NAV_BOTTOM.map((i) => (
              <NavLink key={i.href} item={i} onNavigate={close} />
            ))}
          </div>
        </nav>
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
        <div className="mx-auto w-full max-w-6xl px-4 py-8 md:px-10 md:py-12">{children}</div>
      </main>
    </div>
  );
}

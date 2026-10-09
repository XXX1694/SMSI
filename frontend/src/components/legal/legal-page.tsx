import Link from 'next/link';
import type { ReactNode } from 'react';
import { LEGAL_EFFECTIVE_DATE, LEGAL_VERSION, type Operator } from '@/lib/legal';

/** Chrome shared by /terms and /privacy: header, template notice, version line and the cross link. */
export function LegalPage({ title, other, operator, children }: { title: string; other: 'terms' | 'privacy'; operator: Operator; children: ReactNode }) {
  return (
    <main id="main" tabIndex={-1} className="mx-auto w-full max-w-2xl px-4 py-10 focus:outline-none">
      <nav aria-label="Legal" className="mb-8 flex flex-wrap items-center justify-between gap-2 text-sm">
        <Link href="/login" className="text-muted-foreground hover:text-foreground">
          Steerpost
        </Link>
        <Link href={`/${other}`} className="text-accent hover:underline">
          {other === 'terms' ? 'Terms of Service' : 'Privacy Policy'}
        </Link>
      </nav>
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Version {LEGAL_VERSION}. Effective {LEGAL_EFFECTIVE_DATE}.
      </p>
      <p role="note" className="mt-4 rounded-md border bg-muted px-3 py-2 text-sm text-muted-foreground">
        This text is a template that ships with Steerpost (formerly SocialOS). It is not legal advice. The operator of this instance is responsible for it and must
        review it before real users sign up.
        {operator.configured ? null : ' The operator has not set their name and contact address yet.'}
      </p>
      <div className="mt-8 space-y-8 text-sm leading-relaxed [&_h2]:mb-2 [&_h2]:text-base [&_h2]:font-semibold [&_li]:mt-1 [&_p+p]:mt-3 [&_ul]:list-disc [&_ul]:pl-5">
        {children}
      </div>
    </main>
  );
}

export function LegalSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h2>{title}</h2>
      {children}
    </section>
  );
}

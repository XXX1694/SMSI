import { Children, type ReactNode } from 'react';

/**
 * The result of `t.rich(...)` as keyed React children (the tag handlers return elements, and an array of elements needs
 * keys): `{nodes(t.rich('agree', { terms: (c) => <Link href="/terms">{c}</Link> }))}`.
 */
export function nodes(parts: (string | ReactNode)[]): ReactNode {
  return Children.toArray(parts);
}

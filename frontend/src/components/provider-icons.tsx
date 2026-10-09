import type { SVGProps } from 'react';

type IconProps = SVGProps<SVGSVGElement>;

/** Single-colour marks (they follow the text colour), so they restyle with the theme and need no brand colours. */
export function GoogleMark(props: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden {...props}>
      <path d="M12.2 10.2v3.9h5.5c-.2 1.3-1.6 3.8-5.5 3.8a6 6 0 1 1 0-12c1.9 0 3.1.8 3.8 1.5l2.6-2.5A9.5 9.5 0 0 0 12.2 2.5a9.5 9.5 0 1 0 0 19c5.5 0 9.1-3.9 9.1-9.3 0-.6-.1-1.1-.2-1.6z" />
    </svg>
  );
}

export function GithubMark(props: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden {...props}>
      <path d="M12 2a10 10 0 0 0-3.2 19.5c.5.1.7-.2.7-.5v-1.8c-2.8.6-3.4-1.2-3.4-1.2-.5-1.2-1.1-1.5-1.1-1.5-.9-.6.1-.6.1-.6 1 .1 1.5 1 1.5 1 .9 1.5 2.4 1.1 2.9.8.1-.7.4-1.1.6-1.4-2.2-.3-4.6-1.1-4.6-5a3.9 3.9 0 0 1 1-2.7 3.6 3.6 0 0 1 .1-2.7s.8-.3 2.8 1a9.5 9.5 0 0 1 5 0c1.9-1.3 2.8-1 2.8-1 .6 1.4.2 2.4.1 2.7a3.9 3.9 0 0 1 1 2.7c0 3.9-2.4 4.7-4.6 5 .4.3.7.9.7 1.8v2.7c0 .3.2.6.7.5A10 10 0 0 0 12 2z" />
    </svg>
  );
}

export function ProviderMark({ id, ...props }: IconProps & { id: string }) {
  if (id === 'google') return <GoogleMark {...props} />;
  if (id === 'github') return <GithubMark {...props} />;
  return null;
}

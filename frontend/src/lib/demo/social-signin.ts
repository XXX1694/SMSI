/**
 * Demo stand-in for "Continue with Google / GitHub" (D-023). A static site has no provider to visit, so `start`
 * answers with the in-app page the real redirect would end on: the sign-up page for a new person. The same checks
 * as the API apply to the last step (Terms accepted, name length).
 */
import type { DemoResponse, DemoState } from './model';
import { safeNext } from '../safe-next';

export const SIGN_IN_PROVIDERS = [
  { id: 'github', name: 'GitHub' },
  { id: 'google', name: 'Google' },
];

const SAMPLE_PEOPLE: Record<string, { email: string; display_name: string }> = {
  github: { email: 'sam.rivera@example.com', display_name: 'Sam Rivera' },
  google: { email: 'sam.rivera@example.com', display_name: 'Sam Rivera' },
};

const fail = (status: number, code: string, message: string, fields?: Record<string, string>): DemoResponse => ({
  status,
  body: { error: { code, message, request_id: 'demo', ...(fields ? { fields } : {}) } },
});

export function startSocialSignIn(s: DemoState, provider: string, next: unknown): DemoResponse {
  const person = SAMPLE_PEOPLE[provider];
  if (!person) return fail(404, 'NOT_FOUND', 'sign-in provider not found');
  s.oauth_pending = { provider, ...person, next: safeNext(typeof next === 'string' ? next : null) ?? '/dashboard' };
  return { status: 200, body: { redirect: '/signup/complete' } };
}

export function pendingSignup(s: DemoState): DemoResponse {
  if (!s.oauth_pending) return fail(404, 'NOT_FOUND', 'no sign-up is waiting');
  return { status: 200, body: s.oauth_pending };
}

/** Creates the account (the demo renames its one user) and signs in. */
export function completeSignup(s: DemoState, body: Record<string, unknown>): DemoResponse {
  const pending = s.oauth_pending;
  if (!pending) return fail(404, 'NOT_FOUND', 'no sign-up is waiting');
  if (body.accept_terms !== true) return fail(400, 'VALIDATION_ERROR', 'You must accept the Terms and the Privacy Policy', { accept_terms: 'must be accepted' });
  const name = typeof body.display_name === 'string' ? body.display_name.trim() : '';
  if (name.length > 100) return fail(400, 'VALIDATION_ERROR', 'display_name too long', { display_name: 'max 100 characters' });
  Object.assign(s.user, { email: pending.email, display_name: name || pending.email });
  s.oauth_pending = null;
  s.signed_in = true;
  return { status: 201, body: undefined };
}

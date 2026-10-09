/** Demo stand-in for account deletion (D-019): the same checks and states, on plain data. Nothing is ever deleted. */
import type { DemoResponse, DemoState } from './model';

const DAY = 86_400_000;
export const DEMO_GRACE_DAYS = 7;

const fail = (status: number, code: string, message: string, fields?: Record<string, string>): DemoResponse => ({
  status,
  body: { error: { code, message, request_id: 'demo', ...(fields ? { fields } : {}) } },
});

/** Asks for deletion: needs the password and the typed email, ends the session, sets scheduled posts back to drafts. */
export function requestDeletion(s: DemoState, body: Record<string, unknown>, nowMs: number): DemoResponse {
  if (s.user.deletion_scheduled_at) return fail(409, 'CONFLICT', 'deletion of this account is already scheduled');
  if (body.password !== s.user.password) return fail(400, 'VALIDATION_ERROR', 'the password is incorrect', { password: 'incorrect' });
  if (String(body.confirm ?? '').trim().toLowerCase() !== s.user.email.toLowerCase()) {
    return fail(400, 'VALIDATION_ERROR', 'type your email address exactly to confirm', { confirm: 'does not match your email' });
  }
  for (const p of s.posts) {
    if (p.status !== 'scheduled') continue;
    p.status = 'draft';
    p.scheduled_at = null;
  }
  const at = new Date(nowMs + DEMO_GRACE_DAYS * DAY).toISOString();
  s.user.deletion_scheduled_at = at;
  s.signed_in = false;
  return { status: 202, body: { status: 'scheduled', scheduled_for: at } };
}

export function cancelDeletion(s: DemoState): DemoResponse {
  if (!s.user.deletion_scheduled_at) return fail(409, 'CONFLICT', 'no deletion is scheduled for this account');
  s.user.deletion_scheduled_at = null;
  return { status: 204, body: undefined };
}

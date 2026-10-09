'use client';
import { useRouter } from 'next/navigation';
import { useRef, useState, type FormEvent } from 'react';
import { centerOf, heroFill } from '@/lib/hero';
import { useAuth } from '@/components/auth-provider';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import type { AppT } from '@/i18n/translate';
import { ApiError } from '@/lib/api';
import { DEMO, DEMO_EMAIL, DEMO_PASSWORD } from '@/lib/demo/config';

export type FieldName = 'email' | 'password' | 'terms';
type Errors = Partial<Record<FieldName, string>>;
const ORDER: FieldName[] = ['email', 'password', 'terms'];
const FIELD_ID: Record<FieldName, string> = { email: 'email', password: 'password', terms: 'accept-terms' };
const looksLikeEmail = (v: string) => /^[^\s@]+@[^\s@]+$/.test(v);

interface Values {
  email: string;
  password: string;
  accepted: boolean;
}

/** The sentence for one field, or undefined when it is fine. The server applies the same rules; this just says so early. */
function check(field: FieldName, v: Values, isLogin: boolean, t: AppT): string | undefined {
  if (field === 'email') return !v.email.trim() ? t('auth.emailRequired') : looksLikeEmail(v.email.trim()) ? undefined : t('auth.emailInvalid');
  if (field === 'password') {
    if (!v.password) return t('auth.passwordRequired');
    return !isLogin && (v.password.length < 8 || v.password.length > 128) ? t('auth.passwordLength') : undefined;
  }
  return isLogin || v.accepted ? undefined : t('auth.acceptTerms');
}

/** The API names the fields it refused (`fields`); the sentence is ours, so it is translated. */
function serverFieldErrors(err: ApiError, t: AppT): Errors {
  const out: Errors = {};
  if (err.fields.email) out.email = t('auth.emailInvalid');
  if (err.fields.password) out.password = t('auth.passwordLength');
  if (err.fields.accept_terms) out.terms = t('auth.acceptTerms');
  return out;
}

/** State, per-field validation and submit of the email form. Nothing is silently disabled: a refused submit says what is missing. */
export function useAuthEmailForm(mode: 'login' | 'register', next: string | null) {
  const t = useTranslations();
  const errorText = useErrorText();
  const { login, register } = useAuth();
  const router = useRouter();
  const isLogin = mode === 'login';
  // The demo has a single built-in user, so its credentials are pre-filled.
  const [email, setEmail] = useState(DEMO && isLogin ? DEMO_EMAIL : '');
  const [password, setPassword] = useState(DEMO && isLogin ? DEMO_PASSWORD : '');
  const [accepted, setAccepted] = useState(false);
  const [errors, setErrors] = useState<Errors>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [duplicate, setDuplicate] = useState(false);
  const [busy, setBusy] = useState(false);
  const form = useRef<HTMLFormElement>(null);
  // A field is checked on blur only once the person has typed in it: tabbing through an empty form is not an error.
  const edited = useRef(new Set<FieldName>());
  const values = { email, password, accepted };

  const setField = (field: FieldName, message: string | undefined) => setErrors((e) => ({ ...e, [field]: message }));
  const edit = (field: FieldName, set: (v: string) => void) => (v: string) => {
    edited.current.add(field);
    set(v);
  };
  const blur = (field: FieldName) => () => {
    if (!edited.current.has(field)) return;
    setDuplicate(false);
    setField(field, check(field, values, isLogin, t));
  };
  const accept = (checked: boolean) => {
    setAccepted(checked);
    if (checked) setField('terms', undefined);
  };

  async function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    setDuplicate(false);
    const found: Errors = {};
    for (const f of ORDER) {
      const message = check(f, values, isLogin, t);
      if (message) found[f] = message;
    }
    setErrors(found);
    const first = ORDER.find((f) => found[f]);
    if (first) {
      setFormError(t('auth.fixFields'));
      form.current?.querySelector<HTMLElement>(`#${FIELD_ID[first]}`)?.focus();
      return;
    }
    setBusy(true);
    try {
      if (isLogin) await login(email.trim(), password);
      else await register(email.trim(), password, '', accepted);
      heroFill(centerOf(form.current?.querySelector('[type="submit"]')), () => router.replace(next ?? '/dashboard'));
    } catch (err) {
      const refused = err instanceof ApiError ? serverFieldErrors(err, t) : {};
      if (err instanceof ApiError && err.status === 409 && !isLogin) setDuplicate(true);
      else if (Object.keys(refused).length > 0) {
        setErrors(refused);
        setFormError(t('auth.fixFields'));
      } else setFormError(errorText(err));
      setBusy(false);
    }
  }

  return { form, email, setEmail: edit('email', setEmail), password, setPassword: edit('password', setPassword), accepted, accept, errors, formError, duplicate, busy, blur, submit, isLogin };
}

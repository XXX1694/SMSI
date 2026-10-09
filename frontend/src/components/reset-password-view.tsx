"use client";
import Link from "next/link";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { AuthShell } from "@/components/auth-shell";
import { RevokeKeysOption } from "@/components/revoke-keys-option";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { useErrorText } from "@/hooks";
import { nodes } from "@/i18n/rich";
import { useTranslations } from "@/i18n/use-translations";
import type { AppT } from "@/i18n/translate";
import { ApiError, api } from "@/lib/api";
import { forgetHashToken, takeHashToken } from "@/lib/hash-token";
import { InlineError } from '@/components/states';

type State = "form" | "success" | "invalid";

function validate(password: string, confirm: string, t: AppT): string | null {
  if (password.length < 8 || password.length > 128) return t("auth.resetPassword.lengthError");
  if (password !== confirm) return t("auth.resetPassword.mismatch");
  return null;
}

export function ResetPasswordView() {
  const t = useTranslations();
  const errorText = useErrorText();
  const [state, setState] = useState<State>("form");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [revokeKeys, setRevokeKeys] = useState(false);
  const [revoked, setRevoked] = useState(false);
  const token = useRef<string | null>(null);

  useEffect(() => {
    token.current = takeHashToken();
    if (!token.current) setState("invalid");
  }, []);

  function dropToken(next: State) {
    forgetHashToken();
    token.current = null;
    setState(next);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const problem = validate(password, confirm, t);
    setError(problem);
    if (problem) return;
    setBusy(true);
    try {
      await api.auth.resetPassword(token.current ?? "", password, revokeKeys);
      setRevoked(revokeKeys);
      dropToken("success");
    } catch (err) {
      // The password length is checked above, so a 400 here means the link is unknown, used or expired.
      if (err instanceof ApiError && err.status === 400) dropToken("invalid");
      else setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  if (state === "success") return <ResetSuccess revoked={revoked} />;
  if (state === "invalid") return <ResetInvalid />;
  return (
    <ResetPasswordForm
      password={password}
      confirm={confirm}
      revokeKeys={revokeKeys}
      error={error}
      busy={busy}
      onPassword={setPassword}
      onConfirm={setConfirm}
      onRevokeKeys={setRevokeKeys}
      onSubmit={submit}
    />
  );
}

function ResetSuccess({ revoked }: { revoked: boolean }) {
  const t = useTranslations("auth.resetPassword");
  const ta = useTranslations("auth");
  return (
    <AuthShell title={t("doneTitle")} description={t("doneBody")}>
      <p className="mb-4 text-sm text-muted-foreground">
        {revoked
          ? t("keysRevoked")
          : nodes(
              t.rich("keysKept", {
                b: (c) => <strong>{c}</strong>,
                link: (c) => (
                  <Link href="/developer" className="text-accent hover:underline">
                    {c}
                  </Link>
                ),
              }),
            )}
      </p>
      <Button asChild className="w-full">
        <Link href="/login">{ta("signIn")}</Link>
      </Button>
    </AuthShell>
  );
}

function ResetInvalid() {
  const t = useTranslations("auth.resetPassword");
  const ta = useTranslations("auth");
  return (
    <AuthShell title={ta("linkInvalidTitle")} description={t("invalidBody")}>
      <Button asChild className="w-full">
        <Link href="/forgot-password">{ta("requestNewLink")}</Link>
      </Button>
    </AuthShell>
  );
}

type FormProps = {
  password: string;
  confirm: string;
  revokeKeys: boolean;
  error: string | null;
  busy: boolean;
  onPassword: (v: string) => void;
  onConfirm: (v: string) => void;
  onRevokeKeys: (v: boolean) => void;
  onSubmit: (e: FormEvent) => void;
};

function ResetPasswordForm(p: FormProps) {
  const t = useTranslations("auth.resetPassword");
  const ta = useTranslations("auth");
  const tc = useTranslations("common");
  return (
    <AuthShell title={t("chooseTitle")} description={t("chooseBody")}>
      <form onSubmit={p.onSubmit} className="space-y-4" noValidate>
        <Field
          label={ta("newPassword")}
          htmlFor="password"
          hint={ta("passwordHint")}
        >
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            required
            value={p.password}
            onChange={(e) => p.onPassword(e.target.value)}
          />
        </Field>
        <Field label={ta("repeatPassword")} htmlFor="confirm">
          <Input
            id="confirm"
            type="password"
            autoComplete="new-password"
            required
            value={p.confirm}
            onChange={(e) => p.onConfirm(e.target.value)}
          />
        </Field>
        <RevokeKeysOption
          id="revoke-keys"
          checked={p.revokeKeys}
          onChange={p.onRevokeKeys}
        />
        {p.error ? (
          <InlineError>{p.error}</InlineError>
        ) : null}
        <Button
          type="submit"
          className="w-full"
          disabled={p.busy || !p.password || !p.confirm}
        >
          {p.busy ? tc("saving") : t("save")}
        </Button>
      </form>
    </AuthShell>
  );
}

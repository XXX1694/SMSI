export type SignInProviderKey = 'google' | 'github' | 'other';

/**
 * The `{provider}` argument of messages that name a provider: an ICU `select` with `google`, `github` and `other`, so
 * the sentence for an unknown or missing provider is written whole, not built from a noun that would not decline.
 */
export function signInProviderKey(id: string | null | undefined): SignInProviderKey {
  const key = id?.toLowerCase();
  return key === 'google' || key === 'github' ? key : 'other';
}

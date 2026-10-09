/** Display names of the sign-in providers the API can name. An id we do not know is not echoed back into a sentence. */
const NAMES: Record<string, string> = { google: 'Google', github: 'GitHub' };

export function signInProviderName(id: string | null | undefined): string | null {
  return (id && NAMES[id.toLowerCase()]) || null;
}

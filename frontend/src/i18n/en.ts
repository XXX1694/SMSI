import errors from '../../messages/en/errors.json';
import { createTranslator } from '@/i18n/translate';
import type { ErrorsT } from '@/lib/errors';

/**
 * An English translator over the error sentences only, for code that runs outside React (the API client builds
 * `ApiError.message` from it). Importing just `errors.json` keeps the rest of the English catalog out of the shared
 * chunk. Components use `useTranslations`, which follows the active locale; tests that need other namespaces use
 * tests/helpers/en-t.ts.
 */
export const enErrorsT: ErrorsT = createTranslator({ locale: 'en', messages: { errors }, onMissing: () => {} });

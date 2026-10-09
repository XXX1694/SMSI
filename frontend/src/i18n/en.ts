import en from '../../messages/en.json';
import { createTranslator, type AppT } from '@/i18n/translate';

/**
 * An English translator for code that runs outside React (the API client builds `ApiError.message` from it, tests use it).
 * Components use `useTranslations`, which follows the active locale.
 */
export const enT: AppT = createTranslator({ locale: 'en', messages: en, onMissing: () => {} });

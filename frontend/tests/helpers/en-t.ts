import en from '@/i18n/en-all';
import { createTranslator, type AppT } from '@/i18n/translate';

/** An English translator over the whole catalog, for lib functions that take a translator. */
export const enT: AppT = createTranslator({ locale: 'en', messages: en, onMissing: () => {} });

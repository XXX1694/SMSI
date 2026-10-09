import type { Catalog } from '@/i18n/pseudo';

/** ar, assembled from messages/ar/*.json. One module per locale, so the build emits one lazy chunk per locale. `npm run i18n:check` keeps this list equal to the files on disk. */
const catalog: Catalog = {};

export default catalog;

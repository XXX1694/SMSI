/**
 * Stable identifiers of the seeded demo data. They live in their own tiny module because
 * `generateStaticParams` needs the post ids at build time and must not pull in the whole mock.
 */
function id(prefix: string, n: number): string {
  return `${prefix}-0000-4000-8000-${n.toString(16).padStart(12, '0')}`;
}

export const SEED_ID = {
  user: id('d0000000', 1),
  account: {
    linkedin: id('a0000000', 1),
    telegram: id('a0000000', 2),
    mock: id('a0000000', 3),
  },
  media: [id('b0000000', 1), id('b0000000', 2), id('b0000000', 3), id('b0000000', 4)] as const,
  apiKey: [id('c0000000', 1), id('c0000000', 2), id('c0000000', 3)] as const,
  mcp: [id('e0000000', 1), id('e0000000', 2), id('e0000000', 3)] as const,
};

/** Post ids in seed order; index `i` is `SEEDED_POST_IDS[i]`. */
export const SEEDED_POST_IDS: readonly string[] = Array.from({ length: 24 }, (_, i) => id('f0000000', i + 1));

export const seedPostId = (i: number): string => {
  const v = SEEDED_POST_IDS[i];
  if (!v) throw new Error(`no seeded post #${i}`);
  return v;
};

export const seedId = id;

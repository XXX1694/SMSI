export interface Manifests {
  appManifest: { pages: Record<string, string[]> };
  buildManifest: { rootMainFiles: string[] };
}
export interface BudgetRow {
  route: string;
  kb: number;
  budgetKb: number;
  over: boolean;
}
export function pageKeyFor(route: string, pages: Record<string, string[]>): string;
export function routeScripts(route: string, manifests: Manifests): string[];
export function gzipBytes(buffer: Buffer): number;
export function toKb(bytes: number): number;
export function compareBudgets(measured: Record<string, number>, budgets: Record<string, number>): BudgetRow[];
export function formatTable(rows: BudgetRow[]): string;
export function filesContaining(contents: Record<string, string>, needle: string): string[];
export function markerProblems(input: {
  marker: string;
  label: string;
  forbidden: Record<string, string>;
  expected: Record<string, string>;
  fix: string;
}): string[];

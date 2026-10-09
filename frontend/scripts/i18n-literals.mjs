/**
 * Finds user-visible English that is written in code instead of the message catalog (D-021). Used by
 * `tests/i18n-literals.test.ts` and `npm run i18n:literals`. It reads TSX/TS with the TypeScript parser:
 *   - JSX text with a letter in it, and `{'string'}` / `{cond ? 'a' : 'b'}` children;
 *   - JSX attributes the user can read or hear: aria-label, title, placeholder, alt, and the copy props of our components
 *     (label, description, hint, note, confirmLabel, dismissLabel, retryLabel);
 *   - in component and lib code, string literals that read like a UI sentence ("Save draft", "Could not load the post.")
 *     or sit in a property that holds UI text (`label`, `title`, `description`, `hint`, `message`, `placeholder`).
 * What it skips: strings with no letters ("·", "/"), identifiers (`posts:read`, `post.publish`), class names, URLs,
 * import paths, type positions, and the allow-list in `scripts/i18n-literals.allow.json`.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import ts from 'typescript';

const ATTRS = new Set([
  'aria-label',
  'aria-description',
  'aria-placeholder',
  'aria-roledescription',
  'aria-valuetext',
  'title',
  'placeholder',
  'alt',
  // Props of our own components that render as visible copy (Field, Dialog, ConfirmDialog, CopyButton, ...).
  'label',
  'description',
  'hint',
  'note',
  'confirmLabel',
  'dismissLabel',
  'retryLabel',
]);
/** Properties whose string value is shown to people when it is written as a plain capitalised string. */
const TEXT_PROPS = new Set(['label', 'title', 'description', 'hint', 'message', 'placeholder', 'action', 'heading', 'caption', 'tooltip', 'summary', 'reason']);
/** Calls whose first string argument is shown to people. */
const TEXT_CALLS = new Set(['toast', 'success', 'error', 'info', 'warning', 'confirm', 'alert']);

const hasLetter = (s) => /\p{L}/u.test(s);
/** "Save draft", "Could not load the post." but not "posts:read", "Content-Type", "GET", "en-GB", "#main". */
const readsLikeCopy = (s) => /^\p{Lu}\p{Ll}+(?:[ ,.'’!?:…-]|$)/u.test(s) && (/\s/.test(s) || /[.!?…]$/.test(s)) && !/^\p{Lu}\p{Ll}+(?:Error|Exception)$/u.test(s);
const capitalisedWord = (s) => /^\p{Lu}\p{Ll}{2,}$/u.test(s);

export function listFiles(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...listFiles(p));
    else if (/\.(tsx?)$/.test(name) && !name.endsWith('.d.ts')) out.push(p);
  }
  return out;
}

function strings(node, out = []) {
  if (!node) return out;
  if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node) || ts.isNonNullExpression(node) || ts.isSatisfiesExpression(node)) return strings(node.expression, out);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) out.push(node.text);
  else if (ts.isTemplateExpression(node)) {
    out.push(node.head.text);
    for (const sp of node.templateSpans) {
      out.push(sp.literal.text);
      strings(sp.expression, out);
    }
  } else if (ts.isConditionalExpression(node)) {
    strings(node.whenTrue, out);
    strings(node.whenFalse, out);
  } else if (ts.isBinaryExpression(node)) {
    const k = node.operatorToken.kind;
    if (k === ts.SyntaxKind.BarBarToken || k === ts.SyntaxKind.QuestionQuestionToken || k === ts.SyntaxKind.PlusToken || k === ts.SyntaxKind.AmpersandAmpersandToken) {
      strings(node.left, out);
      strings(node.right, out);
    }
  }
  return out;
}

/** @returns {{ line: number, kind: string, text: string }[]} */
export function scanSource(text, fileName = 'x.tsx') {
  const sf = ts.createSourceFile(fileName, text, ts.ScriptTarget.Latest, true, fileName.endsWith('x') ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const found = [];
  const add = (node, kind, value) => {
    const { line } = sf.getLineAndCharacterOfPosition(node.getStart(sf));
    found.push({ line: line + 1, kind, text: value.replace(/\s+/g, ' ').trim().slice(0, 80) });
  };
  const inTypePosition = (n) => {
    for (let p = n.parent; p; p = p.parent) {
      if (ts.isTypeNode(p) || ts.isTypeAliasDeclaration(p) || ts.isInterfaceDeclaration(p)) return true;
      if (ts.isStatement(p)) return false;
    }
    return false;
  };
  /** True when the literal is (part of) an attribute value itself, not code inside a handler like onClick={() => ...}. */
  const inJsxAttribute = (n) => {
    for (let p = n.parent; p; p = p.parent) {
      if (ts.isJsxAttribute(p)) return true;
      if (ts.isArrowFunction(p) || ts.isFunctionExpression(p) || ts.isJsxElement(p) || ts.isJsxSelfClosingElement(p) || ts.isStatement(p)) return false;
    }
    return false;
  };

  const visit = (node) => {
    // Static page metadata (`export const metadata`) is rendered on the server and stays English (D-021, Consequences).
    if (ts.isVariableDeclaration(node) && ['metadata', 'viewport'].includes(node.name.getText(sf))) return;
    if (ts.isFunctionDeclaration(node) && node.name?.text === 'generateMetadata') return;
    if (ts.isJsxText(node)) {
      if (hasLetter(node.text)) add(node, 'jsx-text', node.text);
    } else if (ts.isJsxAttribute(node) && ATTRS.has(node.name.getText(sf)) && node.initializer) {
      const init = node.initializer;
      const vals = ts.isStringLiteral(init) ? [init.text] : ts.isJsxExpression(init) ? strings(init.expression) : [];
      for (const v of vals) if (hasLetter(v)) add(node, `attr:${node.name.getText(sf)}`, v);
    } else if (ts.isJsxExpression(node) && node.expression && (ts.isJsxElement(node.parent) || ts.isJsxFragment(node.parent))) {
      for (const v of strings(node.expression)) if (hasLetter(v)) add(node, 'jsx-expr', v);
    } else if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) || ts.isTemplateHead(node)) {
      const parent = node.parent;
      const skip =
        (ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent) || ts.isExternalModuleReference(parent)) ||
        inJsxAttribute(node) || // attributes are handled above (className, href, ... are not copy)
        ts.isLiteralTypeNode(parent) ||
        ts.isCaseClause(parent) ||
        (ts.isElementAccessExpression(parent) && parent.argumentExpression === node) ||
        (ts.isBinaryExpression(parent) && [ts.SyntaxKind.EqualsEqualsEqualsToken, ts.SyntaxKind.ExclamationEqualsEqualsToken, ts.SyntaxKind.InKeyword].includes(parent.operatorToken.kind)) ||
        (ts.isCallExpression(parent) && ['cn', 'cva', 'clsx', 'twMerge', 'require', 'matchMedia', 'addEventListener', 'removeEventListener', 'querySelector', 'querySelectorAll', 'getItem', 'setItem', 'readStorage', 'writeStorage', 'dispatchEvent', 'get', 'has', 'set', 'includes', 'startsWith', 'endsWith', 'replace', 'split', 'join', 'test', 'padStart', 'localeCompare', 'getElementById', 'getAttribute', 'setAttribute', 'push', 'fetch', 'createElement'].includes(parent.expression.getText(sf).split('.').pop())) ||
        inTypePosition(node);
      if (!skip) {
        const v = ts.isTemplateHead(node) ? node.text : node.text;
        const prop = ts.isPropertyAssignment(parent) && parent.initializer === node ? parent.name.getText(sf).replace(/['"]/g, '') : null;
        const callee = ts.isCallExpression(parent) && parent.arguments[0] === node ? parent.expression.getText(sf).split('.').pop() : null;
        if (hasLetter(v)) {
          if (readsLikeCopy(v) || (prop && TEXT_PROPS.has(prop) && capitalisedWord(v)) || (callee && TEXT_CALLS.has(callee) && /^\p{Lu}/u.test(v))) add(node, 'string', v);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return found;
}

/**
 * Scan a source tree.
 * @param {{ root: string, dirs: string[], allow: { files?: Record<string,string>, texts?: Record<string,Record<string,string>> } }} opts
 * `allow.files`: path (relative to root, `/` separators) -> reason; the whole file is skipped.
 * `allow.texts`: path -> { exact string: reason } that may stay.
 */
export function scanTree({ root, dirs, allow }) {
  const results = [];
  for (const d of dirs) {
    const abs = join(root, d);
    for (const f of statSync(abs).isDirectory() ? listFiles(abs) : [abs]) {
      const rel = relative(root, f).split(sep).join('/');
      if (allow.files?.[rel]) continue;
      const okTexts = new Set(Object.keys(allow.texts?.[rel] ?? {}));
      for (const hit of scanSource(readFileSync(f, 'utf8'), f)) {
        if (okTexts.has(hit.text) || okTexts.has(hit.text.replace(/…$/, ''))) continue;
        results.push({ file: rel, ...hit });
      }
    }
  }
  return results;
}

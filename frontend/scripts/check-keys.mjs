#!/usr/bin/env node
// Guards the message keys a screen asks for, and the parameters it fills them with (docs/32).
//
// The dictionaries are typed against each other, so a *missing translation* cannot compile — but
// three things still compile and only break on screen:
//
//   1. a key that names no entry (`titleKey="page.foo"` one suffix short) renders as itself;
//   2. a message with `{{count}}` asked for without `{ count }` renders the placeholder verbatim;
//   3. a translation that drops a placeholder shows `{{count}}` in one language only.
//
// This script checks all three against the German dictionary (the reference) and the English one.
//
// Usage: node scripts/check-keys.mjs [glob-root]   (default: src/app)

import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative } from 'node:path';
import { pathToFileURL } from 'node:url';

const root = process.argv[2] ?? 'src/app';
const scratch = mkdtempSync(join(tmpdir(), 'campus-erp-keys-'));

// The dictionaries are TypeScript with extension-less imports; bundling them once is cheaper and
// more faithful than teaching this script to read them.
async function loadDictionary(language) {
  const bundle = join(scratch, `${language}.mjs`);
  execFileSync(
    'node_modules/.bin/esbuild',
    [
      `src/app/i18n/messages.${language}.ts`,
      '--bundle',
      '--format=esm',
      `--outfile=${bundle}`,
      '--log-level=error',
    ],
    { stdio: 'inherit' },
  );
  const module = await import(pathToFileURL(bundle).href);
  return module[language];
}

const [de, en] = await Promise.all([loadDictionary('de'), loadDictionary('en')]);

/** Every leaf path of a message tree, e.g. `kasse.orders.loading`, with its text. */
function flatten(tree, prefix = '', out = new Map()) {
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === 'object') flatten(value, path, out);
    else out.set(path, String(value));
  }
  return out;
}

const german = flatten(de);
const english = flatten(en);

function placeholders(text) {
  return new Set([...text.matchAll(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g)].map((match) => match[1]));
}

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else if (path.endsWith('.ts') && !path.endsWith('.spec.ts')) out.push(path);
  }
  return out;
}

/** The object literal starting at `source[start]` (a `{`), brace-balanced. */
function balanced(source, start) {
  let depth = 0;
  for (let i = start; i < source.length; i += 1) {
    if (source[i] === '{') depth += 1;
    else if (source[i] === '}') {
      depth -= 1;
      if (depth === 0) return source.slice(start + 1, i);
    }
  }
  return null;
}

/** `{ a: 1, b }` -> `a`, `b`. */
function parameters(block) {
  const names = new Set();
  for (const part of block.split(',')) {
    const name = part.split(':')[0].trim();
    if (/^[A-Za-z0-9_$]+$/.test(name)) names.add(name);
  }
  return names;
}

/** Every key a component asks for, with the parameters it passes and where it asks. */
function usedKeys(source) {
  const found = [];
  const patterns = [
    // `'key' | transloco` and `'key' | transloco: { ref }`
    /'([A-Za-z][A-Za-z0-9_.]*)'\s*\|\s*transloco\s*(?::\s*\{)?/g,
    // `translate('key')` and `translate('key', { ref })`
    /translate\(\s*'([A-Za-z][A-Za-z0-9_.]*)'\s*(?:,\s*\{)?/g,
    // `titleKey="page.foo.title"` — a key handed to `<app-page>` rather than rendered inline
    /(?:titleKey|subtitleKey)="([^"]+)"/g,
  ];
  for (const pattern of patterns) {
    for (const match of source.matchAll(pattern)) {
      const line = source.slice(0, match.index).split('\n').length;
      const braceAt = source.indexOf('{', match.index + match[0].length - 1);
      const passes = match[0].trimEnd().endsWith('{') && braceAt !== -1;
      const block = passes ? balanced(source, braceAt) : null;
      found.push({ key: match[1], line, params: block === null ? null : parameters(block) });
    }
  }
  return found;
}

const unknown = [];
const unfilled = [];
const unused = [];
let checked = 0;
let withParameters = 0;

for (const file of walk(root)) {
  const source = readFileSync(file, 'utf8');
  const where = relative(process.cwd(), file);
  for (const { key, line, params } of usedKeys(source)) {
    // Keys built by concatenation (`'language.' + code`) never reach this point, and a key
    // that is really a bare word is a mistake of a different kind — both are named instead.
    if (!key.includes('.')) continue;
    checked += 1;
    const text = german.get(key);
    if (text === undefined) {
      unknown.push(`${where}:${line}: ${key}`);
      continue;
    }
    const wanted = placeholders(text);
    if (wanted.size === 0 && (params === null || params.size === 0)) continue;
    withParameters += 1;
    const passed = params ?? new Set();
    for (const name of wanted) {
      if (!passed.has(name)) unfilled.push(`${where}:${line}: ${key} wants {{${name}}}`);
    }
    for (const name of passed) {
      if (!wanted.has(name)) unused.push(`${where}:${line}: ${key} has no {{${name}}}`);
    }
  }
}

// A translation that loses a placeholder shows it verbatim in that language.
const drifted = [];
for (const [key, text] of german) {
  const englishText = english.get(key);
  if (englishText === undefined) continue;
  const expected = [...placeholders(text)].sort().join(',');
  const actual = [...placeholders(englishText)].sort().join(',');
  if (expected !== actual) drifted.push(`${key}: de has [${expected}], en has [${actual}]`);
}

const reports = [
  ['Components ask for message keys that the dictionary does not have', unknown],
  ['Messages want parameters the component does not pass', unfilled],
  ['Components pass parameters the message does not use', unused],
  ['Translations that do not carry the same parameters', drifted],
].filter(([, offenders]) => offenders.length > 0);

if (reports.length > 0) {
  for (const [heading, offenders] of reports) {
    console.error(heading + ':');
    for (const offender of offenders) console.error('  ' + offender);
  }
  console.error('\n' + reports.reduce((total, [, offenders]) => total + offenders.length, 0) + ' problem(s).');
  process.exit(1);
}

console.log(
  `key check ok: ${checked} key(s) resolve, ${withParameters} of them take parameters, ` +
    'and both dictionaries carry the same placeholders',
);

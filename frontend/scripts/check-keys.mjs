#!/usr/bin/env node
// Guards that every message key a screen asks for exists (docs/32).
//
// The dictionaries are typed against each other, so a *missing translation* cannot compile —
// but a key that names no entry anywhere still compiles and renders as itself ("page.foo.bar"
// in the middle of the interface). This script resolves each key a component uses against the
// German dictionary, which is the reference, and fails the build when one is not there.
//
// Usage: node scripts/check-keys.mjs [glob-root]   (default: src/app)

import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative } from 'node:path';
import { pathToFileURL } from 'node:url';

const root = process.argv[2] ?? 'src/app';
const scratch = mkdtempSync(join(tmpdir(), 'campus-erp-keys-'));
const bundle = join(scratch, 'messages.de.mjs');

// The dictionary is TypeScript with extension-less imports; bundling it once is cheaper and
// more faithful than teaching this script to read it.
execFileSync(
  'node_modules/.bin/esbuild',
  ['src/app/i18n/messages.de.ts', '--bundle', '--format=esm', `--outfile=${bundle}`, '--log-level=error'],
  { stdio: 'inherit' },
);

const { de } = await import(pathToFileURL(bundle).href);

/** Every leaf path of a message tree, e.g. `kasse.orders.loading`. */
function flatten(tree, prefix = '') {
  const keys = [];
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === 'object') keys.push(...flatten(value, path));
    else keys.push(path);
  }
  return keys;
}

const known = new Set(flatten(de));

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else if (path.endsWith('.ts') && !path.endsWith('.spec.ts')) out.push(path);
  }
  return out;
}

/** Every key a component asks for, with where it asks. */
function usedKeys(source) {
  const found = [];
  const patterns = [
    /'([A-Za-z][A-Za-z0-9_.]*)'\s*\|\s*transloco/g,
    /translate\(\s*'([A-Za-z][A-Za-z0-9_.]*)'/g,
    /(?:titleKey|subtitleKey)="([^"]+)"/g,
  ];
  for (const pattern of patterns) {
    for (const match of source.matchAll(pattern)) {
      const line = source.slice(0, match.index).split('\n').length;
      found.push({ key: match[1], line });
    }
  }
  return found;
}

const offenders = [];
let checked = 0;

for (const file of walk(root)) {
  const source = readFileSync(file, 'utf8');
  for (const { key, line } of usedKeys(source)) {
    // Keys built by concatenation (`'language.' + code`) never reach this point, and a key
    // that is really a bare word is a mistake of a different kind — both are named instead.
    if (!key.includes('.')) continue;
    checked += 1;
    if (!known.has(key)) offenders.push(`${relative(process.cwd(), file)}:${line}: ${key}`);
  }
}

if (offenders.length > 0) {
  console.error('Components ask for message keys that the dictionary does not have:');
  for (const offender of offenders) console.error('  ' + offender);
  console.error(`\n${offenders.length} unknown key(s) of ${checked} checked.`);
  process.exit(1);
}

console.log(`key check ok: ${checked} message key(s) resolve`);

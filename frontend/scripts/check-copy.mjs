#!/usr/bin/env node
// Guards the rule that a screen carries no copy of its own (docs/32).
//
// Every user-visible string in a component template is a message key rendered through
// `| transloco`. A hard-coded word is invisible to the language picker, so it is a defect
// even when it is spelled correctly: this script fails the build when a template contains
// text that is not an interpolation, an element, or a binding.
//
// Usage: node scripts/check-copy.mjs [glob-root]   (default: src/app)

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const root = process.argv[2] ?? 'src/app';

/** Components whose template is pure structure: no words to find. */
const ALLOWED = new Set([
  'app.component.ts',
  'shell.component.ts',
  'page.component.ts',
  'language-picker.component.ts',
]);

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      out.push(...walk(path));
    } else if (path.endsWith('.component.ts') && !path.endsWith('.spec.ts')) {
      out.push(path);
    }
  }
  return out;
}

/** The `template:` literal(s) of a component file, with the line they start on. */
function templates(source, file) {
  const found = [];
  const pattern = /template:\s*`/g;
  let match;
  while ((match = pattern.exec(source)) !== null) {
    const start = match.index + match[0].length;
    const end = source.indexOf('`', start);
    if (end === -1) continue;
    const line = source.slice(0, start).split('\n').length;
    found.push({ body: source.slice(start, end), line, file });
  }
  return found;
}

const offenders = [];

for (const file of walk(root)) {
  const name = file.split('/').pop() ?? '';
  if (ALLOWED.has(name)) continue;

  for (const { body, line } of templates(readFileSync(file, 'utf8'), file)) {
    const lines = body.split('\n');
    lines.forEach((text, index) => {
      // Text between tags: `>Wort<`, ignoring interpolations, entity references and
      // self-closing or attribute-only lines.
      for (const match of text.matchAll(/>([^<>]+)</g)) {
        const candidate = match[1].replace(/\{\{[^}]*\}\}/g, '').trim();
        if (!/[A-Za-zÄÖÜäöüß]/.test(candidate)) continue;
        if (candidate.startsWith('&')) continue;
        // A short all-caps token is an identifier, not copy: SEPA scheme names (CORE, B2B),
        // IBAN/BIC blocks, currency codes. Those are the same in every language.
        if (/^[A-Z0-9]{2,5}$/.test(candidate)) continue;
        offenders.push(`${relative(process.cwd(), file)}:${line + index}: ${candidate}`);
      }
    });
  }
}

if (offenders.length > 0) {
  console.error('Components carry text that is not a message key (docs/32-design-system-and-i18n.md):');
  for (const offender of offenders) console.error('  ' + offender);
  console.error(`\n${offenders.length} hard-coded string(s). Use {{ 'key' | transloco }}.`);
  process.exit(1);
}

console.log('copy check ok: every template string is a message key');

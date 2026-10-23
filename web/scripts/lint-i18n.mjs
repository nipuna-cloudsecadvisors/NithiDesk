#!/usr/bin/env node
// i18n lint (doc 11 §2: "Lint rule in CI fails on literal UI strings").
//
// Fails when:
//   1. a .tsx file has JSX text containing a letter (any script);
//   2. a string literal containing a letter is rendered as a JSX child
//      ({'Save'}) or passed to a user-facing prop (title, placeholder,
//      aria-label, alt, label, ...);
//   3. a static t('key') / tl('key') call names a key missing from en.json;
//   4. en.json and si.json do not have the same keys, or a value is empty.
// A line can opt out with a trailing  // i18n-ignore  comment.
//
// Usage: node scripts/lint-i18n.mjs [--keys]   (--keys prints used keys)

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const src = join(root, 'src');
const LETTER = /\p{L}/u;
const UI_PROPS = new Set(['title', 'placeholder', 'aria-label', 'aria-description', 'alt', 'label', 'aria-valuetext', 'summary']);
const T_FUNCS = new Set(['t', 'tl']);

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.(tsx?|jsx?)$/.test(name) && !/\.test\.tsx?$/.test(name)) out.push(p);
  }
  return out;
}

const problems = [];
const usedKeys = new Map(); // key -> first location
const dynamicPrefixes = new Set();

function report(file, sf, node, msg) {
  const { line } = sf.getLineAndCharacterOfPosition(node.getStart(sf));
  const text = sf.text.split('\n')[line] ?? '';
  if (text.includes('i18n-ignore')) return;
  problems.push(`${relative(root, file)}:${line + 1}: ${msg}`);
}

for (const file of walk(src)) {
  const code = readFileSync(file, 'utf8');
  const isTsx = file.endsWith('.tsx');
  const sf = ts.createSourceFile(file, code, ts.ScriptTarget.Latest, true, isTsx ? ts.ScriptKind.TSX : ts.ScriptKind.TS);

  const visit = (node) => {
    if (isTsx && ts.isJsxText(node)) {
      const text = node.getText(sf).trim();
      if (text && LETTER.test(text)) report(file, sf, node, `JSX text literal: "${text.slice(0, 60)}"`);
    }
    if (isTsx && ts.isJsxExpression(node) && node.expression && node.parent && (ts.isJsxElement(node.parent) || ts.isJsxFragment(node.parent))) {
      const e = node.expression;
      if ((ts.isStringLiteral(e) || ts.isNoSubstitutionTemplateLiteral(e)) && LETTER.test(e.text)) {
        report(file, sf, e, `string literal rendered as text: "${e.text.slice(0, 60)}"`);
      }
    }
    if (isTsx && ts.isJsxAttribute(node)) {
      const name = node.name.getText(sf);
      if (UI_PROPS.has(name) && node.initializer) {
        let lit = null;
        if (ts.isStringLiteral(node.initializer)) lit = node.initializer;
        else if (ts.isJsxExpression(node.initializer) && node.initializer.expression) {
          const e = node.initializer.expression;
          if (ts.isStringLiteral(e) || ts.isNoSubstitutionTemplateLiteral(e)) lit = e;
        }
        if (lit && LETTER.test(lit.text)) report(file, sf, lit, `literal in UI prop ${name}: "${lit.text.slice(0, 60)}"`);
      }
    }
    if (ts.isCallExpression(node)) {
      const callee = node.expression;
      const fname = ts.isIdentifier(callee) ? callee.text : ts.isPropertyAccessExpression(callee) ? callee.name.text : '';
      if (T_FUNCS.has(fname) && node.arguments.length > 0) {
        const a = node.arguments[0];
        if (ts.isStringLiteral(a) || ts.isNoSubstitutionTemplateLiteral(a)) {
          if (!usedKeys.has(a.text)) {
            const { line } = sf.getLineAndCharacterOfPosition(a.getStart(sf));
            usedKeys.set(a.text, `${relative(root, file)}:${line + 1}`);
          }
        } else if (ts.isTemplateExpression(a)) {
          dynamicPrefixes.add(a.head.text);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
}

const load = (name) => JSON.parse(readFileSync(join(src, 'locales', name), 'utf8'));
const en = load('en.json');
const si = load('si.json');

for (const [key, where] of usedKeys) {
  if (!(key in en)) problems.push(`${where}: key "${key}" is not in en.json`);
}
for (const key of Object.keys(en)) {
  if (!(key in si)) problems.push(`si.json: missing key "${key}"`);
  if (typeof en[key] !== 'string' || en[key].trim() === '') problems.push(`en.json: empty value for "${key}"`);
}
for (const key of Object.keys(si)) {
  if (!(key in en)) problems.push(`si.json: key "${key}" is not in en.json`);
  if (typeof si[key] !== 'string' || si[key].trim() === '') problems.push(`si.json: empty value for "${key}"`);
}
for (const p of dynamicPrefixes) {
  if (!Object.keys(en).some((k) => k.startsWith(p))) problems.push(`dynamic key prefix "${p}" has no keys in en.json`);
}

if (process.argv.includes('--keys')) {
  for (const k of [...usedKeys.keys()].sort()) console.log(k);
  console.log('# dynamic prefixes:', [...dynamicPrefixes].sort().join(' '));
}

if (problems.length) {
  console.error(problems.join('\n'));
  console.error(`\ni18n lint: ${problems.length} problem(s).`);
  process.exit(1);
}
console.log(`i18n lint: ok (${usedKeys.size} static keys used, ${Object.keys(en).length} keys in each catalogue).`);

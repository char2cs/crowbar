/**
 * The one shiki highlighter Monaco and Plate share: a grammar loads on first
 * ask and is announced once, and the JavaScript regex engine must tokenize the
 * editor languages exactly as the oniguruma engine does.
 */
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { createHighlighterCore, type HighlighterCore } from 'shiki/core'
import { createOnigurumaEngine } from 'shiki/engine/oniguruma'
import { bundledLanguages, type BundledLanguage } from 'shiki/langs'
import type { StateStack } from 'shiki/textmate'
import {
  grammarFor,
  grammarStateOf,
  onGrammarReady,
  type ShikiGrammar,
} from '@/features/editor/lib/shiki-grammars'

const LOAD = { timeout: 10_000 }

async function readyGrammar(languageId: string): Promise<ShikiGrammar> {
  grammarFor(languageId)
  await vi.waitFor(() => expect(grammarStateOf(languageId)).not.toBe('loading'), LOAD)
  const grammar = grammarFor(languageId)
  if (!grammar) throw new Error(`${languageId} is ${grammarStateOf(languageId)}`)
  return grammar
}

const rootScope = (grammar: ShikiGrammar) => grammar.tokenizeLine('x', null).tokens[0]?.scopes[0]

/** Each line's tokens as text + scopes, threading the rule stack like the editor does. */
function tokenize(grammar: ShikiGrammar, lines: readonly string[]) {
  let stack: StateStack | null = null
  return lines.map((line) => {
    const { tokens, ruleStack } = grammar.tokenizeLine(line, stack)
    stack = ruleStack
    return tokens.map((t) => ({ text: line.slice(t.startIndex, t.endIndex), scopes: t.scopes }))
  })
}

it('loads a grammar on first ask and announces it', async () => {
  const announced = new Promise<string>((resolve) => {
    const off = onGrammarReady((languageId) => {
      off()
      resolve(languageId)
    })
  })

  expect(grammarFor('typescript')).toBeNull()
  expect(grammarStateOf('typescript')).toBe('loading')
  expect(await announced).toBe('typescript')

  const line = 'function probe() {}'
  const tokens = grammarFor('typescript')?.tokenizeLine(line, null).tokens ?? []
  const probe = tokens.find((t) => line.slice(t.startIndex, t.endIndex) === 'probe')
  expect(probe?.scopes.at(-1)).toMatch(/^entity\.name\.function/)
}, 30_000)

it('maps crowbar ids and shiki aliases', async () => {
  expect(rootScope(await readyGrammar('typescriptreact'))).toBe('source.tsx')
  expect(rootScope(await readyGrammar('sh'))).toBe('source.shell')
}, 30_000)

it('stays failed for a language shiki has no grammar for', async () => {
  expect(grammarFor('gitignore')).toBeNull()
  await vi.waitFor(() => expect(grammarStateOf('gitignore')).toBe('failed'), LOAD)

  expect(grammarFor('gitignore')).toBeNull()
  // Asking again did not restart the load.
  expect(grammarStateOf('gitignore')).toBe('failed')
}, 30_000)

// Fenced code stays untagged: markdown embeds a fence's grammar only once it is loaded.
const FIXTURES: Record<string, string[]> = {
  typescript: [
    "import { readFile } from 'node:fs/promises'",
    'export interface Probe<T extends object = {}> { readonly id: number; value?: T }',
    'type Pick2<T, K extends keyof T> = { [P in K]: T[P] }',
    'const RE = /^(?<name>[a-z_]\\w*)\\s*=\\s*([\'"])(.*?)\\2$/giu',
    'export async function load(path: string): Promise<Probe<{ ok: true }>[]> {',
    "  const text = await readFile(path, 'utf8') // trailing comment",
    "  return text.split('\\n').map((line, i) => ({ id: i, value: { ok: true } }))",
    '}',
    '@sealed() class Box<T> extends Base implements Probe<T> { #secret = 0n; static of = <U,>(u: U) => u }',
    'const s = `tpl ${1 + 2} and ${`nested ${RE.source}`}` as const satisfies string',
  ],
  tsx: [
    "import React, { useState } from 'react'",
    'type Props = { title: string; items: readonly string[] }',
    'export function List({ title, items }: Props): JSX.Element {',
    '  const [open, setOpen] = useState<boolean>(false)',
    '  return (',
    '    <section className="list" data-open={open} onClick={() => setOpen((o) => !o)}>',
    '      <h2>{title}</h2>{items.map((item) => <Item key={item} label={`#${item}`} />)}',
    '    </section>',
    '  )',
    '}',
  ],
  javascript: [
    "'use strict'",
    "const { join } = require('path')",
    '/** @param {string} a @returns {number} */',
    'function parse(a, ...rest) { return Number.parseInt(a, 16) ?? rest.length }',
    'class Counter extends EventTarget { static #n = 0; get value() { return Counter.#n++ } }',
    'const re = /\\d+(?:\\.\\d*)?e[+-]?\\d+/g, big = 0x1F_FFn',
    "export default async function* gen() { yield* [1, 2, 3]; await import('./x.js') }",
    "outer: for (const k in obj) { if (k?.startsWith?.('_')) continue outer }",
    "const html = String.raw`<p>${join('a', 'b')}</p>`",
    "/* block */ console.log(typeof parse === 'function' && !!re.test('1e5'))",
  ],
  python: [
    'from __future__ import annotations',
    'import re, typing as t',
    '@dataclass(frozen=True)',
    'class Point(Base, metaclass=Meta):',
    '    """Docstring with \'quotes\'."""',
    '    x: int = 0',
    '    def dist(self, other: Point | None = None) -> float:',
    "        return (self.x ** 2 + 0x1F + 1_000j) if other else f\"{self.x!r:>10} {re.sub(r'\\d+', '', 'a1')}\"",
    'async def main(*args, **kw): await asyncio.gather(*[lambda: None for _ in range(3)])',
    "print(b'bytes', rb'raw\\n', len([1, 2]), sep='\\t')  # comment",
  ],
  go: [
    'package main',
    '',
    'import ("fmt"; "strings")',
    '',
    'type Rect[T ~int | ~float64] struct{ W, H T `json:"w,omitempty"` }',
    'func (r *Rect[T]) Area() float64 { return float64(r.W * r.H) } // area',
    'const raw = `C:\\path\\n` + "esc\\t\\"q\\"" + string(\'\\x41\')',
    'func main() {',
    '\tch := make(chan map[string][]int, 0x10); go func() { ch <- nil }()',
    '\tdefer fmt.Println(strings.ToUpper("done"), 1e-3, 0o17, \'r\', <-ch)',
  ],
  rust: [
    'use std::collections::HashMap;',
    '#[derive(Debug, Clone)]',
    "pub struct Cache<'a, T: Clone + 'a> { items: HashMap<&'a str, T>, hits: u64 }",
    "impl<'a, T: Clone> Cache<'a, T> {",
    "    pub fn get(&mut self, key: &'a str) -> Option<&T> { self.hits += 1; self.items.get(key) }",
    '}',
    'macro_rules! sq { ($x:expr) => { $x * $x }; }',
    'fn main() -> Result<(), Box<dyn std::error::Error>> {',
    '    let s = r#"raw "string""#; let b = b\'x\'; let n = 1_000u32 as f64; println!("{s} {:?}", sq!(n)); Ok(())',
    '}',
  ],
  json: [
    '{',
    '  "name": "probe",',
    '  "version": "1.0.0",',
    '  "private": true,',
    '  "count": -1.5e+10,',
    '  "nothing": null,',
    '  "escaped": "line\\nbreak \\"quoted\\" \\u00e9",',
    '  "list": [1, "two", { "three": [false] }],',
    '  "nested": { "deep": { "deeper": {} } }',
    '}',
  ],
  yaml: [
    '# config',
    'version: "3.9"',
    'services:',
    '  web: &web',
    '    image: nginx:1.25',
    '    ports: ["80:80", 443]',
    '    environment: { DEBUG: true, LEVEL: ~, RATIO: 0.5 }',
    '    command: >-',
    "      run --flag 'quoted value'",
    '  worker: { <<: *web, replicas: !!int "2" }',
  ],
  markdown: [
    '# Heading one',
    'Some *emphasis*, **strong**, `inline code` and a [link](https://example.com "title").',
    '> A blockquote with ~~strike~~ text',
    '- item one',
    '- [ ] task item',
    '1. ordered **bold** item ![image](./a.png)',
    '```',
    'plain fence',
    '```',
    'Setext heading\\',
  ],
  shellscript: [
    '#!/usr/bin/env bash',
    'set -euo pipefail',
    'readonly DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"',
    'for f in "$DIR"/*.txt; do [[ -f $f ]] && echo "${f##*/}" | tr \'a-z\' \'A-Z\'; done',
    'cat <<EOF > out.log',
    'value: ${HOME:-/tmp} $((1 + 2))',
    'EOF',
    'function greet() { local name=${1:?missing}; printf \'%s\\n\' "hi $name"; }',
    'case "$1" in start|run) greet "$@" ;; *) exit 1 ;; esac',
    'arr=(one two); echo ${#arr[@]} 2>/dev/null || true  # comment',
  ],
  css: [
    '@import url("base.css") screen;',
    ':root { --accent: #0af; --pad: calc(1rem + 2px); }',
    '.card > a:hover::after, #id[data-x~="y"] { color: rgb(0 128 255 / 50%); }',
    '@media (min-width: 640px) and (prefers-color-scheme: dark) {',
    '  .card { padding: var(--pad) !important; transition: opacity .2s ease-in-out; }',
    '}',
    '@keyframes spin { from { transform: rotate(0deg) } to { transform: rotate(360deg) } }',
    '/* comment */ .grid { grid-template-columns: repeat(auto-fill, minmax(12em, 1fr)); }',
    '@font-face { font-family: "Inter"; src: url(inter.woff2) format("woff2"); }',
    'li:nth-child(2n + 1):not(.done) { content: "\\201C"; margin: -0.5em auto 0 }',
  ],
  html: [
    '<!DOCTYPE html>',
    '<html lang="en">',
    '<head><meta charset="utf-8"><title>Probe &amp; test</title>',
    '<style>body { margin: 0; color: #333 }</style></head>',
    '<body class="main" data-id=\'1\'>',
    '  <!-- comment -->',
    '  <a href="https://example.com" target=_blank>link</a><input disabled>',
    '  <script type="module">const x = 1; document.title = `n ${x}`</script>',
    '</body>',
    '</html>',
  ],
}

describe('tokenizes the editor languages like the oniguruma engine', { timeout: 30_000 }, () => {
  let reference: HighlighterCore

  beforeAll(async () => {
    reference = await createHighlighterCore({
      engine: createOnigurumaEngine(import('shiki/wasm')),
      langs: [],
    })
  }, 30_000)

  it.each(Object.entries(FIXTURES))('%s', async (languageId, lines) => {
    const ours = await readyGrammar(languageId)
    await reference.loadLanguage(bundledLanguages[languageId as BundledLanguage])

    expect(tokenize(ours, lines)).toEqual(tokenize(reference.getLanguage(languageId), lines))
  })
})

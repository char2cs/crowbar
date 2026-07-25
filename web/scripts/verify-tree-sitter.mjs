#!/usr/bin/env node
/**
 * verify-tree-sitter.mjs
 *
 * Proves the vendored tree-sitter assets actually work, rather than merely
 * existing. For every parser folder this checks, in order:
 *
 *   1. parser.wasm starts with the WebAssembly magic bytes (\0asm). A previous
 *      bug in this project served SPA HTML in place of wasm, which silently
 *      disabled highlighting app-wide — presence alone proves nothing.
 *   2. The grammar loads under the installed web-tree-sitter runtime, which
 *      catches ABI drift between the compiled grammar and the runtime.
 *   3. A trivial snippet parses.
 *   4. highlights.scm COMPILES against that grammar. This is the check that
 *      matters most: a query naming a node type the grammar does not define
 *      throws at Query construction and disables highlighting for that
 *      language, while the wasm itself still looks perfectly healthy.
 *
 * Exits non-zero if any language fails. Run via `bun run verify:tree-sitter`.
 */

import { readFileSync, existsSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { Language, Parser, Query } from 'web-tree-sitter'

const __dirname = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = resolve(__dirname, '..')
const PARSERS_DIR = join(WEB_ROOT, 'public', 'tree-sitter', 'parsers')

// A snippet per language that should parse without a top-level ERROR node.
// Kept deliberately trivial: this validates the grammar is wired up, not that
// it implements the whole language.
const SNIPPETS = {
  bash: 'echo hi\n',
  c: 'int main(void) { return 0; }\n',
  c_sharp: 'class A { void M() {} }\n',
  cpp: 'int main() { return 0; }\n',
  css: 'a { color: red; }\n',
  dart: 'void main() {}\n',
  diff: '--- a\n+++ b\n@@ -1 +1 @@\n-x\n+y\n',
  dockerfile: 'FROM alpine\nRUN echo hi\n',
  elisp: '(defun f (x) x)\n',
  elixir: 'defmodule A do\nend\n',
  elm: 'module Main exposing (..)\n',
  go: 'package main\n\nfunc main() {}\n',
  graphql: '{ user { id } }\n',
  html: '<!doctype html><p>hi</p>\n',
  java: 'class A { void m() {} }\n',
  json: '{"a": 1}\n',
  kotlin: 'fun main() {}\n',
  lua: 'local x = 1\n',
  markdown: '# Title\n\ntext\n',
  nix: '{ a = 1; }\n',
  ocaml: 'let x = 1\n',
  php: '<?php echo 1;\n',
  protobuf: 'syntax = "proto3";\nmessage A { int32 a = 1; }\n',
  python: 'def f():\n    return 1\n',
  ql: 'from int x select x\n',
  ruby: 'def f\n  1\nend\n',
  rust: 'fn main() { let x = 1; }\n',
  scala: 'object A { def m = 1 }\n',
  solidity: 'contract A { uint x; }\n',
  sql: 'SELECT 1;\n',
  svelte: '<script>let a = 1</script>\n<p>{a}</p>\n',
  swift: 'let x = 1\n',
  terraform: 'resource "a" "b" {}\n',
  toml: 'a = 1\n',
  tsx: 'const a = <div />\n',
  typescript: 'const a: number = 1\n',
  vue: '<template><p>hi</p></template>\n',
  xml: '<?xml version="1.0"?><a><b/></a>\n',
  yaml: 'a: 1\n',
  zig: 'pub fn main() void {}\n',
}

const WASM_MAGIC = Buffer.from([0x00, 0x61, 0x73, 0x6d])

function checkMagic(file) {
  const head = Buffer.alloc(4)
  const fd = readFileSync(file)
  fd.copy(head, 0, 0, 4)
  return head.equals(WASM_MAGIC)
}

async function main() {
  await Parser.init()

  const langs = Object.keys(SNIPPETS).sort()
  const rows = []
  let failed = 0

  for (const lang of langs) {
    const dir = join(PARSERS_DIR, lang)
    const wasmPath = join(dir, 'parser.wasm')
    const scmPath = join(dir, 'highlights.scm')

    const row = {
      lang,
      wasm: '-',
      magic: '-',
      load: '-',
      parse: '-',
      scm: '-',
      query: '-',
      patterns: '',
      note: '',
    }
    rows.push(row)

    if (!existsSync(wasmPath)) {
      row.wasm = 'MISSING'
      row.note = 'parser.wasm absent'
      failed++
      continue
    }
    row.wasm = 'ok'

    if (!checkMagic(wasmPath)) {
      row.magic = 'BAD'
      row.note = 'not a wasm binary (HTML error page?)'
      failed++
      continue
    }
    row.magic = 'ok'

    let language
    try {
      language = await Language.load(readFileSync(wasmPath))
      row.load = 'ok'
    } catch (err) {
      row.load = 'FAIL'
      row.note = `load: ${err.message}`.slice(0, 90)
      failed++
      continue
    }

    try {
      const parser = new Parser()
      parser.setLanguage(language)
      const tree = parser.parse(SNIPPETS[lang])
      // hasError covers the whole tree; a grammar that loads but mis-parses
      // everything shows up here rather than silently passing.
      row.parse = tree.rootNode.hasError ? 'ERROR' : 'ok'
      if (tree.rootNode.hasError) {
        row.note = `snippet produced ERROR node: ${tree.rootNode.toString().slice(0, 60)}`
        failed++
      }
      tree.delete()
      parser.delete()
    } catch (err) {
      row.parse = 'FAIL'
      row.note = `parse: ${err.message}`.slice(0, 90)
      failed++
      continue
    }

    if (!existsSync(scmPath)) {
      row.scm = 'MISSING'
      row.note = row.note || 'highlights.scm absent'
      failed++
      continue
    }
    row.scm = 'ok'

    const source = readFileSync(scmPath, 'utf8')
    try {
      const query = new Query(language, source)
      row.query = 'ok'
      const count =
        typeof query.patternCount === 'function' ? query.patternCount() : query.patternCount
      row.patterns = String(count ?? '')
      query.delete?.()
    } catch (err) {
      row.query = 'FAIL'
      row.note = `query: ${err.message}`.replace(/\s+/g, ' ').slice(0, 90)
      failed++
    }
  }

  const pad = (s, n) => String(s).padEnd(n)
  console.log(
    `\n${pad('language', 12)}${pad('wasm', 8)}${pad('magic', 7)}${pad('load', 6)}${pad('parse', 7)}${pad('scm', 6)}${pad('query', 7)}${pad('pat', 5)}note`,
  )
  console.log('-'.repeat(110))
  for (const r of rows) {
    console.log(
      `${pad(r.lang, 12)}${pad(r.wasm, 8)}${pad(r.magic, 7)}${pad(r.load, 6)}${pad(r.parse, 7)}${pad(r.scm, 6)}${pad(r.query, 7)}${pad(r.patterns, 5)}${r.note}`,
    )
  }
  console.log('-'.repeat(110))
  console.log(`${rows.length - failed}/${rows.length} languages fully verified\n`)

  if (failed > 0) {
    console.error(`[verify-tree-sitter] FAILED: ${failed} language(s) broken`)
    process.exit(1)
  }
}

main()

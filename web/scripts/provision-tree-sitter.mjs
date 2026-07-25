#!/usr/bin/env node
/**
 * provision-tree-sitter.mjs
 *
 * Prepares web/public/tree-sitter/ so the Vite dev server and build can serve
 * everything under /tree-sitter/*. Runs from predev / prebuild.
 *
 * ── RUNTIME WASM (web-tree-sitter.wasm) ───────────────────────────────────
 * Source: node_modules/web-tree-sitter/web-tree-sitter.wasm
 *
 * web-tree-sitter requests its runtime via locateFile() using the script name
 * "web-tree-sitter.wasm" — NOT "tree-sitter.wasm". Serving the wrong name gives
 * a 404 → MIME error → syntax highlighting disabled app-wide. Copying straight
 * from node_modules keeps the name and version correct automatically. It is
 * also copied as "tree-sitter.wasm" for any legacy path using the old name.
 *
 * This file is generated, so it stays gitignored.
 *
 * ── PARSER GRAMMARS (parsers/<lang>/parser.wasm + highlights.scm) ─────────
 * These are VENDORED — committed to the repository, not produced here.
 *
 * They are built from pinned upstream grammar repositories by the maintenance
 * script `refresh-tree-sitter-grammars.mjs`, which records the exact upstream
 * commit for each grammar in `tree-sitter-grammars.json`. Licences are listed
 * in NOTICE.md. Nothing about the build depends on a network connection, a
 * toolchain, or any checkout outside this repository.
 *
 * So this script only VERIFIES the grammars, and does so by content rather than
 * by presence: a parser.wasm that is secretly an HTML error page passes an
 * existence check and then silently disables highlighting. For the full check
 * (grammar loads, query compiles) run `bun run verify:tree-sitter`.
 */

import {
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  readSync,
  closeSync,
  statSync,
  cpSync,
} from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = resolve(__dirname, '..')
const PUBLIC_TREE_SITTER = join(WEB_ROOT, 'public', 'tree-sitter')
const PARSERS_DIR = join(PUBLIC_TREE_SITTER, 'parsers')
const MANIFEST_PATH = join(__dirname, 'tree-sitter-grammars.json')

const NODE_MODULES_WEB_TREE_SITTER = join(
  WEB_ROOT,
  'node_modules',
  'web-tree-sitter',
  'web-tree-sitter.wasm',
)

const WASM_MAGIC = Buffer.from([0x00, 0x61, 0x73, 0x6d])

/** True when the file begins with the WebAssembly magic bytes `\0asm`. */
function isWasmBinary(file) {
  let fd
  try {
    fd = openSync(file, 'r')
    const head = Buffer.alloc(4)
    const read = readSync(fd, head, 0, 4, 0)
    return read === 4 && head.equals(WASM_MAGIC)
  } catch {
    return false
  } finally {
    if (fd !== undefined) closeSync(fd)
  }
}

function sameContent(a, b) {
  try {
    if (statSync(a).size !== statSync(b).size) return false
    return readFileSync(a).equals(readFileSync(b))
  } catch {
    return false
  }
}

function copyIfDifferent(src, dest) {
  if (!existsSync(src)) return 'missing'
  if (existsSync(dest) && sameContent(src, dest)) return 'skip'
  mkdirSync(dirname(dest), { recursive: true })
  cpSync(src, dest)
  return 'copied'
}

function main() {
  mkdirSync(PUBLIC_TREE_SITTER, { recursive: true })

  // 1. Runtime wasm, from node_modules.
  const runtimeDest = join(PUBLIC_TREE_SITTER, 'web-tree-sitter.wasm')
  const runtimeResult = copyIfDifferent(NODE_MODULES_WEB_TREE_SITTER, runtimeDest)
  if (runtimeResult === 'missing') {
    console.error(
      `[provision-tree-sitter] ERROR: runtime wasm not found at ${NODE_MODULES_WEB_TREE_SITTER}\n` +
        '  Run `bun install` to restore node_modules.',
    )
    process.exit(1)
  }
  const legacyResult = copyIfDifferent(
    NODE_MODULES_WEB_TREE_SITTER,
    join(PUBLIC_TREE_SITTER, 'tree-sitter.wasm'),
  )
  const copied = [runtimeResult, legacyResult].filter((r) => r === 'copied').length
  if (copied > 0) {
    console.log(`[provision-tree-sitter] Copied runtime wasm (${copied} file(s)) from node_modules`)
  }

  // 2. Verify the vendored grammars.
  const manifest = JSON.parse(readFileSync(MANIFEST_PATH, 'utf8'))
  const expected = Object.keys(manifest.grammars)

  const problems = []
  for (const lang of expected) {
    const wasm = join(PARSERS_DIR, lang, 'parser.wasm')
    const query = join(PARSERS_DIR, lang, 'highlights.scm')

    if (!existsSync(wasm)) problems.push(`${lang}: parser.wasm missing`)
    else if (!isWasmBinary(wasm)) problems.push(`${lang}: parser.wasm is not a wasm binary`)

    if (!existsSync(query)) problems.push(`${lang}: highlights.scm missing`)
    else if (statSync(query).size === 0) problems.push(`${lang}: highlights.scm is empty`)
  }

  if (problems.length > 0) {
    console.error(
      `[provision-tree-sitter] ERROR: vendored tree-sitter grammars are not usable:\n` +
        problems.map((p) => `  - ${p}`).join('\n') +
        '\n\n  These files are committed to the repository under web/public/tree-sitter/parsers/.\n' +
        '  If they are missing or corrupt, restore them with:\n' +
        '    git checkout -- web/public/tree-sitter/parsers\n' +
        '  To rebuild them from upstream instead:\n' +
        '    node scripts/refresh-tree-sitter-grammars.mjs && node scripts/verify-tree-sitter.mjs',
    )
    process.exit(1)
  }

  console.log(`[provision-tree-sitter] Verified ${expected.length} vendored grammars — OK`)
}

main()

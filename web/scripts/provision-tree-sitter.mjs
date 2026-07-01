#!/usr/bin/env node
/**
 * provision-tree-sitter.mjs
 *
 * Copies tree-sitter assets into web/public/tree-sitter/ so the Vite dev
 * server and build can serve them under /tree-sitter/*.
 *
 * ── RUNTIME WASM (web-tree-sitter.wasm) ───────────────────────────────────
 * Source: node_modules/web-tree-sitter/web-tree-sitter.wasm
 *
 * web-tree-sitter@0.26.9 requests its runtime via locateFile() as the script
 * name "web-tree-sitter.wasm" — NOT "tree-sitter.wasm". Serving the wrong
 * name causes a 404 → MIME error → all syntax highlighting disabled app-wide.
 * We copy directly from node_modules so the name and version are always
 * correct regardless of what other projects do.
 *
 * We also copy it as "tree-sitter.wasm" for any legacy code path that may
 * still reference that name (harmless extra byte-for-byte copy).
 *
 * ── PARSER GRAMMARS (parsers/<lang>/parser.wasm + highlights.scm) ─────────
 * Source: ~/Projects/Cloned/athas/public/tree-sitter/parsers/
 *
 * The per-language grammar wasms still come from the sibling athas clone.
 * Both projects use web-tree-sitter@0.26.9 and athas parsers were built with
 * tree-sitter-cli@0.26.x, making them binary-compatible.
 *
 * ⚠ KNOWN GAP: The athas clone must exist on the developer's machine and have
 * tree-sitter assets built (`bun install` in that project). This is a local-
 * dev–only workaround. The long-term fix is to add per-language grammar
 * packages as devDependencies and run `tree-sitter build --wasm` in this
 * script (tracked as a separate hardening task).
 *
 * Run automatically via pnpm predev / pnpm prebuild.
 * Idempotent: skips files that already exist and match byte-for-byte.
 */

import { cpSync, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { homedir } from 'node:os'

const __dirname = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = resolve(__dirname, '..')
const PUBLIC_TREE_SITTER = join(WEB_ROOT, 'public', 'tree-sitter')

// Runtime wasm: always from node_modules (version-correct, always present)
const NODE_MODULES_WEB_TREE_SITTER = join(
  WEB_ROOT,
  'node_modules',
  'web-tree-sitter',
  'web-tree-sitter.wasm',
)

// Grammar wasms: sibling athas project (local-dev workaround; see header comment)
const ATHAS_TREE_SITTER = join(homedir(), 'Projects', 'Cloned', 'athas', 'public', 'tree-sitter')

// Languages whose parser.wasm lives in a different folder than the languageId
// (mirrors PARSER_FOLDER_BY_LANGUAGE in extension-assets.ts)
const PARSER_FOLDER_BY_LANGUAGE = {
  angular: 'html',
  less: 'css',
  sass: 'css',
  scss: 'css',
  javascript: 'tsx',
  javascriptreact: 'tsx',
  typescript: 'tsx',
  typescriptreact: 'tsx',
  mdx: 'markdown',
  csharp: 'c_sharp',
  scheme: 'elisp',
  dotenv: 'bash',
}

// Full set of distinct parser folder names needed by crowbar
const PARSER_FOLDERS = [
  ...new Set([
    // Direct languageId -> folder mappings (the folders used in /tree-sitter/parsers/<folder>/)
    'tsx', // typescript, typescriptreact, javascript, javascriptreact
    'typescript',
    'go',
    'json',
    'markdown', // mdx
    'css', // less, sass, scss
    'html', // angular
    'bash', // dotenv, sh, zsh
    'python',
    'rust',
    'java',
    'c',
    'cpp',
    'c_sharp', // csharp
    'ruby',
    'php',
    'xml',
    'diff',
    'yaml',
    'toml',
    'swift',
    'kotlin',
    'scala',
    'lua',
    'nix',
    'dart',
    'elixir',
    'elisp', // scheme
    'ocaml',
    'sql',
    'solidity',
    'terraform',
    'zig',
    'vue',
    'svelte',
    'graphql',
    'dockerfile',
    'elm',
    'protobuf',
    'ql',
  ]),
]

function sameContent(a, b) {
  try {
    const sa = statSync(a)
    const sb = statSync(b)
    if (sa.size !== sb.size) return false
    // For large wasm files compare first+last 512 bytes only (fast check)
    const ba = readFileSync(a)
    const bb = readFileSync(b)
    return ba.equals(bb)
  } catch {
    return false
  }
}

function copyIfDifferent(src, dest) {
  if (!existsSync(src)) return { status: 'missing', src }
  if (existsSync(dest) && sameContent(src, dest)) return { status: 'skip', dest }
  mkdirSync(dirname(dest), { recursive: true })
  cpSync(src, dest)
  return { status: 'copied', dest }
}

function main() {
  mkdirSync(PUBLIC_TREE_SITTER, { recursive: true })

  let copied = 0
  let skipped = 0
  let missing = 0

  // 1. Copy the runtime wasm from node_modules (correct name: web-tree-sitter.wasm)
  //    web-tree-sitter@0.26.9 requests the runtime by the script name via locateFile(),
  //    which resolves to "web-tree-sitter.wasm" — not "tree-sitter.wasm".
  const runtimeNodeSrc = NODE_MODULES_WEB_TREE_SITTER
  const runtimeNodeDest = join(PUBLIC_TREE_SITTER, 'web-tree-sitter.wasm')
  const runtimeNodeResult = copyIfDifferent(runtimeNodeSrc, runtimeNodeDest)
  if (runtimeNodeResult.status === 'copied') {
    console.log('[provision-tree-sitter] Copied web-tree-sitter.wasm (from node_modules)')
    copied++
  } else if (runtimeNodeResult.status === 'skip') {
    skipped++
  } else {
    console.error(
      `[provision-tree-sitter] ERROR: runtime wasm not found at ${runtimeNodeSrc}\n` +
        '  Run `pnpm install` to restore node_modules.',
    )
    process.exit(1)
  }

  // Also copy as tree-sitter.wasm for any legacy code path that uses that name
  const runtimeLegacyDest = join(PUBLIC_TREE_SITTER, 'tree-sitter.wasm')
  const runtimeLegacyResult = copyIfDifferent(runtimeNodeSrc, runtimeLegacyDest)
  if (runtimeLegacyResult.status === 'copied') {
    console.log('[provision-tree-sitter] Copied tree-sitter.wasm (legacy alias)')
    copied++
  } else if (runtimeLegacyResult.status === 'skip') {
    skipped++
  }

  // 2. Copy each language parser (from athas — see header comment re: known gap)
  if (!existsSync(ATHAS_TREE_SITTER)) {
    console.warn(
      `[provision-tree-sitter] WARNING: Grammar source not found at ${ATHAS_TREE_SITTER}\n` +
        '  The athas project must be cloned at ~/Projects/Cloned/athas with tree-sitter\n' +
        '  assets already built (run `bun install` in that project first).\n' +
        '  Skipping grammar parsers — syntax highlighting will be disabled.',
    )
    console.log(
      `[provision-tree-sitter] Done — ${copied} copied, ${skipped} up-to-date, ${missing} missing (no parsers)`,
    )
    return
  }
  for (const lang of PARSER_FOLDERS) {
    const srcDir = join(ATHAS_TREE_SITTER, 'parsers', lang)
    const destDir = join(PUBLIC_TREE_SITTER, 'parsers', lang)

    for (const file of ['parser.wasm', 'highlights.scm']) {
      const src = join(srcDir, file)
      const dest = join(destDir, file)
      const result = copyIfDifferent(src, dest)
      if (result.status === 'copied') {
        copied++
      } else if (result.status === 'skip') {
        skipped++
      } else {
        // highlights.scm is optional — some parsers may not have one
        if (file === 'parser.wasm') {
          console.warn(`[provision-tree-sitter] WARNING: missing ${lang}/parser.wasm`)
          missing++
        }
      }
    }
  }

  console.log(
    `[provision-tree-sitter] Done — ${copied} copied, ${skipped} up-to-date, ${missing} missing`,
  )

  if (missing > 0) {
    process.exit(1)
  }
}

main()

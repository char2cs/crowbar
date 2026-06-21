#!/usr/bin/env node
/**
 * provision-tree-sitter.mjs
 *
 * Copies pre-built tree-sitter parser assets (parser.wasm + highlights.scm)
 * from the sibling `athas` project into web/public/tree-sitter/ so the Vite
 * dev server and build can serve them under /tree-sitter/*.
 *
 * Source: ~/Projects/Cloned/athas/public/tree-sitter/
 *   - tree-sitter.wasm          (web-tree-sitter runtime, 0.26.9-compatible)
 *   - parsers/<lang>/parser.wasm
 *   - parsers/<lang>/highlights.scm
 *
 * Both projects use web-tree-sitter@0.26.9 and the athas parsers were built
 * with tree-sitter-cli@0.26.x, making them binary-compatible.
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

// Canonical source: sibling athas project (same machine, same web-tree-sitter version)
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
    'tsx',       // typescript, typescriptreact, javascript, javascriptreact
    'typescript',
    'go',
    'json',
    'markdown',  // mdx
    'css',       // less, sass, scss
    'html',      // angular
    'bash',      // dotenv, sh, zsh
    'python',
    'rust',
    'java',
    'c',
    'cpp',
    'c_sharp',   // csharp
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
    'elisp',     // scheme
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
  if (!existsSync(ATHAS_TREE_SITTER)) {
    console.error(
      `[provision-tree-sitter] ERROR: Source not found at ${ATHAS_TREE_SITTER}\n` +
        '  The athas project must be cloned at ~/Projects/Cloned/athas with tree-sitter\n' +
        '  assets already built (run `bun install` in that project first).',
    )
    process.exit(1)
  }

  mkdirSync(PUBLIC_TREE_SITTER, { recursive: true })

  let copied = 0
  let skipped = 0
  let missing = 0

  // 1. Copy the runtime wasm
  const runtimeSrc = join(ATHAS_TREE_SITTER, 'tree-sitter.wasm')
  const runtimeDest = join(PUBLIC_TREE_SITTER, 'tree-sitter.wasm')
  const runtimeResult = copyIfDifferent(runtimeSrc, runtimeDest)
  if (runtimeResult.status === 'copied') {
    console.log('[provision-tree-sitter] Copied tree-sitter.wasm')
    copied++
  } else if (runtimeResult.status === 'skip') {
    skipped++
  } else {
    console.warn(`[provision-tree-sitter] WARNING: runtime wasm missing at ${runtimeSrc}`)
    missing++
  }

  // 2. Copy each language parser
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

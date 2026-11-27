#!/usr/bin/env node

import { readdirSync, statSync, existsSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ASSETS_DIR = resolve(__dirname, '..', 'dist', 'assets')

const WORKER_CHUNK = /worker[^/]*\.js$/i

const MIN_BYTES = 64

if (!existsSync(ASSETS_DIR)) {
  console.error(`verify-worker-bundles: no build output at ${ASSETS_DIR}`)
  process.exit(1)
}

const chunks = readdirSync(ASSETS_DIR).filter((f) => WORKER_CHUNK.test(f))

if (chunks.length === 0) {
  console.error('verify-worker-bundles: no worker chunks emitted at all — expected several')
  process.exit(1)
}

const empty = chunks
  .map((f) => ({ file: f, bytes: statSync(join(ASSETS_DIR, f)).size }))
  .filter((c) => c.bytes < MIN_BYTES)

if (empty.length > 0) {
  console.error('verify-worker-bundles: worker chunk(s) emitted with no code:')
  for (const c of empty) console.error(`  ${c.file} — ${c.bytes} bytes`)
  console.error(
    '\nThis is almost always tree-shaking: a worker entry that only imports\n' +
      'another module for its side effects, where that module comes from a\n' +
      'package declaring "sideEffects": false. Name the real module as the\n' +
      "worker entry (import it with Vite's `?worker` suffix) instead.",
  )
  process.exit(1)
}

console.log(`verify-worker-bundles: ${chunks.length} worker chunks, all non-empty`)

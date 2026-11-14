#!/usr/bin/env node
/**
 * verify-worker-bundles.mjs
 *
 * Proves every emitted Web Worker chunk actually contains code.
 *
 * This exists because of a bug that reached a packaged build: the review
 * surface's Shiki worker was a one-line local module whose only statement was
 * `import '@pierre/diffs/worker/worker.js'` — a side-effect-only import. That
 * package declares `"sideEffects": ["dist/components/web-components.js"]`, so
 * every other file in it, worker.js included, is advertised as side-effect
 * free and the production bundler was entitled to drop the import. It did. The
 * emitted worker chunk was 0 BYTES.
 *
 * Nothing failed loudly, which is the point of this check. An empty worker
 * script loads with a 200, installs no message handler, and simply never
 * answers the pool — so every highlight request stayed pending forever and the
 * branch review rendered no diff at all, with no console error. Dev was
 * unaffected the whole time, because Vite serves that module unbundled and runs
 * the import, so the failure was invisible until an installed build.
 *
 * A worker chunk is never legitimately empty, which makes "non-empty" a cheap
 * and exact invariant. Runs as `postbuild`, so every build gates on it.
 */

import { readdirSync, statSync, existsSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ASSETS_DIR = resolve(__dirname, '..', 'dist', 'assets')

// Vite names a worker chunk after its entry module, so the emitted file keeps
// "worker" in its name. Matching on that is what lets this check cover workers
// added later without anyone remembering to register them here.
const WORKER_CHUNK = /worker[^/]*\.js$/i

// Big enough to catch "the module was shaken away" and far below any real
// worker: the smallest one this app ships is ~1KB of Monaco glue.
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

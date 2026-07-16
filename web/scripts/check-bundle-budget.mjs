// web/scripts/check-bundle-budget.mjs — fails if the entry chunk exceeds budget.
import { readFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
// Measured 481.12 KB gzip post-purge (task-25, 2026-07-14, fresh `bunx vite
// build` on enhancement/performance HEAD) — 530_000 is ~10% headroom over
// that. Ratchet floor, not a target: tighten as further chunking work lands,
// don't loosen it to make a regression pass.
const BUDGET_GZIP_BYTES = 530_000
const html = readFileSync('dist/index.html', 'utf8')
const entry = html.match(/src="\/assets\/(index-[^"]+\.js)"/)?.[1]
if (!entry) {
  console.error('entry chunk not found in dist/index.html')
  process.exit(1)
}
const gz = gzipSync(readFileSync(`dist/assets/${entry}`)).length
const monaco = readFileSync(`dist/assets/${entry}`, 'utf8').includes('MonacoEnvironment')
if (monaco) {
  console.error('FAIL: Monaco is back in the entry chunk')
  process.exit(1)
}
if (gz > BUDGET_GZIP_BYTES) {
  console.error(`FAIL: entry ${gz}B gzip > budget ${BUDGET_GZIP_BYTES}B`)
  process.exit(1)
}
console.log(`entry ${entry}: ${gz}B gzip (budget ${BUDGET_GZIP_BYTES}) — OK`)

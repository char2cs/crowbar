// web/scripts/check-bundle-budget.mjs — fails if the boot bundle exceeds budget.
//
// The boot bundle is what the app downloads and parses before first paint: the
// entry chunk plus the `_shell` route chunk (the app frame every route renders
// in), and everything either one imports STATICALLY. Measuring the entry alone
// let heavy code slip into boot through `_shell` unnoticed.
import { readdirSync, readFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'

// Ratchet floors, not targets: tighten as chunking work lands, never loosen them
// to make a regression pass. The entry budget covers the entry's static closure
// (see below); it used to cover the entry file alone, which Rollup can grow or
// shrink just by regrouping modules the entry statically imports anyway.
// Measured 2026-09-24: entry closure 229.1 KB (was 233.1 KB before the sidebar
// stores stopped importing the agent chat stream), boot closure 517.0 KB.
const ENTRY_BUDGET_GZIP_BYTES = 235_000
const BOOT_BUDGET_GZIP_BYTES = 570_000

const ASSETS = 'dist/assets'
const html = readFileSync('dist/index.html', 'utf8')
const entry = html.match(/src="\/assets\/(index-[^"]+\.js)"/)?.[1]
if (!entry) {
  console.error('entry chunk not found in dist/index.html')
  process.exit(1)
}
const shell = readdirSync(ASSETS).filter((f) => /^_shell-[^/]+\.js$/.test(f))

// Static imports only: `import ... from "./x.js"`, `export ... from "./x.js"`,
// `import "./x.js"`. Dynamic `import("./x.js")` is lazy by definition.
const STATIC_IMPORT = /(?:\bfrom\s*|\bimport\s*)["']\.\/([^"']+\.js)["']/g

function closure(roots) {
  const seen = new Set()
  const queue = [...roots]
  while (queue.length > 0) {
    const file = queue.pop()
    if (seen.has(file)) continue
    seen.add(file)
    const src = readFileSync(`${ASSETS}/${file}`, 'utf8')
    for (const match of src.matchAll(STATIC_IMPORT)) queue.push(match[1])
  }
  return seen
}

const boot = closure([entry, ...shell])
let gz = 0
const monaco = []
for (const file of boot) {
  const src = readFileSync(`${ASSETS}/${file}`)
  gz += gzipSync(src).length
  if (src.includes('MonacoEnvironment')) monaco.push(file)
}

let failed = false
if (monaco.length > 0) {
  console.error(`FAIL: Monaco is in the boot bundle (${monaco.join(', ')})`)
  failed = true
}
// The entry's own static closure: what must load before main.tsx runs at all.
// Measured as a closure, not the single file, because Rollup is free to move a
// module between the entry and a chunk the entry statically imports — both are
// on the critical path, and only the sum is a load cost.
let entryGz = 0
for (const file of closure([entry])) entryGz += gzipSync(readFileSync(`${ASSETS}/${file}`)).length
if (entryGz > ENTRY_BUDGET_GZIP_BYTES) {
  console.error(`FAIL: entry closure ${entryGz}B gzip > budget ${ENTRY_BUDGET_GZIP_BYTES}B`)
  failed = true
}
if (gz > BOOT_BUDGET_GZIP_BYTES) {
  console.error(`FAIL: boot bundle ${gz}B gzip > budget ${BOOT_BUDGET_GZIP_BYTES}B`)
  failed = true
}
console.log(
  `entry ${entry} + static imports: ${entryGz}B gzip (budget ${ENTRY_BUDGET_GZIP_BYTES}); ` +
    `boot bundle: ${boot.size} chunks, ${gz}B gzip (budget ${BOOT_BUDGET_GZIP_BYTES})`,
)
process.exit(failed ? 1 : 0)

// web/scripts/check-bundle-budget.mjs — fails if the boot bundle exceeds budget.
//
// The boot bundle is what the app downloads and parses before first paint: the
// entry chunk plus the `_shell` route chunk (the app frame every route renders
// in), and everything either one imports STATICALLY. Measuring the entry alone
// let heavy code slip into boot through `_shell` unnoticed.
import { readdirSync, readFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'

// Ratchet floors, not targets: tighten as chunking work lands, never loosen them
// to make a regression pass. Measured 2026-09-24 on stab/deps: entry 105.5 KB,
// boot closure 1675 KB — nearly all of it Monaco, still reached statically
// through `_shell` (editor-pane → language-contributions). Once Monaco is
// lazy, re-measure and cut BOOT_BUDGET to the new size plus ~10%.
const ENTRY_BUDGET_GZIP_BYTES = 112_000
const BOOT_BUDGET_GZIP_BYTES = 1_725_000

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
const entryGz = gzipSync(readFileSync(`${ASSETS}/${entry}`)).length
if (entryGz > ENTRY_BUDGET_GZIP_BYTES) {
  console.error(`FAIL: entry ${entryGz}B gzip > budget ${ENTRY_BUDGET_GZIP_BYTES}B`)
  failed = true
}
if (gz > BOOT_BUDGET_GZIP_BYTES) {
  console.error(`FAIL: boot bundle ${gz}B gzip > budget ${BOOT_BUDGET_GZIP_BYTES}B`)
  failed = true
}
console.log(
  `entry ${entry}: ${entryGz}B gzip (budget ${ENTRY_BUDGET_GZIP_BYTES}); ` +
    `boot bundle: ${boot.size} chunks, ${gz}B gzip (budget ${BOOT_BUDGET_GZIP_BYTES})`,
)
process.exit(failed ? 1 : 0)

import { readdirSync, readFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'

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

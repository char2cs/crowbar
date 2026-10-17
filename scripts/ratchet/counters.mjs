// Totals, not per-file counts, so moving code between files does not churn the baseline.
// The regexes are approximate by design: a `catch` inside a string literal would count.
import { realpathSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { countLines, loadCode, runGate } from './lib.mjs'

export const COUNTERS = [
  { id: 'go.timers', lang: 'go', re: /\btime\.(?:After|NewTimer|NewTicker|AfterFunc|Tick)\(/g },
  // Single space after `_` on purpose: gofmt-aligned `_        = T{}` is a compile-time assertion, not a discard.
  { id: 'go.discarded-errors', lang: 'go', re: /^[ \t]*_(?:, _)* = /gm },
  { id: 'web.timers', lang: 'web', re: /\b(?:setTimeout|setInterval)\b/g },
  // Whitespace and comments alone make a body empty, so `// ignore` does not hide a swallow.
  { id: 'web.empty-catch', lang: 'web', re: /\bcatch\b\s*(?:\([^)]*\))?\s*\{(?:\s|\/\/[^\n]*|\/\*[\s\S]*?\*\/)*\}/g },
  {
    id: 'web.swallowed-promise',
    lang: 'web',
    re: /\.catch\(\s*(?:\([^)]*\)|[\w$]+)\s*=>\s*(?:\{(?:\s|\/\/[^\n]*|\/\*[\s\S]*?\*\/)*\}|undefined|null|void 0)\s*\)/g,
  },
]

/** Every match of `counter` in the non-test files of its language. */
function* scan(files, { lang, re }) {
  for (const file of files) {
    if (file.test || file.lang !== lang) continue
    for (const match of file.text.matchAll(re)) yield { file, index: match.index }
  }
}

/** id -> match count over non-test files of the counter's language; every id is present. */
export function measureCounters(files) {
  const out = {}
  for (const counter of COUNTERS) out[counter.id] = [...scan(files, counter)].length
  return out
}

/** `path:line` of every match of counter `id`, for finding what to remove. */
export function listMatches(files, id) {
  const counter = COUNTERS.find((c) => c.id === id)
  return [...scan(files, counter)].map(({ file, index }) => `${file.path}:${countLines(file.text.slice(0, index)) + 1}`)
}

const dir = dirname(fileURLToPath(import.meta.url))

export function main(args, { files, baselinePath = join(dir, 'counters-baseline.json'), log = console.log } = {}) {
  const listAt = args.indexOf('--list')
  if (listAt !== -1) {
    const id = args[listAt + 1]
    if (!COUNTERS.some((c) => c.id === id)) {
      log(`unknown counter ${id ?? '(none given)'}; valid ids: ${COUNTERS.map((c) => c.id).join(', ')}`)
      return 1
    }
    for (const hit of listMatches(files ?? loadCode(), id)) log(hit)
    return 0
  }
  const measured = measureCounters(files ?? loadCode())
  return runGate({
    name: 'counters',
    baselinePath,
    measured,
    init: args.includes('--init'),
    update: args.includes('--update'),
    allowAdded: true,
    summary: Object.entries(measured).map(([id, n]) => `${id} ${n}`).join(', '),
    hint: 'Remove the pattern, or handle the case it hides. Find the matches: node scripts/ratchet/counters.mjs --list <id>',
    log,
  })
}

// realpath: node resolves the module URL through symlinks but not argv[1]; a mismatch would skip main and exit 0.
if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) process.exit(main(process.argv.slice(2)))

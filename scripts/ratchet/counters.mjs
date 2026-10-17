// Totals, not per-file counts, so moving code between files does not churn the baseline.
// The regexes are approximate by design: a `catch` inside a string literal would count.
import { realpathSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { loadCode, runGate } from './lib.mjs'

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

/** id -> match count over non-test files of the counter's language; every id is present. */
export function measureCounters(files) {
  const out = {}
  for (const { id, lang, re } of COUNTERS) {
    let n = 0
    for (const file of files) {
      if (file.test || file.lang !== lang) continue
      n += file.text.match(re)?.length ?? 0
    }
    out[id] = n
  }
  return out
}

export function main(args) {
  const measured = measureCounters(loadCode())
  return runGate({
    name: 'counters',
    baselinePath: join(dirname(fileURLToPath(import.meta.url)), 'counters-baseline.json'),
    measured,
    update: args.includes('--update'),
    allowAdded: true,
    summary: Object.entries(measured).map(([id, n]) => `${id} ${n}`).join(', '),
    hint: 'Remove the pattern, or handle the case it hides.',
  })
}

// realpath: node resolves the module URL through symlinks but not argv[1]; a mismatch would skip main and exit 0.
if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) process.exit(main(process.argv.slice(2)))

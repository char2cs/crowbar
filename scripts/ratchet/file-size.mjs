// Per-file baseline, not a count: a count lets one file grow while another shrinks.
// A new file over the cap fails because it is never in the baseline.
import { realpathSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { classify, countLines, loadCode, runGate } from './lib.mjs'

export const MAX_LINES = 500

/** path -> lines for every file, source and test alike, strictly over `max`. */
export function measureOversize(files, max = MAX_LINES) {
  const out = {}
  for (const { path, text } of files) {
    const lines = countLines(text)
    if (lines > max) out[path] = lines
  }
  return out
}

function summarize(measured) {
  const n = { go: { src: 0, tests: 0 }, web: { src: 0, tests: 0 } }
  for (const path of Object.keys(measured)) {
    const { lang, test } = classify(path)
    n[lang][test ? 'tests' : 'src']++
  }
  return `${Object.keys(measured).length} oversize files baselined: go src ${n.go.src}, go tests ${n.go.tests}, web src ${n.web.src}, web tests ${n.web.tests}`
}

export function main(args) {
  const measured = measureOversize(loadCode())
  return runGate({
    name: 'file-size',
    baselinePath: join(dirname(fileURLToPath(import.meta.url)), 'file-size-baseline.json'),
    measured,
    update: args.includes('--update'),
    allowAdded: false,
    summary: summarize(measured),
    hint: `Split the file below ${MAX_LINES} lines. Renamed an oversize file? Change its key in the baseline, value unchanged.`,
  })
}

// realpath: node resolves the module URL through symlinks but not argv[1]; a mismatch would skip main and exit 0.
if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) process.exit(main(process.argv.slice(2)))

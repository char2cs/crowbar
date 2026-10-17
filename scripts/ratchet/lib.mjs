// Shared driver for the ratchet gates: measure, diff against a committed baseline, and
// only ever let that baseline shrink.
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

const UPDATE_CMD = 'make ratchet ARGS=--update'
const INIT_CMD = 'make ratchet ARGS=--init'

/** Number of `\n` characters, like `wc -l`. Walks the text instead of splitting it: files are large. */
export function countLines(text) {
  let n = 0
  for (let i = text.indexOf('\n'); i !== -1; i = text.indexOf('\n', i + 1)) n++
  return n
}

/** @returns {{ lang: 'go' | 'web', test: boolean } | null} null for anything the gates do not measure. */
export function classify(path) {
  // Vendored and generated code is not ours to shrink; golangci and eslint exclude the same paths.
  if (path.startsWith('api/third_party/') || path === 'web/src/routeTree.gen.ts') return null
  if (path.startsWith('api/') && path.endsWith('.go')) return { lang: 'go', test: path.endsWith('_test.go') }
  if (path.startsWith('web/src/') && (path.endsWith('.ts') || path.endsWith('.tsx')))
    return { lang: 'web', test: path.startsWith('web/src/__tests__/') }
  return null
}

/**
 * @typedef {{ path: string, lang: 'go' | 'web', test: boolean, text: string }} CodeFile
 * @returns {CodeFile[]} every tracked or untracked-but-not-ignored measured file that is still on disk.
 */
export function loadCode(root = REPO_ROOT) {
  const listed = execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', 'api', 'web/src'], {
    cwd: root,
    encoding: 'utf8',
    maxBuffer: 1 << 28,
  })
  const files = []
  for (const path of new Set(listed.split('\0'))) {
    const kind = classify(path)
    // A staged deletion is still listed by --cached but has nothing left to measure.
    if (!kind || !existsSync(join(root, path))) continue
    files.push({ path, ...kind, text: readFileSync(join(root, path), 'utf8') })
  }
  return files
}

/**
 * @typedef {{ key: string, was?: number, now?: number }} Change
 * @returns {{ grew: Change[], added: Change[], shrunk: Change[], gone: Change[] }}
 */
export function diffBaseline(baseline, measured) {
  const out = { grew: [], added: [], shrunk: [], gone: [] }
  for (const key of Object.keys(measured).sort()) {
    const now = measured[key]
    if (!Object.hasOwn(baseline, key)) out.added.push({ key, now })
    else if (now > baseline[key]) out.grew.push({ key, was: baseline[key], now })
    else if (now < baseline[key]) out.shrunk.push({ key, was: baseline[key], now })
  }
  for (const key of Object.keys(baseline).sort()) {
    if (!Object.hasOwn(measured, key)) out.gone.push({ key, was: baseline[key] })
  }
  return out
}

const render = {
  grew: (c) => `  grew   ${c.key}: ${c.was} -> ${c.now}`,
  added: (c) => `  new    ${c.key}: ${c.now}`,
  shrunk: (c) => `  shrank ${c.key}: ${c.was} -> ${c.now}`,
  gone: (c) => `  gone   ${c.key}: was ${c.was}`,
}

const writeBaseline = (path, measured) => {
  const sorted = Object.fromEntries(Object.keys(measured).sort().map((k) => [k, measured[k]]))
  writeFileSync(path, JSON.stringify(sorted, null, 2) + '\n')
}

/**
 * `init` is the only way to create a baseline, so a lost or misnamed file can never be
 * silently re-recorded at today's numbers.
 * @returns {number} the process exit code.
 */
export function runGate({ name, baselinePath, measured, init, update, allowAdded, summary, hint, log = console.log }) {
  const exists = existsSync(baselinePath)
  if (init && exists) {
    log(`${name}: baseline already exists at ${baselinePath}; --init only creates a missing one`)
    return 1
  }
  if (init) {
    writeBaseline(baselinePath, measured)
    log(`${name}: baseline created (${Object.keys(measured).length} keys)`)
    return 0
  }
  if (!exists) {
    log(`${name}: no baseline at ${baselinePath}; run: ${INIT_CMD}`)
    return 1
  }

  const diff = diffBaseline(JSON.parse(readFileSync(baselinePath, 'utf8')), measured)
  const growth = [...diff.grew, ...(allowAdded ? [] : diff.added)]
  const writeBack = [...diff.shrunk, ...diff.gone, ...(allowAdded ? diff.added : [])]
  const emit = (kinds) => {
    for (const kind of kinds) for (const change of diff[kind]) log(render[kind](change))
  }

  if (update) {
    if (growth.length > 0) {
      emit(['grew', ...(allowAdded ? [] : ['added'])])
      log(`${name}: refusing to raise a baseline`)
      return 1
    }
    if (writeBack.length === 0) {
      log(`${name}: already current`)
      return 0
    }
    emit(['shrunk', 'gone', 'added'])
    writeBaseline(baselinePath, measured)
    log(`${name}: baseline updated (${Object.keys(measured).length} keys)`)
    return 0
  }

  if (growth.length === 0 && writeBack.length === 0) {
    log(`${name}: ok (${summary})`)
    return 0
  }
  emit(['grew', 'added', 'shrunk', 'gone'])
  if (growth.length > 0) log(['Baselines never go up: delete or split code.', hint].filter(Boolean).join(' '))
  // `update` refuses a run with growth, so the write-back is only advice once the growth is gone.
  else log(`Write the shrink back: ${UPDATE_CMD}, then commit the baseline.`)
  return 1
}

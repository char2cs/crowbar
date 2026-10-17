// Modules map path prefixes to line budgets. To move code between modules, edit both budgets by
// hand in the same commit and never raise `<lang>.total`; `--update` only lowers.
import { readFileSync, realpathSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { countLines, loadCode, runGate } from './lib.mjs'

class ConfigError extends Error {}

const ROOTS = { go: 'api', web: 'web/src' }
// The test totals are the only keys `--update` may add; a new module budget is edited in by hand.
const FIRST_TIME_KEYS = ['go.tests', 'web.tests']

const dir = dirname(fileURLToPath(import.meta.url))
const readModules = () => JSON.parse(readFileSync(join(dir, 'modules.json'), 'utf8'))

const owns = (prefix, path) => path === prefix || path.startsWith(prefix + '/')

/** The module whose longest matching prefix claims `path`, or null. */
function ownerOf(path, byName) {
  let owner = null
  let longest = -1
  for (const [name, prefixes] of Object.entries(byName)) {
    for (const prefix of prefixes) {
      if (prefix.length > longest && owns(prefix, path)) {
        owner = name
        longest = prefix.length
      }
    }
  }
  return owner
}

/** Every way modules.json can silently misattribute lines, reported together. */
function validateModules(modules) {
  const problems = []
  for (const [lang, byName] of Object.entries(modules)) {
    const root = ROOTS[lang]
    const claimedBy = new Map()
    if (Object.hasOwn(byName, 'total')) problems.push(`${lang}: a module is named "total"`)
    for (const [name, prefixes] of Object.entries(byName)) {
      for (const prefix of prefixes) {
        claimedBy.set(prefix, [...(claimedBy.get(prefix) ?? []), name])
        if (prefix.endsWith('/')) problems.push(`${lang}.${name}: "${prefix}" ends with "/"`)
        if (!owns(root, prefix)) problems.push(`${lang}.${name}: "${prefix}" is outside ${root}/`)
      }
    }
    for (const [prefix, names] of claimedBy) {
      const distinct = [...new Set(names)]
      if (distinct.length > 1) problems.push(`${lang}: "${prefix}" is in ${distinct.join(' and ')}`)
    }
  }
  if (problems.length > 0) throw new ConfigError(['invalid scripts/ratchet/modules.json:', ...problems.map((p) => `  ${p}`)].join('\n'))
}

/**
 * @param {import('./lib.mjs').CodeFile[]} files
 * @param {{ go: Record<string, string[]>, web: Record<string, string[]> }} modules
 * @returns {Record<string, number>} `<lang>.<module>` and `<lang>.total` -> source lines; `<lang>.tests` -> test lines.
 */
export function measureLoc(files, modules) {
  validateModules(modules)
  const out = {}
  for (const [lang, byName] of Object.entries(modules)) {
    for (const name of Object.keys(byName)) out[`${lang}.${name}`] = 0
    out[`${lang}.total`] = 0
    out[`${lang}.tests`] = 0
  }
  const unassigned = []
  for (const file of files) {
    if (file.test) {
      out[`${file.lang}.tests`] += countLines(file.text)
      continue
    }
    const owner = ownerOf(file.path, modules[file.lang] ?? {})
    if (owner === null) {
      unassigned.push(file.path)
      continue
    }
    const lines = countLines(file.text)
    out[`${file.lang}.${owner}`] += lines
    out[`${file.lang}.total`] += lines
  }
  if (unassigned.length > 0) {
    throw new ConfigError(
      ['unassigned source files (add them to scripts/ratchet/modules.json):', ...unassigned.sort().map((p) => `  ${p}`)].join('\n'),
    )
  }
  return out
}

export function main(
  args,
  { files = loadCode(), modules = readModules(), baselinePath = join(dir, 'loc-budgets-baseline.json'), log = console.log } = {},
) {
  let measured
  try {
    measured = measureLoc(files, modules)
  } catch (err) {
    if (!(err instanceof ConfigError)) throw err
    log(err.message)
    return 1
  }
  return runGate({
    name: 'loc-budgets',
    baselinePath,
    measured,
    init: args.includes('--init'),
    update: args.includes('--update'),
    allowAdded: FIRST_TIME_KEYS,
    summary: `go ${measured['go.total']}, web ${measured['web.total']}`,
    log,
  })
}

// realpath: node resolves the module URL through symlinks but not argv[1]; a mismatch would skip main and exit 0.
if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) process.exit(main(process.argv.slice(2)))

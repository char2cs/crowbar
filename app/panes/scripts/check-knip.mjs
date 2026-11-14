// web/scripts/check-knip.mjs — dead-code gate over knip (full + --production).
//
// knip.json's `ignoreIssues` / `ignoreDependencies` are the baseline of
// findings that predate the gate. This script runs knip WITHOUT them and
// fails on (a) any finding the baseline does not list and (b) any baseline
// entry that no longer has a finding, so the baseline can only shrink.
// Fix a finding by deleting the dead code, then drop its baseline entry.
import { execFileSync } from 'node:child_process'
import { readFileSync, rmSync, writeFileSync } from 'node:fs'

const config = JSON.parse(readFileSync('knip.json', 'utf8'))
const baselineIssues = config.ignoreIssues ?? {}
const baselineDeps = new Set(config.ignoreDependencies ?? [])

const stripped = { ...config, ignoreIssues: {}, ignoreDependencies: [] }
const tmp = '.knip-check.tmp.json'
writeFileSync(tmp, JSON.stringify(stripped))

const DEP_TYPES = new Set([
  'dependencies',
  'devDependencies',
  'optionalPeerDependencies',
  'binaries',
])

function run(extra) {
  let out
  try {
    out = execFileSync('knip', ['--config', tmp, '--reporter', 'json', ...extra], {
      encoding: 'utf8',
      maxBuffer: 64 * 1024 * 1024,
    })
  } catch (err) {
    // knip exits 1 when it has findings; the JSON is still on stdout.
    if (!err.stdout) throw err
    out = err.stdout
  }
  return JSON.parse(out).issues
}

const found = new Map() // "file\0type" -> names
const foundDeps = new Set()
try {
  for (const issues of [run([]), run(['--production'])]) {
    for (const issue of issues) {
      for (const [type, items] of Object.entries(issue)) {
        if (type === 'file' || !Array.isArray(items) || items.length === 0) continue
        if (DEP_TYPES.has(type)) {
          for (const item of items) foundDeps.add(item.name)
          continue
        }
        const key = `${issue.file}\0${type}`
        const names = found.get(key) ?? new Set()
        for (const item of items) names.add(item.name)
        found.set(key, names)
      }
    }
  }
} finally {
  rmSync(tmp, { force: true })
}

const problems = []
for (const [key, names] of found) {
  const [file, type] = key.split('\0')
  if (!(baselineIssues[file] ?? []).includes(type)) {
    problems.push(`new ${type} in ${file}: ${[...names].join(', ')}`)
  }
}
for (const dep of foundDeps) {
  if (!baselineDeps.has(dep)) problems.push(`unused dependency: ${dep}`)
}
for (const [file, types] of Object.entries(baselineIssues)) {
  for (const type of types) {
    if (!found.has(`${file}\0${type}`)) {
      problems.push(`stale baseline: remove "${type}" for ${file} from knip.json ignoreIssues`)
    }
  }
}
for (const dep of baselineDeps) {
  if (!foundDeps.has(dep)) {
    problems.push(`stale baseline: remove ${dep} from knip.json ignoreDependencies`)
  }
}

if (problems.length > 0) {
  console.error(problems.join('\n'))
  process.exit(1)
}
const baselined = Object.values(baselineIssues).reduce((n, t) => n + t.length, 0)
console.log(`knip: clean (${baselined} file/type entries + ${baselineDeps.size} deps baselined)`)

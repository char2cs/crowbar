// app/panes/scripts/check-eslint-baselines.mjs — stale-entry gate for the *_BASELINE
import { existsSync } from 'node:fs'
import { ESLint } from 'eslint'
import config, { BASELINES } from '../eslint.config.js'

const unbaselined = config.filter((entry) => !entry.name?.startsWith('baseline/'))
const eslint = new ESLint({ overrideConfigFile: true, overrideConfig: unbaselined })

const problems = []
const files = [...new Set(BASELINES.flatMap((b) => b.files))]
const present = files.filter((file) => existsSync(file))
for (const file of files) {
  if (!present.includes(file)) problems.push(`stale baseline: ${file} no longer exists`)
}

const violations = new Map()
for (const result of await eslint.lintFiles(present)) {
  const rel = result.filePath.slice(process.cwd().length + 1)
  violations.set(rel, new Set(result.messages.map((message) => message.ruleId)))
}

for (const { rule, files: listed } of BASELINES) {
  for (const file of listed) {
    if (present.includes(file) && !violations.get(file)?.has(rule)) {
      problems.push(`stale baseline: ${file} no longer violates ${rule} — remove it`)
    }
  }
}

if (problems.length > 0) {
  for (const problem of problems) console.error(problem)
  process.exit(1)
}
const total = BASELINES.reduce((n, b) => n + b.files.length, 0)
console.log(`eslint baselines: clean (${total} entries, all still violating)`)

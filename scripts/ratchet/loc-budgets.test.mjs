import assert from 'node:assert/strict'
import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'
import { main, measureLoc } from './loc-budgets.mjs'
import { tmpDir } from './test-helpers.mjs'

const f = (path, lines, isTest = false) => ({
  path,
  lang: path.startsWith('api/') ? 'go' : 'web',
  test: isTest,
  text: '\n'.repeat(lines),
})

const modules = {
  go: { svc: ['api/internal/app'], mocks: ['api/internal/app/mocks'], wiring: ['api/internal/app/container.go'] },
  web: {},
}

test('longest prefix wins', () => {
  const out = measureLoc(
    [f('api/internal/app/a.go', 10), f('api/internal/app/mocks/m.go', 20), f('api/internal/app/container.go', 30)],
    modules,
  )
  assert.deepEqual(out, { 'go.svc': 10, 'go.mocks': 20, 'go.wiring': 30, 'go.total': 60, 'go.tests': 0, 'web.total': 0, 'web.tests': 0 })
})

test('longest prefix wins whatever order the modules are declared in', () => {
  const reversed = {
    go: { wiring: ['api/internal/app/container.go'], mocks: ['api/internal/app/mocks'], svc: ['api/internal/app'] },
    web: {},
  }
  const out = measureLoc(
    [f('api/internal/app/a.go', 10), f('api/internal/app/mocks/m.go', 20), f('api/internal/app/container.go', 30)],
    reversed,
  )
  assert.deepEqual(out, { 'go.svc': 10, 'go.mocks': 20, 'go.wiring': 30, 'go.total': 60, 'go.tests': 0, 'web.total': 0, 'web.tests': 0 })
})

test('prefix respects directory boundaries', () => {
  assert.throws(() => measureLoc([f('api/internal/application/x.go', 5)], modules), /unassigned source files[\s\S]*api\/internal\/application\/x\.go/)
})

test('an exact-file prefix does not claim files that merely start with its name', () => {
  assert.throws(() => measureLoc([f('api/internal/app/container.gone.go', 5)], { go: { w: ['api/internal/app/container.go'] }, web: {} }), /container\.gone\.go/)
})

test('test files count toward <lang>.tests only, never a module or the total', () => {
  const out = measureLoc([f('api/internal/app/a.go', 4), f('api/x_test.go', 90, true), f('api/y_test.go', 9, true), f('web/src/__tests__/y.test.ts', 70, true)], modules)
  assert.deepEqual(out, { 'go.svc': 4, 'go.mocks': 0, 'go.wiring': 0, 'go.total': 4, 'go.tests': 99, 'web.total': 0, 'web.tests': 70 })
})

test('a test file no module claims is fine', () => {
  assert.equal(measureLoc([f('api/nowhere/z_test.go', 6, true)], modules)['go.tests'], 6)
})

test('totals are per language and modules with no files are 0', () => {
  const out = measureLoc([f('web/src/features/git/a.ts', 7)], { go: { svc: ['api/internal/app'] }, web: { git: ['web/src/features/git'], empty: ['web/src/extensions'] } })
  assert.deepEqual(out, { 'go.svc': 0, 'go.total': 0, 'go.tests': 0, 'web.git': 7, 'web.empty': 0, 'web.total': 7, 'web.tests': 0 })
})

test('unassigned source file throws and names every offender', () => {
  assert.throws(
    () => measureLoc([f('api/new/x.go', 5), f('web/src/new/y.ts', 5)], modules),
    /unassigned source files[\s\S]*api\/new\/x\.go[\s\S]*web\/src\/new\/y\.ts/,
  )
})

const invalid = (modules) => () => measureLoc([], modules)

test('a prefix claimed by two modules of one language is a config error naming every offender', () => {
  assert.throws(
    invalid({ go: { a: ['api/x', 'api/y'], b: ['api/x'], c: ['api/y'] }, web: { a: ['web/src/x'], b: ['web/src/x'] } }),
    /modules\.json[\s\S]*go: "api\/x" is in a and b[\s\S]*go: "api\/y" is in a and c[\s\S]*web: "web\/src\/x" is in a and b/,
  )
})

test('the same prefix in a different language is not a clash', () => {
  assert.doesNotThrow(invalid({ go: { a: ['api/x'] }, web: { a: ['web/src/x'] } }))
})

test('a prefix ending in a slash is a config error naming every offender', () => {
  assert.throws(
    invalid({ go: { a: ['api/x/'] }, web: { b: ['web/src/y/'] } }),
    /go\.a: "api\/x\/" ends with "\/"[\s\S]*web\.b: "web\/src\/y\/" ends with "\/"/,
  )
})

test('a prefix outside its language root is a config error naming every offender', () => {
  assert.throws(
    invalid({ go: { a: ['web/src/x'], b: ['apix/y'] }, web: { c: ['api/z'], d: ['web/other'] } }),
    /go\.a: "web\/src\/x" is outside api\/[\s\S]*go\.b: "apix\/y" is outside api\/[\s\S]*web\.c: "api\/z" is outside web\/src\/[\s\S]*web\.d: "web\/other" is outside web\/src\//,
  )
})

test('a root directory or file inside the root is a valid prefix', () => {
  assert.doesNotThrow(invalid({ go: { a: ['api', 'api/x.go'] }, web: { a: ['web/src'] } }))
})

test('a module named total is a config error naming every offender', () => {
  assert.throws(invalid({ go: { total: ['api/x'] }, web: { total: ['web/src/y'] } }), /go: a module is named "total"[\s\S]*web: a module is named "total"/)
})

test('main prints a config error and fails without gating', (t) => {
  const lines = []
  const modules = { go: { total: ['api/x'] }, web: {} }
  assert.equal(main([], { files: [], modules, baselinePath: join(tmpDir(t), 'baseline.json'), log: (l) => lines.push(l) }), 1)
  assert.match(lines.join('\n'), /go: a module is named "total"/)
})

const gateOf = (t, baseline, args) => {
  const baselinePath = join(tmpDir(t), 'baseline.json')
  writeFileSync(baselinePath, JSON.stringify(baseline))
  const lines = []
  const run = (files, mods, extra = []) => main([...args, ...extra], { files, modules: mods, baselinePath, log: (l) => lines.push(l) })
  return { run, baselinePath, text: () => lines.join('\n') }
}
const svcModules = { go: { svc: ['api/internal'] }, web: {} }
const svcFiles = [f('api/internal/a.go', 10), f('api/internal/a_test.go', 5, true)]

test('the test totals are first-time keys: --update accepts them', (t) => {
  const g = gateOf(t, { 'go.svc': 10, 'go.total': 10, 'web.total': 0 }, ['--update'])
  assert.equal(g.run(svcFiles, svcModules), 0)
  assert.deepEqual(JSON.parse(readFileSync(g.baselinePath, 'utf8')), { 'go.svc': 10, 'go.tests': 5, 'go.total': 10, 'web.tests': 0, 'web.total': 0 })
})

test('a new module key is still refused, even beside the test totals', (t) => {
  const before = { 'go.svc': 10, 'go.total': 13, 'web.total': 0 }
  const g = gateOf(t, before, ['--update'])
  const grown = { go: { svc: ['api/internal'], fresh: ['api/other'] }, web: {} }
  assert.equal(g.run([...svcFiles, f('api/other/b.go', 3)], grown), 1)
  assert.match(g.text(), /  new    go\.fresh: 3/)
  assert.doesNotMatch(g.text(), /go\.tests/)
  assert.match(g.text(), /refusing to raise a baseline/)
  assert.deepEqual(JSON.parse(readFileSync(g.baselinePath, 'utf8')), before)
})

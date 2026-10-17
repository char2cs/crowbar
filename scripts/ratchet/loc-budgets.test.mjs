import assert from 'node:assert/strict'
import test from 'node:test'
import { measureLoc } from './loc-budgets.mjs'

const f = (path, lines, test = false) => ({
  path,
  lang: path.startsWith('api/') ? 'go' : 'web',
  test,
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
  assert.deepEqual(out, { 'go.svc': 10, 'go.mocks': 20, 'go.wiring': 30, 'go.total': 60, 'web.total': 0 })
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
  assert.deepEqual(out, { 'go.svc': 10, 'go.mocks': 20, 'go.wiring': 30, 'go.total': 60, 'web.total': 0 })
})

test('prefix respects directory boundaries', () => {
  assert.throws(() => measureLoc([f('api/internal/application/x.go', 5)], modules), /unassigned source files[\s\S]*api\/internal\/application\/x\.go/)
})

test('an exact-file prefix does not claim files that merely start with its name', () => {
  assert.throws(() => measureLoc([f('api/internal/app/container.gone.go', 5)], { go: { w: ['api/internal/app/container.go'] }, web: {} }), /container\.gone\.go/)
})

test('test files do not count', () => {
  const out = measureLoc([f('api/internal/app/a.go', 4), f('api/x_test.go', 99, true), f('web/src/__tests__/y.test.ts', 99, true)], modules)
  assert.deepEqual(out, { 'go.svc': 4, 'go.mocks': 0, 'go.wiring': 0, 'go.total': 4, 'web.total': 0 })
})

test('totals are per language and modules with no files are 0', () => {
  const out = measureLoc([f('web/src/features/git/a.ts', 7)], { go: { svc: ['api/internal/app'] }, web: { git: ['web/src/features/git'], empty: ['web/src/extensions'] } })
  assert.deepEqual(out, { 'go.svc': 0, 'go.total': 0, 'web.git': 7, 'web.empty': 0, 'web.total': 7 })
})

test('unassigned source file throws and names every offender', () => {
  assert.throws(
    () => measureLoc([f('api/new/x.go', 5), f('web/src/new/y.ts', 5)], modules),
    /unassigned source files[\s\S]*api\/new\/x\.go[\s\S]*web\/src\/new\/y\.ts/,
  )
})

import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { classify, countLines, diffBaseline, loadCode, runGate } from './lib.mjs'

test('countLines counts newline characters like wc -l', () => {
  assert.equal(countLines(''), 0)
  assert.equal(countLines('a\nb\n'), 2)
  assert.equal(countLines('a\nb'), 1)
  assert.equal(countLines('a\r\nb\r\n'), 2)
})

test('classify admits only Go and web/src TypeScript and marks tests', () => {
  assert.deepEqual(classify('api/internal/x.go'), { lang: 'go', test: false })
  assert.deepEqual(classify('api/internal/x_test.go'), { lang: 'go', test: true })
  assert.deepEqual(classify('web/src/a/b.tsx'), { lang: 'web', test: false })
  assert.deepEqual(classify('web/src/__tests__/a/b.test.ts'), { lang: 'web', test: true })
  for (const p of ['web/src/index.css', 'web/vite.config.ts', 'api/go.mod', 'desktop/src-tauri/src/lib.rs', 'scripts/ratchet/lib.mjs'])
    assert.equal(classify(p), null)
})

test('diffBaseline buckets every kind of change and is empty when equal', () => {
  assert.deepEqual(diffBaseline({ a: 1 }, { a: 1 }), { grew: [], added: [], shrunk: [], gone: [] })
  const d = diffBaseline({ up: 1, down: 5, old: 3 }, { up: 2, down: 4, fresh: 9 })
  assert.deepEqual(d.grew, [{ key: 'up', was: 1, now: 2 }])
  assert.deepEqual(d.shrunk, [{ key: 'down', was: 5, now: 4 }])
  assert.deepEqual(d.added, [{ key: 'fresh', now: 9 }])
  assert.deepEqual(d.gone, [{ key: 'old', was: 3 }])
})

test('diffBaseline sorts each bucket by key', () => {
  const d = diffBaseline({ b: 1, a: 1 }, { b: 2, a: 2, d: 1, c: 1 })
  assert.deepEqual(d.grew.map((c) => c.key), ['a', 'b'])
  assert.deepEqual(d.added.map((c) => c.key), ['c', 'd'])
})

const gate = (dir) => {
  const baselinePath = join(dir, 'baseline.json')
  const lines = []
  const run = (opts) =>
    runGate({ name: 'demo', baselinePath, summary: '2 things', update: false, allowAdded: false, log: (l) => lines.push(l), ...opts })
  return { baselinePath, lines, run, text: () => lines.join('\n') }
}
const tmp = () => mkdtempSync(join(tmpdir(), 'ratchet-'))

test('check with no baseline file fails and points at --update', () => {
  const g = gate(tmp())
  assert.equal(g.run({ measured: { a: 1 } }), 1)
  assert.match(g.text(), /demo: no baseline at .*baseline\.json; run: make ratchet ARGS=--update/)
})

test('check with an equal baseline passes', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, JSON.stringify({ a: 1 }))
  assert.equal(g.run({ measured: { a: 1 } }), 0)
  assert.match(g.lines[0], /^demo: ok \(2 things\)$/)
})

test('check with growth fails, names the change and does not offer a write-back', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, JSON.stringify({ a: 1 }))
  assert.equal(g.run({ measured: { a: 2 } }), 1)
  assert.match(g.text(), /  grew   a: 1 -> 2/)
  assert.match(g.text(), /Baselines never go up: delete or split code\./)
  assert.doesNotMatch(g.text(), /make ratchet ARGS=--update/)
})

test('check with a shrink fails until the shrink is written back', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, JSON.stringify({ a: 5 }))
  assert.equal(g.run({ measured: { a: 4 } }), 1)
  assert.match(g.text(), /  shrank a: 5 -> 4/)
  assert.match(g.text(), /Write the shrink back: make ratchet ARGS=--update, then commit the baseline\./)
  assert.doesNotMatch(g.text(), /Baselines never go up/)
})

test('check with an added key is growth unless allowAdded, and shows the hint only as growth', () => {
  const hint = 'Split the file.'
  const denied = gate(tmp())
  writeFileSync(denied.baselinePath, JSON.stringify({}))
  assert.equal(denied.run({ measured: { fresh: 9 }, hint }), 1)
  assert.match(denied.text(), /  new    fresh: 9/)
  assert.match(denied.text(), /Split the file\./)
  assert.doesNotMatch(denied.text(), /make ratchet ARGS=--update/)

  const allowed = gate(tmp())
  writeFileSync(allowed.baselinePath, JSON.stringify({}))
  assert.equal(allowed.run({ measured: { fresh: 9 }, allowAdded: true, hint }), 1)
  assert.match(allowed.text(), /make ratchet ARGS=--update/)
  assert.doesNotMatch(allowed.text(), /Split the file\./)
})

test('an oversize rename reports both ends and the rename hint', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, JSON.stringify({ 'old.go': 600 }))
  const hint = 'Renamed an oversize file? Change its key in the baseline, value unchanged.'
  assert.equal(g.run({ measured: { 'new.go': 600 }, hint }), 1)
  assert.match(g.text(), /  gone   old\.go: was 600/)
  assert.match(g.text(), /  new    new\.go: 600/)
  assert.match(g.text(), /Renamed/)
})

test('update with no baseline writes sorted keys with a trailing newline', () => {
  const g = gate(tmp())
  assert.equal(g.run({ update: true, measured: { b: 2, a: 1 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "a": 1,\n  "b": 2\n}\n')
  assert.match(g.lines[0], /^demo: baseline created \(2 keys\)$/)
})

test('update refuses growth and leaves the file untouched', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, '{"a":1}')
  assert.equal(g.run({ update: true, measured: { a: 2 } }), 1)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{"a":1}')
  assert.match(g.text(), /refusing to raise/)
})

test('update refuses an added key unless allowAdded', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, '{"a":1}')
  assert.equal(g.run({ update: true, measured: { a: 1, b: 1 } }), 1)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{"a":1}')
  assert.match(g.text(), /refusing to raise/)

  assert.equal(g.run({ update: true, allowAdded: true, measured: { a: 1, b: 1 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "a": 1,\n  "b": 1\n}\n')
})

test('update writes a shrink and a gone key back, then reports already current', () => {
  const g = gate(tmp())
  writeFileSync(g.baselinePath, JSON.stringify({ down: 5, old: 3, same: 1 }))
  assert.equal(g.run({ update: true, measured: { down: 4, same: 1 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "down": 4,\n  "same": 1\n}\n')
  assert.match(g.text(), /demo: baseline updated \(2 keys\)/)

  g.lines.length = 0
  assert.equal(g.run({ update: true, measured: { down: 4, same: 1 } }), 0)
  assert.match(g.lines[0], /^demo: already current$/)
})

test('loadCode lists tracked and untracked unignored Go and web/src files that still exist', () => {
  const dir = tmp()
  const put = (rel, text = 'x\n') => {
    mkdirSync(dirname(join(dir, rel)), { recursive: true })
    writeFileSync(join(dir, rel), text)
  }
  const git = (...args) => execFileSync('git', args, { cwd: dir })
  git('init', '-q')
  put('api/a.go')
  put('api/b_test.go')
  git('add', 'api/a.go')
  rmSync(join(dir, 'api/a.go'))
  put('api/c.go')
  put('api/ignored.go')
  put('.gitignore', 'api/ignored.go\n')
  put('web/src/d.ts', 'hello\n')
  put('web/src/index.css')

  const files = loadCode(dir)
  assert.deepEqual(files.map((f) => f.path).sort(), ['api/b_test.go', 'api/c.go', 'web/src/d.ts'])
  assert.equal(files.find((f) => f.path === 'api/b_test.go').test, true)
  assert.equal(files.find((f) => f.path === 'web/src/d.ts').text, 'hello\n')
})

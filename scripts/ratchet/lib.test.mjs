import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { classify, countLines, diffBaseline, loadCode, runGate } from './lib.mjs'
import { tmpDir } from './test-helpers.mjs'

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

test('classify skips vendored and generated code but not look-alikes', () => {
  for (const p of ['api/third_party/mod/a.go', 'api/third_party/mod/a_test.go', 'web/src/routeTree.gen.ts']) assert.equal(classify(p), null)
  assert.deepEqual(classify('api/third_partyx/a.go'), { lang: 'go', test: false })
  assert.deepEqual(classify('web/src/routes/routeTree.gen.ts'), { lang: 'web', test: false })
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

test('a missing baseline fails in check and update mode, names --init and writes nothing', (t) => {
  for (const update of [false, true]) {
    const g = gate(tmpDir(t))
    assert.equal(g.run({ update, measured: { a: 1 } }), 1)
    assert.match(g.text(), /demo: no baseline at .*baseline\.json; run: make ratchet ARGS=--init/)
    assert.equal(existsSync(g.baselinePath), false)
  }
})

test('check with an equal baseline passes', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, JSON.stringify({ a: 1 }))
  assert.equal(g.run({ measured: { a: 1 } }), 0)
  assert.match(g.lines[0], /^demo: ok \(2 things\)$/)
})

test('check with growth fails, names the change and does not offer a write-back', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, JSON.stringify({ a: 1 }))
  assert.equal(g.run({ measured: { a: 2 } }), 1)
  assert.match(g.text(), /  grew   a: 1 -> 2/)
  assert.match(g.text(), /Baselines never go up: delete or split code\./)
  assert.doesNotMatch(g.text(), /make ratchet ARGS=--update/)
})

test('check with a shrink fails until the shrink is written back', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, JSON.stringify({ a: 5 }))
  assert.equal(g.run({ measured: { a: 4 } }), 1)
  assert.match(g.text(), /  shrank a: 5 -> 4/)
  assert.match(g.text(), /Write the shrink back: make ratchet ARGS=--update, then commit the baseline\./)
  assert.doesNotMatch(g.text(), /Baselines never go up/)
})

test('check never advises a write-back that update would refuse', (t) => {
  const grew = gate(tmpDir(t))
  writeFileSync(grew.baselinePath, JSON.stringify({ up: 1, down: 5 }))
  assert.equal(grew.run({ measured: { up: 2, down: 4 } }), 1)
  assert.match(grew.text(), /  grew   up: 1 -> 2/)
  assert.match(grew.text(), /  shrank down: 5 -> 4/)
  assert.doesNotMatch(grew.text(), /Write the shrink back|make ratchet ARGS=--update/)

  const refused = gate(tmpDir(t))
  writeFileSync(refused.baselinePath, JSON.stringify({ down: 5 }))
  assert.equal(refused.run({ measured: { down: 4, fresh: 1 } }), 1)
  assert.match(refused.text(), /  new    fresh: 1/)
  assert.doesNotMatch(refused.text(), /Write the shrink back|make ratchet ARGS=--update/)
})

test('check with an added key is growth unless allowAdded, and shows the hint only as growth', (t) => {
  const hint = 'Split the file.'
  const denied = gate(tmpDir(t))
  writeFileSync(denied.baselinePath, JSON.stringify({}))
  assert.equal(denied.run({ measured: { fresh: 9 }, hint }), 1)
  assert.match(denied.text(), /  new    fresh: 9/)
  assert.match(denied.text(), /Split the file\./)
  assert.doesNotMatch(denied.text(), /make ratchet ARGS=--update/)

  const allowed = gate(tmpDir(t))
  writeFileSync(allowed.baselinePath, JSON.stringify({}))
  assert.equal(allowed.run({ measured: { fresh: 9 }, allowAdded: true, hint }), 1)
  assert.match(allowed.text(), /make ratchet ARGS=--update/)
  assert.doesNotMatch(allowed.text(), /Split the file\./)
})

test('an oversize rename reports both ends and the hint', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, JSON.stringify({ 'old.go': 600 }))
  const hint = 'Split the file.'
  assert.equal(g.run({ measured: { 'new.go': 600 }, hint }), 1)
  assert.match(g.text(), /  gone   old\.go: was 600/)
  assert.match(g.text(), /  new    new\.go: 600/)
  assert.match(g.text(), /Split the file\./)
})

test('init creates a missing baseline with sorted keys and a trailing newline', (t) => {
  const g = gate(tmpDir(t))
  assert.equal(g.run({ init: true, measured: { b: 2, a: 1 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "a": 1,\n  "b": 2\n}\n')
  assert.match(g.lines[0], /^demo: baseline created \(2 keys\)$/)
})

test('init never overwrites an existing baseline', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, '{"a":9}')
  assert.equal(g.run({ init: true, measured: { a: 1 } }), 1)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{"a":9}')
  assert.match(g.text(), /demo: baseline already exists at .*baseline\.json; --init only creates a missing one/)
})

test('update logs each key it writes, before saying it updated', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, JSON.stringify({ down: 5, old: 3, same: 1 }))
  assert.equal(g.run({ update: true, allowAdded: true, measured: { down: 4, same: 1, fresh: 2 } }), 0)
  assert.deepEqual(g.lines, ['  shrank down: 5 -> 4', '  gone   old: was 3', '  new    fresh: 2', 'demo: baseline updated (3 keys)'])
})

test('update refuses growth and leaves the file untouched', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, '{"a":1}')
  assert.equal(g.run({ update: true, measured: { a: 2 } }), 1)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{"a":1}')
  assert.match(g.text(), /refusing to raise/)
})

test('allowAdded can name the only keys that may be added', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, '{"a":1}')
  assert.equal(g.run({ update: true, allowAdded: ['ok'], measured: { a: 1, ok: 2, nope: 3 } }), 1)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{"a":1}')
  assert.match(g.text(), /  new    nope: 3/)
  assert.doesNotMatch(g.text(), /  new    ok/)

  g.lines.length = 0
  assert.equal(g.run({ update: true, allowAdded: ['ok'], measured: { a: 1, ok: 2 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "a": 1,\n  "ok": 2\n}\n')
})

test('update refuses an added key unless allowAdded', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, '{"a":1}')
  assert.equal(g.run({ update: true, measured: { a: 1, b: 1 } }), 1)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{"a":1}')
  assert.match(g.text(), /refusing to raise/)

  assert.equal(g.run({ update: true, allowAdded: true, measured: { a: 1, b: 1 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "a": 1,\n  "b": 1\n}\n')
})

test('update writes a shrink and a gone key back, then reports already current', (t) => {
  const g = gate(tmpDir(t))
  writeFileSync(g.baselinePath, JSON.stringify({ down: 5, old: 3, same: 1 }))
  assert.equal(g.run({ update: true, measured: { down: 4, same: 1 } }), 0)
  assert.equal(readFileSync(g.baselinePath, 'utf8'), '{\n  "down": 4,\n  "same": 1\n}\n')
  assert.match(g.text(), /demo: baseline updated \(2 keys\)/)

  g.lines.length = 0
  assert.equal(g.run({ update: true, measured: { down: 4, same: 1 } }), 0)
  assert.match(g.lines[0], /^demo: already current$/)
})

test('loadCode lists tracked and untracked unignored Go and web/src files that still exist', (t) => {
  // Inside a git hook GIT_INDEX_FILE and friends point at the outer repo; the fixture repo must not inherit them.
  for (const key of Object.keys(process.env).filter((k) => k.startsWith('GIT_'))) {
    const value = process.env[key]
    delete process.env[key]
    t.after(() => {
      process.env[key] = value
    })
  }
  const dir = tmpDir(t)
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

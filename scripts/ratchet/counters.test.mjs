import assert from 'node:assert/strict'
import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'
import { COUNTERS, listMatches, main, measureCounters } from './counters.mjs'
import { tmpDir } from './test-helpers.mjs'

const file = (lang, text, isTest = false) => ({ path: 'x', lang, test: isTest, text })
const count = (id, lang, ...lines) => measureCounters([file(lang, lines.join('\n'))])[id]

test('go.timers counts the time package timer constructors only', () => {
  assert.equal(count('go.timers', 'go', 'time.After(d)', 'time.NewTimer(d)', 'time.NewTicker(d)', 'time.AfterFunc(d, f)'), 4)
  assert.equal(count('go.timers', 'go', 'time.Now()', 'context.WithTimeout(ctx, d)', 'clock.After(d)'), 0)
})

test('go.discarded-errors counts blank assignments, not declarations or comments', () => {
  assert.equal(count('go.discarded-errors', 'go', '\t_ = f.Close()', '\t_, _ = fmt.Fprintf(w, "x")'), 2)
  assert.equal(
    count('go.discarded-errors', 'go', 'var _ Iface = (*T)(nil)', 'x, _ := m[k]', '_        = models.Telemetry{}', '// _ = f.Close()'),
    0,
  )
})

test('web.timers counts setTimeout and setInterval, not clearTimeout', () => {
  assert.equal(count('web.timers', 'web', 'setTimeout(fn, 1)', 'window.setTimeout(fn, 1)', 'setInterval(fn, 1)'), 3)
  assert.equal(count('web.timers', 'web', 'clearTimeout(id)'), 0)
})

test('web.empty-catch counts bodies of only whitespace and comments', () => {
  assert.equal(
    count('web.empty-catch', 'web', 'try { a() } catch {}', 'try { a() } catch (e) {}', 'try { a() } catch {\n  // ignore\n}', 'try { a() } catch { /* nope */ }'),
    4,
  )
  assert.equal(count('web.empty-catch', 'web', 'try { a() } catch (e) { log(e) }', 'p.catch(handle)'), 0)
})

test('web.swallowed-promise counts .catch handlers that discard the rejection', () => {
  assert.equal(
    count(
      'web.swallowed-promise',
      'web',
      'p.catch(() => {})',
      'p.catch((e) => {})',
      'p.catch(() => undefined)',
      'p.catch(() => null)',
      'p.catch(() => { /* ignore */ })',
    ),
    5,
  )
  assert.equal(count('web.swallowed-promise', 'web', 'p.catch((e) => report(e))', 'p.catch(() => { retry() })'), 0)
})

test('only non-test files of the counter language are scanned', () => {
  assert.equal(measureCounters([file('go', 'setTimeout(fn, 1)')])['web.timers'], 0)
  assert.equal(measureCounters([file('web', 'setTimeout(fn, 1)', true)])['web.timers'], 0)
  assert.equal(measureCounters([file('web', 'setTimeout(fn, 1)'), file('web', 'setTimeout(fn, 2)')])['web.timers'], 2)
})

test('every counter id is present, zero for an empty input', () => {
  const out = measureCounters([])
  assert.deepEqual(Object.keys(out).sort(), COUNTERS.map((c) => c.id).sort())
  assert.ok(Object.values(out).every((n) => n === 0))
})

const src = (path, lang, text, isTest = false) => ({ path, lang, test: isTest, text })

test('listMatches gives path:line for every match of one counter, in the files measureCounters scans', () => {
  const files = [
    src('api/a.go', 'go', 'x\n\ttime.After(d)\nfoo\n\ttime.NewTimer(d)\n'),
    src('api/b.go', 'go', 'time.Now()\ntime.Tick(d)'),
    src('api/a_test.go', 'go', 'time.After(d)', true),
    src('web/src/c.ts', 'web', 'time.After(d)'),
  ]
  const hits = listMatches(files, 'go.timers')
  assert.deepEqual(hits, ['api/a.go:2', 'api/a.go:4', 'api/b.go:2'])
  assert.equal(hits.length, measureCounters(files)['go.timers'])
})

test('listMatches reports a multi-line match at the line it starts on', () => {
  const files = [src('web/src/a.ts', 'web', 'a()\ntry {\n  b()\n} catch (e) {\n  // ignore\n}\nc()')]
  assert.deepEqual(listMatches(files, 'web.empty-catch'), ['web/src/a.ts:4'])
})

test('main --list prints the matches of one counter and does not gate', () => {
  const lines = []
  const files = [src('api/a.go', 'go', 'x\ntime.After(d)')]
  assert.equal(main(['--list', 'go.timers'], { files, log: (l) => lines.push(l) }), 0)
  assert.deepEqual(lines, ['api/a.go:2'])
})

test('main --list with an unknown or missing id fails naming the valid ids', () => {
  for (const args of [['--list', 'go.nope'], ['--list']]) {
    const lines = []
    assert.equal(main(args, { files: [], log: (l) => lines.push(l) }), 1)
    assert.match(lines.join('\n'), /unknown counter.*valid ids: go\.timers, go\.discarded-errors, web\.timers, web\.empty-catch, web\.swallowed-promise/)
  }
})

test('--update never adds a counter key, so a hand-deleted one stays refused and the file is untouched', (t) => {
  const baselinePath = join(tmpDir(t), 'baseline.json')
  const before = { 'go.discarded-errors': 0, 'web.timers': 0, 'web.empty-catch': 0, 'web.swallowed-promise': 0 }
  writeFileSync(baselinePath, JSON.stringify(before))
  const lines = []
  const files = [src('api/a.go', 'go', 'time.After(d)')]
  assert.equal(main(['--update'], { files, baselinePath, log: (l) => lines.push(l) }), 1)
  assert.match(lines.join('\n'), /  new    go\.timers: 1/)
  assert.match(lines.join('\n'), /refusing to raise a baseline/)
  assert.deepEqual(JSON.parse(readFileSync(baselinePath, 'utf8')), before)
})

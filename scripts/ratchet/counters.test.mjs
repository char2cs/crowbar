import assert from 'node:assert/strict'
import test from 'node:test'
import { COUNTERS, measureCounters } from './counters.mjs'

const file = (lang, text, test = false) => ({ path: 'x', lang, test, text })
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

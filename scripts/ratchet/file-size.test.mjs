import assert from 'node:assert/strict'
import test from 'node:test'
import { MAX_LINES, measureOversize } from './file-size.mjs'

const f = (path, n, test = false) => ({ path, lang: path.startsWith('api/') ? 'go' : 'web', test, text: 'x\n'.repeat(n) })

test('MAX_LINES is 500', () => assert.equal(MAX_LINES, 500))

test('measureOversize keeps files strictly over the cap, tests included', () => {
  const out = measureOversize([f('api/a.go', 500), f('api/b.go', 501), f('web/src/__tests__/c.test.ts', 900, true), f('web/src/d.ts', 12)])
  assert.deepEqual(out, { 'api/b.go': 501, 'web/src/__tests__/c.test.ts': 900 })
})

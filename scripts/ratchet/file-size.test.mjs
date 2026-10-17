import assert from 'node:assert/strict'
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'
import { MAX_LINES, main, measureOversize } from './file-size.mjs'
import { tmpDir } from './test-helpers.mjs'

const f = (path, n, isTest = false) => ({ path, lang: path.startsWith('api/') ? 'go' : 'web', test: isTest, text: 'x\n'.repeat(n) })

test('MAX_LINES is 500', () => assert.equal(MAX_LINES, 500))

test('measureOversize keeps files strictly over the cap, tests included', () => {
  const out = measureOversize([f('api/a.go', 500), f('api/b.go', 501), f('web/src/__tests__/c.test.ts', 900, true), f('web/src/d.ts', 12)])
  assert.deepEqual(out, { 'api/b.go': 501, 'web/src/__tests__/c.test.ts': 900 })
})

test('a moved oversize file is told to split, never to re-key the baseline', (t) => {
  const baselinePath = join(tmpDir(t), 'baseline.json')
  writeFileSync(baselinePath, JSON.stringify({ 'api/old.go': 600 }))
  const lines = []
  assert.equal(main([], { files: [f('api/new.go', 600)], baselinePath, log: (l) => lines.push(l) }), 1)
  const out = lines.join('\n')
  assert.match(out, /Split the file below 500 lines\. Moving or renaming an oversize file counts as touching it: split it in the same change\./)
  assert.doesNotMatch(out, /Change its key/)
})

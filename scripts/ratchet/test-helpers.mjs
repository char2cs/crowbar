// Shared by the *.test.mjs files; not itself a test, so the `*.test.mjs` glob skips it.
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

/** A fresh temp dir that is removed when the test (or `before`/`after` context) `t` ends. */
export function tmpDir(t) {
  const dir = mkdtempSync(join(tmpdir(), 'ratchet-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  return dir
}

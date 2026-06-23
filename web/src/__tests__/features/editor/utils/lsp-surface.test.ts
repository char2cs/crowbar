import { describe, it, expect } from 'vitest'

import { shouldSkipLsp } from '@/features/editor/utils/lsp-surface'

// H11 regression: diff buffers use `pathOverride: sourcePath`, so a diff/virtual/
// read-only surface resolves to the REAL workspace file path while holding
// serialized diff text. If such a surface ran the LSP document lifecycle it
// would POST didOpen/didChange for that path — and since documentChange is not
// refcounted, the diff-garbled text would clobber the LSP's in-memory copy of a
// file the user may be editing in a real pane. shouldSkipLsp gates BOTH LSP
// effects in DiffMonacoEditor so diff surfaces never touch the lifecycle.
describe('shouldSkipLsp (H11: keep diff surfaces out of the LSP lifecycle)', () => {
  it('skips when read-only', () => {
    expect(shouldSkipLsp(true, false, false)).toBe(true)
  })

  it('skips in preview mode', () => {
    expect(shouldSkipLsp(false, true, false)).toBe(true)
  })

  it('skips when the buffer is virtual (the diff-viewer case)', () => {
    expect(shouldSkipLsp(false, false, true)).toBe(true)
  })

  it('skips when several flags are set at once', () => {
    expect(shouldSkipLsp(true, true, true)).toBe(true)
  })

  it('does NOT skip a normal editable, non-virtual, non-preview surface', () => {
    expect(shouldSkipLsp(false, false, false)).toBe(false)
  })
})

import { beforeEach, describe, expect, it } from 'vitest'
import {
  excalidrawDesignStorageKey,
  loadExcalidrawDesign,
  MAX_EXCALIDRAW_DESIGN_BYTES,
  saveExcalidrawDesign,
} from '@/features/agent/composer/lib/excalidraw-design-persistence'

describe('excalidraw design persistence', () => {
  beforeEach(() => localStorage.clear())

  it('round-trips a saved scene for the same workspace/chat', () => {
    expect(saveExcalidrawDesign('w1', 'c1', '{"elements":[]}')).toBe(true)
    expect(loadExcalidrawDesign('w1', 'c1')).toBe('{"elements":[]}')
  })

  it('returns null when nothing has been saved for this chat', () => {
    expect(loadExcalidrawDesign('w1', 'c1')).toBeNull()
  })

  it('keys entries by both workspace and chat so different chats never collide', () => {
    saveExcalidrawDesign('w1', 'c1', '{"elements":["a"]}')
    saveExcalidrawDesign('w1', 'c2', '{"elements":["b"]}')
    saveExcalidrawDesign('w2', 'c1', '{"elements":["c"]}')

    expect(loadExcalidrawDesign('w1', 'c1')).toBe('{"elements":["a"]}')
    expect(loadExcalidrawDesign('w1', 'c2')).toBe('{"elements":["b"]}')
    expect(loadExcalidrawDesign('w2', 'c1')).toBe('{"elements":["c"]}')
  })

  it("overwrites a chat's previous saved design rather than keeping both", () => {
    saveExcalidrawDesign('w1', 'c1', '{"elements":["first"]}')
    saveExcalidrawDesign('w1', 'c1', '{"elements":["second"]}')

    expect(loadExcalidrawDesign('w1', 'c1')).toBe('{"elements":["second"]}')
  })

  it('rejects and does not persist a scene over the size cap', () => {
    const huge = JSON.stringify({ elements: [], pad: 'x'.repeat(MAX_EXCALIDRAW_DESIGN_BYTES) })

    expect(saveExcalidrawDesign('w1', 'c1', huge)).toBe(false)
    expect(loadExcalidrawDesign('w1', 'c1')).toBeNull()
  })

  it('escapes workspace/chat ids that themselves contain the key separator', () => {
    expect(excalidrawDesignStorageKey('w/1', 'c:1')).toContain('w%2F1:c%3A1')
  })
})

import { describe, expect, it } from 'vitest'
import { parseExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'

describe('parseExcalidrawScene', () => {
  it('accepts a plausible scene shape', () => {
    const raw = JSON.stringify({
      elements: [{ type: 'rectangle' }],
      appState: { viewBackgroundColor: '#fff' },
    })
    expect(parseExcalidrawScene(raw)).toEqual({
      elements: [{ type: 'rectangle' }],
      appState: { viewBackgroundColor: '#fff' },
    })
  })

  it('rejects invalid JSON', () => {
    expect(parseExcalidrawScene('not json')).toBeNull()
  })

  it('rejects valid JSON missing elements/appState', () => {
    expect(parseExcalidrawScene(JSON.stringify({ foo: 'bar' }))).toBeNull()
  })

  it('rejects elements that is not an array', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: 'nope', appState: {} }))).toBeNull()
  })

  it('rejects a JSON array or primitive at the top level', () => {
    expect(parseExcalidrawScene('[]')).toBeNull()
    expect(parseExcalidrawScene('42')).toBeNull()
  })

  // Additional comprehensive tests for full branch coverage
  it('rejects null at the top level', () => {
    expect(parseExcalidrawScene('null')).toBeNull()
  })

  // REGRESSION: a real `.excalidraw` file — and an agent asked to write
  // "valid Excalidraw JSON" reliably produces this exact shape — routinely
  // omits appState entirely. It holds view/style state, never what's drawn,
  // so a missing one defaults to `{}` rather than rejecting the whole scene.
  it('accepts a scene with no appState at all, defaulting to {}', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: [{ type: 'rectangle' }] }))).toEqual({
      elements: [{ type: 'rectangle' }],
      appState: {},
    })
  })

  // The real shape that surfaced this: type/version/elements/files, no
  // appState — a genuine .excalidraw file export.
  it('accepts the real .excalidraw file export shape (type/version/elements/files, no appState)', () => {
    const raw = JSON.stringify({
      type: 'excalidraw',
      version: 2,
      elements: [{ id: 'a', type: 'rectangle' }],
      files: {},
    })
    expect(parseExcalidrawScene(raw)).toEqual({
      elements: [{ id: 'a', type: 'rectangle' }],
      appState: {},
    })
  })

  it('rejects when appState is not an object', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: [], appState: 'invalid' }))).toBeNull()
  })

  it('rejects when appState is null', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: [], appState: null }))).toBeNull()
  })

  it('rejects when appState is an array', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: [], appState: [] }))).toBeNull()
  })

  it('rejects when elements is null', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: null, appState: {} }))).toBeNull()
  })

  it('rejects when elements is an object', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: {}, appState: {} }))).toBeNull()
  })

  it('rejects when elements is a string', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: 'string', appState: {} }))).toBeNull()
  })

  it('accepts with empty elements array', () => {
    const raw = JSON.stringify({ elements: [], appState: {} })
    expect(parseExcalidrawScene(raw)).toEqual({ elements: [], appState: {} })
  })

  it('accepts with empty appState object', () => {
    const raw = JSON.stringify({ elements: [], appState: {} })
    expect(parseExcalidrawScene(raw)).toEqual({ elements: [], appState: {} })
  })

  it('accepts with extra fields in the root object', () => {
    const raw = JSON.stringify({
      elements: [{ id: 'test' }],
      appState: { zoom: 1 },
      extra: 'field',
      another: 42,
    })
    expect(parseExcalidrawScene(raw)).toEqual({
      elements: [{ id: 'test' }],
      appState: { zoom: 1 },
    })
  })

  it('accepts with complex nested elements', () => {
    const raw = JSON.stringify({
      elements: [
        { type: 'rectangle', x: 10, y: 20, width: 100, height: 50 },
        { type: 'text', text: 'hello', container: { nested: true } },
      ],
      appState: { viewBackgroundColor: '#fff', zoom: { value: 1.5 } },
    })
    expect(parseExcalidrawScene(raw)).toEqual({
      elements: [
        { type: 'rectangle', x: 10, y: 20, width: 100, height: 50 },
        { type: 'text', text: 'hello', container: { nested: true } },
      ],
      appState: { viewBackgroundColor: '#fff', zoom: { value: 1.5 } },
    })
  })

  it('rejects when top level is a string', () => {
    expect(parseExcalidrawScene('"just a string"')).toBeNull()
  })

  it('rejects when top level is a number', () => {
    expect(parseExcalidrawScene('123')).toBeNull()
  })

  it('rejects when top level is a boolean', () => {
    expect(parseExcalidrawScene('true')).toBeNull()
    expect(parseExcalidrawScene('false')).toBeNull()
  })

  it('rejects invalid JSON with various syntax errors', () => {
    expect(parseExcalidrawScene('{')).toBeNull()
    expect(parseExcalidrawScene('{invalid}')).toBeNull()
    expect(parseExcalidrawScene('{"unclosed": ')).toBeNull()
  })
})

import { describe, expect, it, vi } from 'vitest'

// DOM-free: mock monaco-editor so no real editor/DOM is constructed.
vi.mock('monaco-editor', () => {
  class FakeUri {
    constructor(public value: string) {}
    static parse(v: string) {
      return new FakeUri(v)
    }
    toString() {
      return this.value
    }
  }
  return {
    Uri: FakeUri,
    editor: {
      createModel: vi.fn((value: string) => {
        let text = value
        return {
          uri: FakeUri.parse('athas://editor/x'),
          dispose: vi.fn(),
          getValue: vi.fn(() => text),
          getFullModelRange: vi.fn(() => ({})),
          pushEditOperations: vi.fn((_s: unknown, edits: Array<{ text: string }>) => {
            text = edits[0].text
            return null
          }),
        }
      }),
      getModel: vi.fn(() => null),
      create: vi.fn(() => ({
        setModel: vi.fn(),
        getModel: vi.fn(() => null),
        saveViewState: vi.fn(),
        restoreViewState: vi.fn(),
        layout: vi.fn(),
        dispose: vi.fn(),
      })),
    },
  }
})

import {
  EDITOR_CREATE_OPTIONS,
  langForUri,
  realEditorApi,
  realModelApi,
} from '@/features/editor/lib/monaco-adapters'

describe('monaco-adapters', () => {
  it('realModelApi exposes createModel + getModel functions', () => {
    const api = realModelApi()
    expect(typeof api.createModel).toBe('function')
    expect(typeof api.getModel).toBe('function')
  })

  it('realEditorApi exposes a create function', () => {
    const api = realEditorApi(EDITOR_CREATE_OPTIONS)
    expect(typeof api.create).toBe('function')
  })

  it('EDITOR_CREATE_OPTIONS excludes model and disables editContext', () => {
    expect('model' in EDITOR_CREATE_OPTIONS).toBe(false)
    expect(EDITOR_CREATE_OPTIONS.editContext).toBe(false)
    expect(EDITOR_CREATE_OPTIONS.automaticLayout).toBe(false)
  })

  it('createModel wraps the model with a string uri and dispose()', () => {
    const model = realModelApi().createModel('hello', 'typescript', 'athas://editor/x')
    expect(typeof model.uri).toBe('string')
    expect(typeof model.dispose).toBe('function')
  })

  it('setValueIfChanged uses pushEditOperations (preserves undo) only when text differs', () => {
    const model = realModelApi().createModel('hello', 'typescript', 'athas://editor/x')
    // No-op when unchanged.
    model.setValueIfChanged('hello')
    expect(model.getValue()).toBe('hello')
    // Applies via edit op when changed.
    model.setValueIfChanged('world')
    expect(model.getValue()).toBe('world')
  })

  it('langForUri derives a monaco language id from the file path', () => {
    expect(langForUri('athas://editor/' + encodeURIComponent('/proj/main.ts'))).toBe('typescript')
  })
})

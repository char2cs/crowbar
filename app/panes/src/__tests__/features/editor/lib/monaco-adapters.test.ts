import { describe, expect, it, vi } from 'vitest'
import { documentUri } from '@/features/editor/documents/document-ref'
import { workspaceRef } from '@/lib/api/documents'

vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => {
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
          uri: FakeUri.parse('crowbar://editor/x'),
          dispose: vi.fn(),
          getValue: vi.fn(() => text),
          getFullModelRange: vi.fn(() => ({})),
          pushStackElement: vi.fn(),
          pushEditOperations: vi.fn((_s: unknown, edits: Array<{ text: string }>) => {
            text = edits[0].text
            return null
          }),
        }
      }),
      getModel: vi.fn(() => null),
      getEditors: vi.fn(() => []),
      create: vi.fn(() => ({
        setModel: vi.fn(),
        getModel: vi.fn(() => null),
        saveViewState: vi.fn(),
        restoreViewState: vi.fn(),
        layout: vi.fn(),
        dispose: vi.fn(),
      })),
    },
    languages: {
      getLanguages: vi.fn(() => []),
      register: vi.fn(),
      setMonarchTokensProvider: vi.fn(),
      registerDocumentRangeSemanticTokensProvider: vi.fn(),
    },
  }
})

import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
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

  it('EDITOR_CREATE_OPTIONS requests no-user-select to close the WKWebView native-selection gap', () => {
    expect(EDITOR_CREATE_OPTIONS.extraEditorClassName).toBe('no-user-select')
  })

  it('createModel wraps the model with a string uri and dispose()', () => {
    const model = realModelApi().createModel('hello', 'typescript', 'crowbar://editor/x')
    expect(typeof model.uri).toBe('string')
    expect(typeof model.dispose).toBe('function')
  })

  it('replaceAll replaces the whole text as an undo step of its own', () => {
    const model = realModelApi().createModel('hello', 'typescript', 'crowbar://editor/x')
    const raw = vi.mocked(monacoEditor.createModel).mock.results.at(-1)?.value

    model.replaceAll('world', false)

    expect(model.getValue()).toBe('world')
    expect(raw.pushStackElement).toHaveBeenCalledTimes(2)
  })

  it('langForUri derives a monaco language id from the file path', () => {
    expect(langForUri(documentUri(workspaceRef('ws-1', '/proj/main.ts')))).toBe('typescript')
  })
})

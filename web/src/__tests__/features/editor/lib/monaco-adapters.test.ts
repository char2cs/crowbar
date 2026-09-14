import { describe, expect, it, vi } from 'vitest'

// DOM-free: mock the light `editor.api.js` entry (what `monaco-adapters.ts`
// imports instead of the bare `monaco-editor` specifier — see the comment
// there) so no real editor/DOM is constructed.
//
// `langForUri` (under test below) also imports `language-contributions.ts`,
// which imports the SAME `editor.api.js` specifier for its `languages`
// namespace (registers the custom diff/gitignore/… Monarch languages and the
// tree-sitter semantic-tokens provider at module top level) — so the mock
// needs a minimal `languages` fake too, or that import throws before any test
// in this file runs. `loadLanguageForPath` itself resolves its per-language
// loaders via DEEP subpath imports (e.g.
// `monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution`),
// which are separate module specifiers this mock does not intercept — those
// load the real (small, registration-only) contribution modules, same as the
// dedicated `language-contributions.test.ts` suite.
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
    languages: {
      getLanguages: vi.fn(() => []),
      register: vi.fn(),
      setMonarchTokensProvider: vi.fn(),
      registerDocumentRangeSemanticTokensProvider: vi.fn(),
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

  // Regression: on Safari/WKWebView, Monaco's own `getExtraEditorClassName()`
  // adds `enable-user-select` instead of `no-user-select`, leaving
  // `.view-lines`/`.view-line`/`.lines-content` natively selectable
  // (viewLines.css). A drag starting on an unrelated, oversized-but-blank hit
  // area elsewhere in the app (never seen by Monaco's own mousedown guard)
  // can then sweep a native WebKit selection across the whole visible editor,
  // including the padded empty space below the last line. Requesting
  // `no-user-select` ourselves wins for those three elements regardless of
  // Monaco's own WebKit-specific class (see monaco-adapters.ts comment).
  it('EDITOR_CREATE_OPTIONS requests no-user-select to close the WKWebView native-selection gap', () => {
    expect(EDITOR_CREATE_OPTIONS.extraEditorClassName).toBe('no-user-select')
  })

  it('createModel wraps the model with a string uri and dispose()', () => {
    const model = realModelApi().createModel('hello', 'typescript', 'crowbar://editor/x')
    expect(typeof model.uri).toBe('string')
    expect(typeof model.dispose).toBe('function')
  })

  it('setValueIfChanged uses pushEditOperations (preserves undo) only when text differs', () => {
    const model = realModelApi().createModel('hello', 'typescript', 'crowbar://editor/x')
    // No-op when unchanged.
    model.setValueIfChanged('hello')
    expect(model.getValue()).toBe('hello')
    // Applies via edit op when changed.
    model.setValueIfChanged('world')
    expect(model.getValue()).toBe('world')
  })

  it('langForUri derives a monaco language id from the file path', () => {
    expect(langForUri('crowbar://editor/' + encodeURIComponent('/proj/main.ts'))).toBe('typescript')
  })
})

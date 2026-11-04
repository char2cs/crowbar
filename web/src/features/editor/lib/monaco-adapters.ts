// `editor.api` is the same singleton as bare 'monaco-editor' without its eager
// imports. The languages register here, before any model this module creates names one.
import { editor as monacoEditor, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import '@/features/editor/monaco/language-contributions'
import type { IEditorLike, MonacoEditorApi } from '@/features/editor/lib/editor-manager'
import type { IModelLike, MonacoModelApi } from '@/features/editor/documents/document-registry'
import { refFromUri } from '@/features/editor/documents/document-ref'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { toMonacoLanguageId } from '@/features/editor/monaco/language'

/**
 * Structural create options; settings-derived ones (font, theme, readOnly) are
 * applied later through `updateOptions`, and the manager assigns the model.
 */
export const EDITOR_CREATE_OPTIONS: monacoEditor.IStandaloneEditorConstructionOptions = {
  // The surface's ResizeObserver lays the widget out, off the per-frame drag path.
  automaticLayout: false,
  // EditContext's post-render bounds walk dominated drag-select cost (INP 564ms →
  // 178ms once disabled); the hidden-textarea input path is used instead.
  editContext: false,
  // WKWebView gets `enable-user-select`, so a drag sweeping in from a blank area
  // paints native selection over the whole editor; force the class every other
  // browser gets (Monaco's own selection model is unaffected).
  extraEditorClassName: 'no-user-select',
  insertSpaces: true,
  detectIndentation: false,
  // Collapse Monaco's default 10px decorations strip between the line-number
  // column and the text so the number sits snug against the code.
  lineDecorationsWidth: 0,
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  occurrencesHighlight: 'off',
  selectionHighlight: true,
  quickSuggestions: true,
  suggestOnTriggerCharacters: true,
  parameterHints: { enabled: true },
  cursorStyle: 'line',
  cursorBlinking: 'blink',
  contextmenu: false,
  overviewRulerLanes: 0,
  fixedOverflowWidgets: true,
  scrollbar: {
    vertical: 'auto',
    horizontal: 'auto',
  },
}

/** Monaco language id for a model URI. */
export function langForUri(uri: string): string {
  return toMonacoLanguageId(getLanguageIdFromPath(refFromUri(uri)?.path ?? ''))
}

/** Wrap a real Monaco text model so it satisfies `IModelLike` (string `uri`). */
function wrapModel(model: monacoEditor.ITextModel): IModelLike {
  return {
    get uri() {
      return model.uri.toString()
    },
    dispose() {
      model.dispose()
    },
    getValue() {
      return model.getValue()
    },
    getValueLength() {
      return model.getValueLength()
    },
    getAlternativeVersionId() {
      return model.getAlternativeVersionId()
    },
    onDidChangeContent(listener) {
      return model.onDidChangeContent(() => listener())
    },
    replaceAll(text, preserveSelections) {
      const showing = preserveSelections
        ? monacoEditor.getEditors().filter((e) => e.getModel() === model)
        : []
      const carets = showing.map((e) => e.getSelections())
      // One undo step of its own (unlike setValue, which resets the undo stack).
      model.pushStackElement()
      model.pushEditOperations(null, [{ range: model.getFullModelRange(), text }], () => null)
      model.pushStackElement()
      showing.forEach((e, i) => {
        const selections = carets[i]
        if (selections) e.setSelections(selections)
      })
    },
  }
}

/** `MonacoModelApi` over monaco; model uris stay the registry's string keys. */
export function realModelApi(): MonacoModelApi {
  return {
    createModel(value, languageId, uri) {
      return wrapModel(monacoEditor.createModel(value, languageId, Uri.parse(uri)))
    },
    getModel(uri) {
      const model = monacoEditor.getModel(Uri.parse(uri))
      return model ? wrapModel(model) : null
    },
  }
}

/** `MonacoEditorApi` over monaco, exposing models as the string-uri `IModelLike`. */
export function realEditorApi(
  options: monacoEditor.IStandaloneEditorConstructionOptions,
): MonacoEditorApi {
  return {
    create(container) {
      const editor = monacoEditor.create(container, { ...options })
      const adapter: IEditorLike = {
        setModel(model) {
          // The registry/manager always hand back models created via realModelApi,
          // so the underlying monaco model is resolvable by its uri.
          editor.setModel(model ? monacoEditor.getModel(Uri.parse(model.uri)) : null)
        },
        getModel() {
          const model = editor.getModel()
          return model ? wrapModel(model) : null
        },
        saveViewState() {
          return editor.saveViewState()
        },
        restoreViewState(state) {
          editor.restoreViewState(state as monacoEditor.ICodeEditorViewState | null)
        },
        layout() {
          editor.layout()
        },
        dispose() {
          editor.dispose()
        },
        raw() {
          return editor
        },
      }
      return adapter
    },
  }
}

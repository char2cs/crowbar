import { editor as monacoEditor, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import '@/features/editor/monaco/language-contributions'
import type { IEditorLike, MonacoEditorApi } from '@/features/editor/lib/editor-manager'
import type { IModelLike, MonacoModelApi } from '@/features/editor/documents/document-registry'
import { refFromUri } from '@/features/editor/documents/document-ref'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { toMonacoLanguageId } from '@/features/editor/monaco/language'

export const EDITOR_CREATE_OPTIONS: monacoEditor.IStandaloneEditorConstructionOptions = {
  automaticLayout: false,
  editContext: false,
  extraEditorClassName: 'no-user-select',
  insertSpaces: true,
  detectIndentation: false,
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

export function langForUri(uri: string): string {
  return toMonacoLanguageId(getLanguageIdFromPath(refFromUri(uri)?.path ?? ''))
}

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

export function realEditorApi(
  options: monacoEditor.IStandaloneEditorConstructionOptions,
): MonacoEditorApi {
  return {
    create(container) {
      const editor = monacoEditor.create(container, { ...options })
      const adapter: IEditorLike = {
        setModel(model) {
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

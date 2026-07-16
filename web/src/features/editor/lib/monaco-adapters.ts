// The bare 'monaco-editor' specifier resolves to the package's "main" bundle
// (`editor.main.js`), which EAGERLY statically imports all 60+ built-in
// basic-languages contributions + the 4 heavy language services (verified by
// reading monaco's own source: editor.main.js literally does
// `import '../basic-languages/<lang>/<lang>.contribution.js'` for every
// built-in language). `editor.api` is the exact same `editor`/`Uri` singleton
// (both resolve through the shared `editor.api2.js`) MINUS that eager
// bundling — this is the load-bearing prerequisite for `language-contributions
// .ts`'s on-demand loading to have any effect at all; importing the bare
// specifier ANYWHERE would silently defeat it.
import { editor as monacoEditor, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type { IEditorLike, MonacoEditorApi } from '@/features/editor/lib/editor-manager'
import type { IModelLike, MonacoModelApi } from '@/features/editor/lib/model-registry'
import { uriToFsPath } from '@/features/editor/lib/editor-uri'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { toMonacoLanguageId } from '@/features/editor/monaco/language'
import { loadLanguageForPath } from '@/features/editor/monaco/language-contributions'

/**
 * Monaco standalone-editor create options, copied verbatim from
 * `monaco-editor.tsx`'s `monacoEditor.create(...)` call, EXCLUDING `model`
 * (the EditorManager assigns the model). Settings-derived values (font,
 * theme, readOnly, …) are applied later by the consumer via `updateOptions`;
 * here we keep the structural/perf-critical options — notably
 * `automaticLayout: false` (the manager drives layout) and `editContext: false`
 * (the Monaco 0.55 EditContext perf workaround).
 */
export const EDITOR_CREATE_OPTIONS: monacoEditor.IStandaloneEditorConstructionOptions = {
  // The retained per-pane widget is laid out by the surface's ResizeObserver
  // (rAF-debounced, suppressed during a pane-resize drag and flushed on
  // pane-resize-end) — keeping editor.layout() off the per-frame drag path so
  // the GPU layer-promotion (editor-theme.css) can keep resize smooth.
  automaticLayout: false,
  // Monaco 0.55 enables the experimental EditContext input mode by default on
  // Chromium. Its post-render handler (_updateSelectionAndControlBoundsAfterRender)
  // calls getDomNodePagePosition — a getBoundingClientRect walk up the offsetParent
  // chain — on every selection/render. Profiling a drag-select across split panes
  // showed this as the dominant cost (INP 564ms → 178ms, forced reflow 768ms → 391ms
  // once disabled). Fall back to the classic hidden-textarea input path.
  editContext: false,
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

/**
 * Monaco language id for a model URI (uri → fs path → athas id → monaco id).
 *
 * Also kicks off (fire-and-forget) the on-demand grammar/language-service load
 * for this path via `loadLanguageForPath` — see `language-contributions.ts`.
 * NOT awaited: this function must return the language id synchronously so the
 * caller (`ModelRegistry.acquire` → `monacoEditor.createModel`) can create the
 * model immediately, without threading an async contract through
 * `EditorManager`/`ModelRegistry`/the pane-switch controller. If the
 * contribution resolves shortly after, monaco retokenizes the already-created
 * model in place once its language is registered — a brief plaintext flash
 * that self-corrects, not a permanent gap (see task-5-report.md for what was
 * observed live). A rejected load (e.g. a transient chunk-load failure) is
 * swallowed here so it never surfaces as an unhandled rejection from this
 * fire-and-forget kickoff — the user just gets no highlighting for that file.
 */
export function langForUri(uri: string): string {
  const fsPath = uriToFsPath(uri)
  void loadLanguageForPath(fsPath).catch(() => {})
  return toMonacoLanguageId(getLanguageIdFromPath(fsPath))
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
    setValueIfChanged(text: string) {
      if (model.getValue() === text) return
      // pushEditOperations replaces the full range as a single undoable edit,
      // preserving the existing undo stack (unlike setValue, which resets it).
      model.pushEditOperations(null, [{ range: model.getFullModelRange(), text }], () => null)
    },
  }
}

/**
 * Real `MonacoModelApi` over `monaco-editor`. Model URIs are the string keys
 * produced by `editor-uri.ts`; we parse them into `monaco.Uri` for the model
 * store and adapt the model's `.uri` back to a string so the registry's
 * string-keyed bookkeeping stays consistent.
 */
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

/**
 * Real `MonacoEditorApi` over `monaco-editor`. `create` instantiates a
 * standalone editor with the given options and adapts it to `IEditorLike`,
 * exposing models as the string-uri `IModelLike` view.
 */
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

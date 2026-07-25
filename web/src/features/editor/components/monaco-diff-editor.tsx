import '../monaco/monaco-environment'
import '../monaco/language-contributions'
import 'monaco-editor/min/vs/editor/editor.main.css'
import '../styles/monaco-editor.css'

// The bare 'monaco-editor' specifier resolves to the package's "main" bundle
// (`editor.main.js`), which EAGERLY statically imports all 60+ built-in
// basic-languages contributions + the 4 heavy language services — the exact
// eager cost this task removes. `editor.api` is the same real editor/languages
// singleton (both resolve through the shared `editor.api2.js`) MINUS that
// eager language bundling; `language-contributions.ts` now owns loading
// languages on demand instead.
import {
  editor as monacoEditor,
  KeyCode,
  KeyMod,
  Range as MonacoRange,
  Uri,
} from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEventHandler,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useEditorSettingsStore } from '../stores/settings-store'
import { useEditorStateStore } from '../stores/state-store'
import { useEditorUIStore } from '../stores/ui-store'
import type { Position, Range } from '../types/editor'
import { getLanguageIdFromPath } from '../utils/language-id'
import { calculateLineHeight } from '../utils/lines'
import { shouldSkipLsp } from '../utils/lsp-surface'
import { editorAPI } from '../extensions/api'
import { LspClient, type LspDiagnostic } from '../lsp/lsp-client'
import type {
  EditorCoordinateResolver,
  EditorModelPositionResolver,
} from '../view-model/view-layout'
import { toMonacoLanguageId } from '../monaco/language'
import { loadLanguageForPath } from '../monaco/language-contributions'
import {
  toEditorPosition,
  clampMonacoPosition,
  toClampedMonacoPosition,
  toEditorRange,
  toMonacoRange,
  toMonacoMarker,
  pathsMatch,
} from '../monaco/editor-conversions'
import { defineMonacoTheme } from '../monaco/define-theme'
import { useDiffCommentZones, type CommentZoneSpec } from './use-diff-comment-zones'

interface DiffMonacoEditorProps {
  paneId?: string
  bufferId?: string
  viewStateKey?: string
  isActiveSurface?: boolean
  isPreviewMode?: boolean
  readOnly?: boolean
  scrollable?: boolean
  backgroundLayer?: ReactNode
  onReadonlySurfaceClick?: (position: { line: number; column: number }) => void
  highlightMatches?: Array<{ start: number; end: number }>
  currentHighlightIndex?: number
  lineNumberStart?: number
  lineNumberMap?: Array<number | null>
  /** Inline review-comment threads rendered as Monaco view zones. */
  commentZones?: CommentZoneSpec[]
  /** Fired when the gutter "+" is clicked on a (1-based) model line. */
  onAddCommentAtLine?: (modelLine: number) => void
  /** Total content height incl. view zones — lets a fixed-height host grow. */
  onContentHeightChange?: (height: number) => void
  /**
   * Per-model-line diff kind. When provided, the added/removed tint is applied
   * as whole-line Monaco decorations instead of a CSS overlay, so it stays
   * aligned when inline comment view zones push lines down.
   */
  diffLineKinds?: Array<'context' | 'added' | 'removed' | 'spacer'>
  /** Diff-wide search hits in this editor (1-based model line + columns). */
  diffSearchMatches?: Array<{ lineNumber: number; startColumn: number; endColumn: number }> | null
  /** The active hit (highlighted distinctly + scrolled into view), if it's here. */
  activeDiffSearchMatch?: { lineNumber: number; startColumn: number; endColumn: number } | null
  /** Bumps to re-trigger reveal of `activeDiffSearchMatch` (repeat next/prev). */
  diffSearchRevealNonce?: number
  onContentChange?: (
    content: string,
    previousContent?: string,
    previousCursorPosition?: Position,
    previousSelection?: Range,
  ) => void
  onVisibleLineRangeChange?: (range: { startLine: number; endLine: number }) => void
  onScrollOffsetChange?: (scrollTop: number, scrollLeft: number) => void
  onCoordinateResolverChange?: (resolver: EditorCoordinateResolver | null) => void
  onModelPositionResolverChange?: (resolver: EditorModelPositionResolver | null) => void
  onMouseMove?: MouseEventHandler<HTMLDivElement>
  onMouseLeave?: () => void
  onMouseEnter?: () => void
  onClick?: MouseEventHandler<HTMLDivElement>
  className?: string
}

// Stable empty reference so DiffMonacoEditor instances without a comment layer
// don't re-run the zone hook on every render.
const EMPTY_COMMENT_ZONES: CommentZoneSpec[] = []

function createModelUri(bufferId: string | undefined, filePath: string): Monaco.Uri {
  const sanitizedPath = filePath.replace(/^\/+/, '')
  const path = sanitizedPath.length > 0 ? sanitizedPath : `${bufferId ?? 'untitled'}.txt`
  return Uri.parse(`crowbar://editor/${encodeURIComponent(bufferId ?? path)}/${path}`)
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive diff host — one Monaco diff instance whose decoration/search/resolver layers all share the editor+model refs; already fixed for other RD rules this program, no natural seam.
export function DiffMonacoEditor({
  bufferId: propBufferId,
  viewStateKey,
  isActiveSurface = true,
  isPreviewMode = false,
  readOnly = false,
  scrollable = true,
  backgroundLayer,
  onReadonlySurfaceClick,
  highlightMatches,
  currentHighlightIndex,
  lineNumberStart,
  lineNumberMap,
  commentZones,
  onAddCommentAtLine,
  onContentHeightChange,
  diffLineKinds,
  diffSearchMatches,
  activeDiffSearchMatch,
  diffSearchRevealNonce,
  onContentChange,
  onVisibleLineRangeChange,
  onScrollOffsetChange,
  onCoordinateResolverChange,
  onModelPositionResolverChange,
  onMouseMove,
  onMouseLeave,
  onMouseEnter,
  onClick,
  className,
}: DiffMonacoEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const editorRef = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null)
  const modelRef = useRef<Monaco.editor.ITextModel | null>(null)
  const applyingExternalChangeRef = useRef(false)
  const previousContentRef = useRef('')
  const decorationCollectionRef = useRef<Monaco.editor.IEditorDecorationsCollection | null>(null)
  const latestContentChangeRef = useRef(onContentChange)
  // Flips true once the Monaco editor instance exists, so the comment-zone hook
  // (which reads editorRef imperatively) can attach after creation.
  const [editorReady, setEditorReady] = useState(false)
  // Diff-viewer editor. The retained per-pane widget (owned by EditorManager) for
  // real workspace panes lives in `editor-surface.tsx` (usePaneEditorController +
  // usePaneEditorSatellites). This component is the create-per-instance path used
  // by the git diff viewer, which renders MANY read-only editors concurrently and
  // shares source paths across split left/right surfaces. Each instance keeps a
  // private editor+model because (a) they would all collide on a single pane
  // widget, and (b) the manager keys models by file path, so split editors
  // sharing a path would share one model.
  const activeBufferId = useWorkspaceStoreContext(
    useCallback(
      (state) => propBufferId ?? state.panes[state.activePaneId]?.activeBufferId ?? null,
      [propBufferId],
    ),
  )
  const activeBuffer = useWorkspaceStoreContext(
    useCallback(
      (state) =>
        activeBufferId
          ? state.buffers.find((buffer) => buffer.id === activeBufferId) || null
          : null,
      [activeBufferId],
    ),
  )
  const buffer = activeBuffer && activeBuffer.type === 'editor' ? activeBuffer : null
  const content = buffer?.content ?? ''
  const lastAppliedContentRef = useRef(content)
  const filePath = buffer?.path ?? ''
  const languageId = buffer?.languageOverride ?? getLanguageIdFromPath(filePath)
  const monacoLanguageId = toMonacoLanguageId(languageId)
  // On-demand grammar load for the diff surface (Task 5). The model's language
  // id is assigned synchronously below (createModel / the setModelLanguage
  // effect) — this kicks off the extension-keyed contribution fetch so
  // highlighting appears once it registers, without a code pane having opened
  // the same language first. Fire-and-forget by design (a brief plaintext
  // flash self-corrects; see language-contributions.ts); load failures are
  // swallowed — the diff just stays unhighlighted. Extension-keyed, so a
  // `languageOverride` that disagrees with the path's extension won't fetch
  // the override's grammar (same policy as the code-pane path via langForUri).
  useEffect(() => {
    if (filePath) void loadLanguageForPath(filePath).catch(() => {})
  }, [filePath])
  const baseFontSize = useEditorSettingsStore.use.fontSize()
  const fontFamily = useEditorSettingsStore.use.fontFamily()
  const editorLineHeight = useEditorSettingsStore.use.lineHeight()
  const tabSize = useEditorSettingsStore.use.tabSize()
  const wordWrap = useEditorSettingsStore.use.wordWrap()
  const lineNumbers = useEditorSettingsStore.use.lineNumbers()
  const renderWhitespace = useEditorSettingsStore.use.renderWhitespace()
  const renderIndentGuides = useEditorSettingsStore.use.renderIndentGuides()
  const semanticHighlighting = useEditorSettingsStore.use.semanticHighlighting()
  const highlightOccurrences = useEditorSettingsStore.use.highlightOccurrences()
  const theme = useEditorSettingsStore.use.theme()
  const zoomLevel = useZoomStore.use.editorZoomLevel()
  const settingsTheme = useSettingsStore((state) => state.settings.theme)
  const minimapEnabled = useSettingsStore((state) => state.settings.showMinimap)
  const autoCompletion = useSettingsStore((state) => state.settings.autoCompletion)
  const parameterHints = useSettingsStore((state) => state.settings.parameterHints)
  const { setCursorPosition, setSelection, setScrollForBuffer, setViewportHeight } =
    useEditorStateStore.use.actions()
  const searchMatches = useEditorUIStore.use.searchMatches()
  const currentSearchMatchIndex = useEditorUIStore.use.currentMatchIndex()

  const fontSize = baseFontSize * zoomLevel
  const lineHeight = calculateLineHeight(fontSize, editorLineHeight)
  const modelUri = useMemo(
    () => createModelUri(activeBufferId ?? undefined, filePath),
    [activeBufferId, filePath],
  )

  // Stable identity for editorAPI adapter registration — changes only when the
  // buffer or view key changes, NOT when isActiveSurface changes.
  const adapterOwnerId = useMemo(
    () => viewStateKey ?? activeBufferId ?? modelUri.toString(),
    [viewStateKey, activeBufferId, modelUri],
  )

  latestContentChangeRef.current = onContentChange

  const lineNumberFormatter = useCallback(
    (lineNumber: number) => {
      const mappedLine = lineNumberMap?.[lineNumber - 1]
      if (typeof mappedLine === 'number') return String(mappedLine)
      return String((lineNumberStart ?? 1) + lineNumber - 1)
    },
    [lineNumberMap, lineNumberStart],
  )

  // Refs for values used inside the creation effect that should NOT trigger remounts.
  // Settings changes are handled by the updateOptions effect below.
  const latestOnScrollOffsetChangeRef = useRef(onScrollOffsetChange)
  latestOnScrollOffsetChangeRef.current = onScrollOffsetChange

  const latestEditorSettingsRef = useRef({
    fontFamily,
    fontSize,
    lineHeight,
    tabSize,
    wordWrap,
    lineNumbers,
    lineNumberFormatter,
    renderWhitespace,
    renderIndentGuides,
    semanticHighlighting,
    highlightOccurrences,
    minimapEnabled,
    autoCompletion,
    parameterHints,
    settingsTheme,
    theme,
    scrollable,
    monacoLanguageId,
  })
  latestEditorSettingsRef.current = {
    fontFamily,
    fontSize,
    lineHeight,
    tabSize,
    wordWrap,
    lineNumbers,
    lineNumberFormatter,
    renderWhitespace,
    renderIndentGuides,
    semanticHighlighting,
    highlightOccurrences,
    minimapEnabled,
    autoCompletion,
    parameterHints,
    settingsTheme,
    theme,
    scrollable,
    monacoLanguageId,
  }

  const latestOnVisibleLineRangeChangeRef = useRef(onVisibleLineRangeChange)
  latestOnVisibleLineRangeChangeRef.current = onVisibleLineRangeChange

  const updateVisibleLineRange = useCallback((editor: Monaco.editor.IStandaloneCodeEditor) => {
    const visibleRanges = editor.getVisibleRanges()
    const firstRange = visibleRanges[0]
    const lastRange = visibleRanges[visibleRanges.length - 1] ?? firstRange
    if (!firstRange || !lastRange) return

    latestOnVisibleLineRangeChangeRef.current?.({
      startLine: Math.max(0, firstRange.startLineNumber - 1 - 30),
      endLine: Math.max(0, lastRange.endLineNumber - 1 + 30),
    })
  }, [])

  const syncCursorAndSelectionRef = useRef<() => void>(() => {})
  syncCursorAndSelectionRef.current = () => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return

    const position = editor.getPosition()
    if (position) {
      setCursorPosition(toEditorPosition(model, position), { ensureVisible: false })
    }
    const selection = editor.getSelection()
    setSelection(selection ? toEditorRange(model, selection) : undefined)
  }
  const syncCursorAndSelection = useCallback(() => syncCursorAndSelectionRef.current(), [])

  // ── Legacy creation effect (non-managed / standalone consumers) ──────────
  // Used when no `paneId` is supplied (git diff viewer, etc.). Creates a private
  // Monaco editor + model per instance and disposes them on unmount — the exact
  // behavior that shipped before the retained-widget refactor. Pane editors use
  // the manager path above instead.
  // react-doctor-disable-next-line effect-needs-cleanup -- cleanup exists (l.504-520 disposes editor + model + disposables); tracer can't follow it.
  useEffect(() => {
    const container = containerRef.current
    if (!container || !buffer) return

    // Read all settings from the ref so this effect never remounts due to settings changes.
    // Settings-only changes are handled by the updateOptions effect below.
    const s = latestEditorSettingsRef.current

    const model = monacoEditor.createModel(content, s.monacoLanguageId, modelUri)
    // Apply tab settings on the model directly (model-level options)
    model.updateOptions({ tabSize: s.tabSize, insertSpaces: true })
    const editor = monacoEditor.create(container, {
      model,
      automaticLayout: false,
      // Monaco 0.55 enables the experimental EditContext input mode by default on
      // Chromium. Its post-render handler (_updateSelectionAndControlBoundsAfterRender)
      // calls getDomNodePagePosition — a getBoundingClientRect walk up the offsetParent
      // chain — on every selection/render. Profiling a drag-select across split panes
      // showed this as the dominant cost (INP 564ms → 178ms, forced reflow 768ms → 391ms
      // once disabled). Fall back to the classic hidden-textarea input path.
      editContext: false,
      fontFamily: s.fontFamily,
      fontSize: s.fontSize,
      lineHeight: s.lineHeight,
      tabSize: s.tabSize,
      insertSpaces: true,
      detectIndentation: false,
      readOnly: readOnly || isPreviewMode,
      domReadOnly: readOnly || isPreviewMode,
      minimap: { enabled: s.minimapEnabled },
      scrollBeyondLastLine: false,
      lineNumbers: s.lineNumbers ? s.lineNumberFormatter : 'off',
      renderWhitespace:
        s.renderWhitespace === 'none'
          ? 'none'
          : (s.renderWhitespace as Monaco.editor.IEditorOptions['renderWhitespace']),
      wordWrap: s.wordWrap ? 'on' : 'off',
      guides: {
        indentation: s.renderIndentGuides,
        highlightActiveIndentation: s.renderIndentGuides,
      },
      'semanticHighlighting.enabled': s.semanticHighlighting,
      occurrencesHighlight: s.highlightOccurrences ? 'singleFile' : 'off',
      selectionHighlight: true,
      quickSuggestions: s.autoCompletion,
      suggestOnTriggerCharacters: s.autoCompletion,
      parameterHints: { enabled: s.parameterHints },
      theme: defineMonacoTheme(s.settingsTheme || s.theme),
      cursorStyle: 'line',
      cursorBlinking: 'blink',
      contextmenu: false,
      overviewRulerLanes: 0,
      fixedOverflowWidgets: true,
      scrollbar: {
        vertical: s.scrollable ? 'auto' : 'hidden',
        horizontal: s.scrollable ? 'auto' : 'hidden',
        // Non-scrollable diff sections sit inside a virtualized scroll
        // container — let the wheel pass through instead of being swallowed.
        alwaysConsumeMouseWheel: s.scrollable,
      },
    })

    decorationCollectionRef.current = editor.createDecorationsCollection([])
    editorRef.current = editor
    setEditorReady(true)
    modelRef.current = model
    previousContentRef.current = content
    lastAppliedContentRef.current = content
    editorAPI.setTextareaRef(null)
    editorAPI.setViewportRef(container)

    const selectEntireModel = () => {
      editor.setSelection(model.getFullModelRange())
      editor.focus()
      syncCursorAndSelection()
    }

    editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyA, selectEntireModel)

    const disposables = [
      editor.onKeyDown((event) => {
        const browserEvent = event.browserEvent
        const isSelectAllShortcut =
          (browserEvent.metaKey || browserEvent.ctrlKey) &&
          !browserEvent.altKey &&
          !browserEvent.shiftKey &&
          browserEvent.key.toLowerCase() === 'a'

        if (!isSelectAllShortcut) return

        event.preventDefault()
        event.stopPropagation()
        selectEntireModel()
      }),
      editor.onDidChangeModelContent(() => {
        if (applyingExternalChangeRef.current) return
        const nextContent = model.getValue()
        const previousContent = previousContentRef.current
        const editorState = useEditorStateStore.getState()
        previousContentRef.current = nextContent
        latestContentChangeRef.current?.(
          nextContent,
          previousContent,
          editorState.cursorPosition,
          editorState.selection,
        )
        syncCursorAndSelection()
      }),
      editor.onDidChangeCursorSelection(syncCursorAndSelection),
      editor.onDidScrollChange((event) => {
        const viewKey = viewStateKey ?? activeBufferId ?? null
        setScrollForBuffer(viewKey, event.scrollTop, event.scrollLeft)
        latestOnScrollOffsetChangeRef.current?.(event.scrollTop, event.scrollLeft)
        updateVisibleLineRange(editor)
      }),
      editor.onDidLayoutChange((info) => {
        setViewportHeight(info.height)
        updateVisibleLineRange(editor)
      }),
    ]

    const unsubscribeCursor = editorAPI.on('cursorChange', (position) => {
      if (!modelRef.current || editorRef.current !== editor) return
      const monacoPosition = toClampedMonacoPosition(model, position)
      editor.setPosition(monacoPosition)
      editor.revealPositionInCenterIfOutsideViewport(monacoPosition)
    })
    const unsubscribeSelection = editorAPI.on('selectionChange', (selection) => {
      if (!modelRef.current || editorRef.current !== editor) return
      if (selection) {
        editor.setSelection(toMonacoRange(model, selection))
      } else {
        const position = editor.getPosition()
        if (position) {
          editor.setSelection(
            new MonacoRange(
              position.lineNumber,
              position.column,
              position.lineNumber,
              position.column,
            ),
          )
        }
      }
    })

    updateVisibleLineRange(editor)

    let layoutRafId: number | null = null
    let needsLayoutAfterResize = false
    const resizeObserver = new ResizeObserver(() => {
      if (document.documentElement.hasAttribute('data-pane-resizing')) {
        needsLayoutAfterResize = true
        return
      }
      if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
      layoutRafId = requestAnimationFrame(() => {
        layoutRafId = null
        editor.layout()
      })
    })
    resizeObserver.observe(container)

    const handlePaneResizeEnd = () => {
      if (!needsLayoutAfterResize) return
      needsLayoutAfterResize = false
      if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
      layoutRafId = requestAnimationFrame(() => {
        layoutRafId = null
        editor.layout()
      })
    }
    window.addEventListener('pane-resize-end', handlePaneResizeEnd)

    return () => {
      resizeObserver.disconnect()
      if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
      window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
      onCoordinateResolverChange?.(null)
      onModelPositionResolverChange?.(null)
      unsubscribeCursor()
      unsubscribeSelection()
      for (const disposable of disposables) {
        disposable.dispose()
      }
      if (editorRef.current === editor) editorRef.current = null
      if (modelRef.current === model) modelRef.current = null
      decorationCollectionRef.current = null
      setEditorReady(false)
      editor.dispose()
      model.dispose()
      editorAPI.setViewportRef(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    activeBufferId,
    filePath,
    isPreviewMode,
    modelUri,
    readOnly,
    setScrollForBuffer,
    setViewportHeight,
    viewStateKey,
  ])

  useEffect(() => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return

    monacoEditor.setModelLanguage(model, monacoLanguageId)
  }, [monacoLanguageId])

  // Register the editorAPI adapter only when this pane is the active surface.
  // Kept separate from the editor-creation effect so that focusing another pane
  // (isActiveSurface: true→false) does NOT destroy and recreate the Monaco
  // editor instance — which was resetting scroll position and cursor line.
  useEffect(() => {
    if (!isActiveSurface || readOnly || isPreviewMode) {
      editorAPI.clearActiveEditorAdapter(adapterOwnerId)
      return
    }

    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return

    const executeTextEdit = (range: Monaco.Range, text: string) => {
      const e = editorRef.current
      const m = modelRef.current
      if (!e || !m) return
      const startOffset = m.getOffsetAt(range.getStartPosition())
      e.pushUndoStop()
      e.executeEdits('crowbar-api', [{ range, text, forceMoveMarkers: true }])
      const nextPosition = m.getPositionAt(startOffset + text.length)
      e.setSelection(
        new MonacoRange(
          nextPosition.lineNumber,
          nextPosition.column,
          nextPosition.lineNumber,
          nextPosition.column,
        ),
      )
      e.setPosition(nextPosition)
      e.pushUndoStop()
      syncCursorAndSelection()
    }

    editorAPI.setActiveEditorAdapter({
      ownerId: adapterOwnerId,
      insertText: (text, position) => {
        const e = editorRef.current
        const m = modelRef.current
        if (!e || !m) return
        if (position) {
          const monacoPosition = toClampedMonacoPosition(m, position)
          executeTextEdit(
            new MonacoRange(
              monacoPosition.lineNumber,
              monacoPosition.column,
              monacoPosition.lineNumber,
              monacoPosition.column,
            ),
            text,
          )
          return
        }
        const selection = e.getSelection()
        if (selection && !selection.isEmpty()) {
          executeTextEdit(selection, text)
          return
        }
        const currentPosition = e.getPosition() ?? { lineNumber: 1, column: 1 }
        executeTextEdit(
          new MonacoRange(
            currentPosition.lineNumber,
            currentPosition.column,
            currentPosition.lineNumber,
            currentPosition.column,
          ),
          text,
        )
      },
      deleteRange: (range) => {
        const m = modelRef.current
        if (!m) return
        executeTextEdit(toMonacoRange(m, range), '')
      },
      replaceRange: (range, text) => {
        const m = modelRef.current
        if (!m) return
        executeTextEdit(toMonacoRange(m, range), text)
      },
      selectAll: () => {
        const e = editorRef.current
        if (!e) return
        const fullRange = e.getModel()?.getFullModelRange()
        if (fullRange) e.setSelection(fullRange)
        e.focus()
        syncCursorAndSelection()
      },
      undo: () => {
        editorRef.current?.trigger('crowbar-api', 'undo', null)
        syncCursorAndSelection()
      },
      redo: () => {
        editorRef.current?.trigger('crowbar-api', 'redo', null)
        syncCursorAndSelection()
      },
      canUndo: () => modelRef.current !== null,
      canRedo: () => modelRef.current !== null,
    })

    return () => {
      editorAPI.clearActiveEditorAdapter(adapterOwnerId)
    }
    // syncCursorAndSelection is a stable useCallback.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adapterOwnerId, isActiveSurface, isPreviewMode, readOnly])

  // External content → model sync.
  //
  // The buffer store's `content` can change for two reasons:
  //   (a) WE just pushed local typing through the synchronous change handler —
  //       the model ALREADY holds it.
  //   (b) A GENUINE external change: disk reload (external-buffer-sync),
  //       format-on-save, undo/redo applied to the store, etc. — the model must
  //       be updated to match.
  //
  // Two guards distinguish them: `lastAppliedContentRef === content` (we just
  // wrote this exact value) and the stronger `model.getValue() === content`
  // (the model already shows it). Only a value that survives BOTH is a genuine
  // external change worth applying.
  useEffect(() => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return
    // Cheap ref check first — skips model.getValue() (O(n) string alloc) when content hasn't changed
    if (lastAppliedContentRef.current === content) return
    if (model.getValue() === content) {
      lastAppliedContentRef.current = content
      return
    }

    applyingExternalChangeRef.current = true
    const selection = editor.getSelection()
    model.setValue(content)
    if (selection) editor.setSelection(selection)
    previousContentRef.current = content
    lastAppliedContentRef.current = content
    applyingExternalChangeRef.current = false
  }, [content, filePath])

  // Effect A: theme-only — re-runs only when the active theme changes.
  // Subscribes to themeRegistry so dynamic theme updates (palette changes) are picked up.
  // Kept separate so font/layout setting changes don't trigger defineMonacoTheme.
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return

    const applyTheme = () => monacoEditor.setTheme(defineMonacoTheme(settingsTheme || theme))
    applyTheme()

    const unsubscribeRegistry = themeRegistry.onRegistryChange(applyTheme)
    const unsubscribeTheme = themeRegistry.onThemeChange(applyTheme)

    return () => {
      unsubscribeRegistry()
      unsubscribeTheme()
    }
  }, [settingsTheme, theme])

  // Effect B: all non-theme editor options. Excludes theme so changing font/tabSize/wordWrap
  // does not trigger defineMonacoTheme or rebuild theme registry subscriptions.
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return

    editor.getModel()?.updateOptions({ tabSize })
    editor.updateOptions({
      fontFamily,
      fontSize,
      lineHeight,
      tabSize,
      readOnly: readOnly || isPreviewMode,
      domReadOnly: readOnly || isPreviewMode,
      lineNumbers: lineNumbers ? lineNumberFormatter : 'off',
      minimap: { enabled: minimapEnabled },
      renderWhitespace: renderWhitespace === 'none' ? 'none' : renderWhitespace,
      wordWrap: wordWrap ? 'on' : 'off',
      guides: {
        indentation: renderIndentGuides,
        highlightActiveIndentation: renderIndentGuides,
      },
      'semanticHighlighting.enabled': semanticHighlighting,
      occurrencesHighlight: highlightOccurrences ? 'singleFile' : 'off',
      selectionHighlight: true,
      quickSuggestions: autoCompletion,
      suggestOnTriggerCharacters: autoCompletion,
      parameterHints: { enabled: parameterHints },
      cursorStyle: 'line',
      cursorBlinking: 'blink',
      scrollbar: {
        vertical: scrollable ? 'auto' : 'hidden',
        horizontal: scrollable ? 'auto' : 'hidden',
        alwaysConsumeMouseWheel: scrollable,
      },
    })
  }, [
    autoCompletion,
    fontFamily,
    fontSize,
    highlightOccurrences,
    isPreviewMode,
    lineHeight,
    lineNumbers,
    lineNumberFormatter,
    minimapEnabled,
    parameterHints,
    readOnly,
    renderIndentGuides,
    renderWhitespace,
    scrollable,
    semanticHighlighting,
    tabSize,
    wordWrap,
  ])

  useEffect(() => {
    const collection = decorationCollectionRef.current
    const model = modelRef.current
    if (!collection || !model) return

    const matches = highlightMatches ?? searchMatches
    const activeIndex = currentHighlightIndex ?? currentSearchMatchIndex

    const decorations = matches.flatMap((match, index) => {
      const start = model.getPositionAt(match.start)
      const end = model.getPositionAt(match.end)
      return [
        {
          range: new MonacoRange(start.lineNumber, start.column, end.lineNumber, end.column),
          options: {
            className:
              index === activeIndex
                ? 'monaco-search-match monaco-search-match-current'
                : 'monaco-search-match',
            overviewRuler: undefined,
          },
        },
      ]
    })

    collection.set(decorations)
  }, [currentHighlightIndex, currentSearchMatchIndex, highlightMatches, searchMatches])

  useEffect(() => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) {
      onCoordinateResolverChange?.(null)
      onModelPositionResolverChange?.(null)
      return
    }

    onCoordinateResolverChange?.((clientX, clientY) => {
      if (model.isDisposed()) return null
      const target = editor.getTargetAtClientPoint(clientX, clientY)
      const position = target?.position
      if (!position) return null
      const editorPosition = toEditorPosition(model, position)
      const top = editor.getTopForLineNumber(position.lineNumber)
      const left = editor.getOffsetForColumn(position.lineNumber, position.column)
      return {
        ...editorPosition,
        viewLine: position.lineNumber - 1,
        modelLine: editorPosition.line,
        top,
        left,
        height: lineHeight,
        segment: {
          viewLine: position.lineNumber - 1,
          modelLine: editorPosition.line,
          startColumn: 0,
          endColumn: model.getLineLength(position.lineNumber),
          top,
          height: lineHeight,
        },
      }
    })

    onModelPositionResolverChange?.((line, column) => {
      if (model.isDisposed()) return null
      const position = clampMonacoPosition(model, {
        lineNumber: line + 1,
        column: column + 1,
      })
      let editorPosition: Position
      let top: number
      let left: number
      let lineLength: number

      try {
        editorPosition = toEditorPosition(model, position)
        top = editor.getTopForLineNumber(position.lineNumber)
        left = editor.getOffsetForColumn(position.lineNumber, position.column)
        lineLength = model.getLineLength(position.lineNumber)
      } catch (error) {
        if (model.isDisposed()) return null
        throw error
      }
      const modelLine = position.lineNumber - 1

      return {
        ...editorPosition,
        viewLine: modelLine,
        modelLine,
        top,
        left,
        height: lineHeight,
        segment: {
          viewLine: modelLine,
          modelLine,
          startColumn: 0,
          endColumn: lineLength,
          top,
          height: lineHeight,
        },
      }
    })

    return () => {
      onCoordinateResolverChange?.(null)
      onModelPositionResolverChange?.(null)
    }
  }, [lineHeight, onCoordinateResolverChange, onModelPositionResolverChange])

  // Restore scroll + cursor when this surface becomes active or the buffer
  // changes, and focus the editor when it becomes the active surface. This
  // legacy create-per-instance path reads the manual view-state cache (keyed by
  // viewStateKey/bufferId).
  useEffect(() => {
    const editor = editorRef.current
    if (!editor || !isActiveSurface) return

    const cached = useEditorStateStore
      .getState()
      .actions.getCachedViewState(viewStateKey ?? activeBufferId ?? '')
    if (cached) {
      editor.setScrollPosition({ scrollTop: cached.scrollTop, scrollLeft: cached.scrollLeft })
      const model = editor.getModel()
      if (model) {
        editor.setPosition(toClampedMonacoPosition(model, cached.cursor))
        if (cached.selection) editor.setSelection(toMonacoRange(model, cached.selection))
      }
    }

    if (!readOnly && !isPreviewMode) {
      const focusTimer = setTimeout(() => editorRef.current?.focus(), 0)
      return () => clearTimeout(focusTimer)
    }
  }, [activeBufferId, isActiveSurface, isPreviewMode, readOnly, viewStateKey])

  // Diff / virtual / read-only surfaces must stay out of the LSP document
  // lifecycle entirely — see shouldSkipLsp for why (path collides with a real
  // file whose true text differs from this surface's serialized diff content).
  const skipLsp = shouldSkipLsp(readOnly, isPreviewMode, buffer?.isVirtual ?? false)

  // LSP diagnostics: open the document so the server analyzes it, then paint
  // its diagnostics as Monaco markers (squiggles) for this file.
  useEffect(() => {
    if (skipLsp) return
    const model = modelRef.current
    if (!model || !filePath) return

    const client = LspClient.getInstance()
    void client.documentOpen(filePath, model.getValue(), languageId ?? 'plaintext')

    const applyMarkers = (fp: string, diagnostics: LspDiagnostic[]) => {
      if (!pathsMatch(fp, filePath)) return
      const current = modelRef.current
      if (!current) return
      monacoEditor.setModelMarkers(current, 'crowbar-lsp', diagnostics.map(toMonacoMarker))
    }
    const unsubscribe = client.onDiagnosticsUpdate(applyMarkers)

    return () => {
      unsubscribe()
      // Only a non-skipped effect ever reached documentOpen above, so the early
      // return guarantees we never documentClose a file we never opened.
      void client.documentClose(filePath)
      const current = modelRef.current
      if (current) monacoEditor.setModelMarkers(current, 'crowbar-lsp', [])
    }
  }, [filePath, languageId, skipLsp])

  // Re-analyze on edits (debounced — the server wants the full buffer text).
  useEffect(() => {
    if (skipLsp || !filePath) return
    const timer = setTimeout(() => {
      void LspClient.getInstance().documentChange(filePath, content)
    }, 400)
    return () => clearTimeout(timer)
  }, [content, filePath, skipLsp])

  const commentPortals = useDiffCommentZones({
    editorRef,
    editorReady,
    enabled: commentZones !== undefined,
    zones: commentZones ?? EMPTY_COMMENT_ZONES,
    onAddCommentAtLine,
  })

  // Report total content height (lines + view zones) so a fixed-height host can
  // grow to fit inline comment threads.
  useEffect(() => {
    const editor = editorRef.current
    if (!editorReady || !editor || !onContentHeightChange) return
    const report = () => onContentHeightChange(editor.getContentHeight())
    report()
    const sub = editor.onDidContentSizeChange(report)
    return () => sub.dispose()
  }, [editorReady, onContentHeightChange])

  // Diff add/removed tint as whole-line decorations (zone-safe, unlike a
  // position-based CSS overlay).
  const diffKindsKey = diffLineKinds?.join('') ?? null
  useEffect(() => {
    const editor = editorRef.current
    if (!editorReady || !editor || !diffLineKinds) return
    const decorations: Monaco.editor.IModelDeltaDecoration[] = []
    for (let i = 0; i < diffLineKinds.length; i++) {
      const kind = diffLineKinds[i]
      const className =
        kind === 'added' ? 'diff-line-added' : kind === 'removed' ? 'diff-line-removed' : null
      if (!className) continue
      const line = i + 1
      decorations.push({
        range: new MonacoRange(line, 1, line, 1),
        options: { isWholeLine: true, className },
      })
    }
    const collection = editor.createDecorationsCollection(decorations)
    return () => collection.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editorReady, diffKindsKey])

  // Diff-wide search: highlight every hit in this file, with the active hit
  // styled distinctly. Keyed on a structural signature so it only reruns when
  // the matches or the active hit actually change.
  const diffSearchMatchesKey = diffSearchMatches
    ? diffSearchMatches.map((m) => `${m.lineNumber}:${m.startColumn}:${m.endColumn}`).join('|')
    : ''
  const activeDiffMatchKey = activeDiffSearchMatch
    ? `${activeDiffSearchMatch.lineNumber}:${activeDiffSearchMatch.startColumn}:${activeDiffSearchMatch.endColumn}`
    : ''
  useEffect(() => {
    const editor = editorRef.current
    if (!editorReady || !editor || !diffSearchMatches || diffSearchMatches.length === 0) return
    const decorations: Monaco.editor.IModelDeltaDecoration[] = diffSearchMatches.map((m) => {
      const isActive =
        activeDiffSearchMatch != null &&
        m.lineNumber === activeDiffSearchMatch.lineNumber &&
        m.startColumn === activeDiffSearchMatch.startColumn &&
        m.endColumn === activeDiffSearchMatch.endColumn
      const cls = isActive ? 'diff-search-match-active' : 'diff-search-match'
      return {
        range: new MonacoRange(m.lineNumber, m.startColumn, m.lineNumber, m.endColumn),
        // inlineClassName highlights the matched text; className backs the range.
        options: { className: cls, inlineClassName: cls },
      }
    })
    const collection = editor.createDecorationsCollection(decorations)
    return () => collection.clear()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editorReady, diffSearchMatchesKey, activeDiffMatchKey])

  // Reveal the active hit. The diff editors are non-scrollable inside a
  // virtualized container, so scroll that container (not the editor) to bring
  // the matched line to the centre.
  useEffect(() => {
    if (!editorReady || !activeDiffSearchMatch) return
    const timer = setTimeout(() => {
      const editor = editorRef.current
      const dom = editor?.getDomNode()
      const scroller = dom?.closest('[data-diff-stack-scroll-container]') as HTMLElement | null
      if (!editor || !dom || !scroller) return
      const lineTop = editor.getTopForLineNumber(activeDiffSearchMatch.lineNumber)
      const editorTopInScroller =
        dom.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop
      const target = editorTopInScroller + lineTop - scroller.clientHeight / 2
      scroller.scrollTo({ top: Math.max(0, target), behavior: 'smooth' })
    }, 60)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editorReady, activeDiffMatchKey, diffSearchRevealNonce])

  if (!buffer) return null

  return (
    // Positioning shell around Monaco's own mount point (containerRef below).
    // Monaco renders its own hidden textarea (role="textbox") inside that
    // container — a fully keyboard-operable surface it owns and manages
    // itself, same as xterm's canvas. onClick here is a coordinate→line/column
    // mapping hook layered on top (for placing review comments in read-only
    // diffs) plus an optional forwarded callback, not this shell's own
    // interactive affordance — role="presentation" says so without touching
    // Monaco's own internal ARIA tree, which lives on different elements
    // further down this same subtree.
    <div
      role="presentation"
      className={`monaco-editor-shell absolute inset-0 min-h-0 bg-transparent ${className ?? ''}`}
      onMouseMove={onMouseMove}
      onMouseLeave={onMouseLeave}
      onMouseEnter={onMouseEnter}
      onClick={(event) => {
        if (readOnly && onReadonlySurfaceClick) {
          const editor = editorRef.current
          const model = modelRef.current
          const target = editor?.getTargetAtClientPoint(event.clientX, event.clientY)
          if (target?.position && model) {
            onReadonlySurfaceClick({
              line: target.position.lineNumber - 1,
              column: target.position.column - 1,
            })
          }
        }
        onClick?.(event)
      }}
    >
      {backgroundLayer}
      <div
        ref={containerRef}
        className="absolute inset-0"
        data-monaco-editor-scroll
        data-line-number-start={lineNumberStart}
        data-line-number-map={lineNumberMap?.length ?? undefined}
      />
      {commentPortals.map((p) => createPortal(p.node, p.contentNode))}
    </div>
  )
}

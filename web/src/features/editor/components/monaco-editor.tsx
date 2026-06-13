import '../monaco/monaco-environment'
import '../monaco/language-contributions'
import 'monaco-editor/min/vs/editor/editor.main.css'
import '../styles/monaco-editor.css'

import {
  editor as monacoEditor,
  KeyCode,
  KeyMod,
  MarkerSeverity,
  Range as MonacoRange,
  Uri,
} from 'monaco-editor'
import type * as Monaco from 'monaco-editor'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  type MouseEventHandler,
  type ReactNode,
} from 'react'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import type { ThemeDefinition } from '@/extensions/themes/types'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { ContentSink } from '@/features/editor/lib/content-sink'
import { useEditorSettingsStore } from '../stores/settings-store'
import { useEditorStateStore } from '../stores/state-store'
import { useEditorUIStore } from '../stores/ui-store'
import type { Position, Range } from '../types/editor'
import { getLanguageIdFromPath } from '../utils/language-id'
import { calculateLineHeight } from '../utils/lines'
import { editorAPI } from '../extensions/api'
import { LspClient, type LspDiagnostic } from '../lsp/lsp-client'
import type {
  EditorCoordinateResolver,
  EditorModelPositionResolver,
} from '../view-model/view-layout'
import { toMonacoLanguageId } from '../monaco/language'

interface MonacoBackedEditorProps {
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

function toEditorPosition(model: Monaco.editor.ITextModel, position: Monaco.IPosition): Position {
  return {
    line: position.lineNumber - 1,
    column: position.column - 1,
    offset: model.getOffsetAt(position),
  }
}

function toMonacoPosition(position: Position): Monaco.IPosition {
  return {
    lineNumber: position.line + 1,
    column: position.column + 1,
  }
}

function clampMonacoPosition(
  model: Monaco.editor.ITextModel,
  position: Monaco.IPosition,
): Monaco.IPosition {
  const lineNumber = Math.max(1, Math.min(model.getLineCount(), position.lineNumber))
  const maxColumn = model.getLineMaxColumn(lineNumber)
  const column = Math.max(1, Math.min(maxColumn, position.column))
  return { lineNumber, column }
}

function toClampedMonacoPosition(
  model: Monaco.editor.ITextModel,
  position: Position,
): Monaco.IPosition {
  return clampMonacoPosition(model, toMonacoPosition(position))
}

function toEditorRange(
  model: Monaco.editor.ITextModel,
  selection: Monaco.Selection,
): Range | undefined {
  if (selection.isEmpty()) return undefined

  const start = selection.getStartPosition()
  const end = selection.getEndPosition()
  return {
    start: toEditorPosition(model, start),
    end: toEditorPosition(model, end),
  }
}

function toMonacoRange(model: Monaco.editor.ITextModel, range: Range): Monaco.Range {
  let start = toClampedMonacoPosition(model, range.start)
  let end = toClampedMonacoPosition(model, range.end)
  if (
    start.lineNumber > end.lineNumber ||
    (start.lineNumber === end.lineNumber && start.column > end.column)
  ) {
    ;[start, end] = [end, start]
  }

  return new MonacoRange(start.lineNumber, start.column, end.lineNumber, end.column)
}

function severityToMonaco(severity: string): Monaco.MarkerSeverity {
  switch (severity.toLowerCase()) {
    case 'error':
      return MarkerSeverity.Error
    case 'warning':
      return MarkerSeverity.Warning
    case 'hint':
      return MarkerSeverity.Hint
    default:
      return MarkerSeverity.Info
  }
}

// Backend diagnostics use 0-based line/character; Monaco markers are 1-based.
function toMonacoMarker(diagnostic: LspDiagnostic): Monaco.editor.IMarkerData {
  return {
    severity: severityToMonaco(diagnostic.severity),
    message: diagnostic.message,
    source: diagnostic.source,
    code: diagnostic.code,
    startLineNumber: diagnostic.range.start.line + 1,
    startColumn: diagnostic.range.start.character + 1,
    endLineNumber: diagnostic.range.end.line + 1,
    endColumn: diagnostic.range.end.character + 1,
  }
}

// Diagnostic paths and buffer paths are both workspace-relative, but tolerate a
// leading-slash or absolute mismatch by comparing suffixes.
function pathsMatch(a: string, b: string): boolean {
  if (a === b) return true
  return a.endsWith(b) || b.endsWith(a)
}

function createModelUri(bufferId: string | undefined, filePath: string): Monaco.Uri {
  const sanitizedPath = filePath.replace(/^\/+/, '')
  const path = sanitizedPath.length > 0 ? sanitizedPath : `${bufferId ?? 'untitled'}.txt`
  return Uri.parse(`athas://editor/${encodeURIComponent(bufferId ?? path)}/${path}`)
}

function getThemeId(theme: string): string {
  return theme.includes('light') ? 'vs' : 'vs-dark'
}

function colorValue(theme: ThemeDefinition, name: string, fallback: string): string {
  return (
    theme.cssVariables?.[`--color-${name}`] ??
    theme.cssVariables?.[`--${name}`] ??
    (theme.syntaxTokens?.[`--color-${name}`] as string | undefined) ??
    (theme.syntaxTokens?.[`--${name}`] as string | undefined) ??
    fallback
  )
}

function stripHash(value: string): string {
  return value.startsWith('#') ? value.slice(1) : value
}

function toHexByte(value: number): string {
  return Math.max(0, Math.min(255, Math.round(value)))
    .toString(16)
    .padStart(2, '0')
}

function toMonacoColor(value: string, fallback: string): string {
  const normalized = value.trim()
  if (/^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/.test(normalized)) return normalized
  if (/^#[0-9a-fA-F]{3}$/.test(normalized)) {
    const [, r, g, b] = normalized
    return `#${r}${r}${g}${g}${b}${b}`
  }

  const rgbaMatch = normalized.match(
    /^rgba?\(\s*([.\d]+)\s*,\s*([.\d]+)\s*,\s*([.\d]+)(?:\s*,\s*([.\d]+)\s*)?\)$/i,
  )
  if (!rgbaMatch) return fallback

  const [, red, green, blue, alpha = '1'] = rgbaMatch
  const alphaByte = toHexByte(Number(alpha) * 255)
  return `#${toHexByte(Number(red))}${toHexByte(Number(green))}${toHexByte(Number(blue))}${alphaByte}`
}

function toMonacoThemeName(themeId: string): string {
  return `athas-${themeId.replace(/[^a-zA-Z0-9_-]/g, '-')}`
}

function syntaxTokenColor(theme: ThemeDefinition, token: string): string | undefined {
  return (
    (theme.syntaxTokens?.[`--color-syntax-${token}`] as string | undefined) ??
    (theme.syntaxTokens?.[`--syntax-${token}`] as string | undefined) ??
    (theme.syntaxTokens?.[`--color-${token}`] as string | undefined) ??
    (theme.syntaxTokens?.[`--${token}`] as string | undefined)
  )
}

function defineMonacoTheme(themeId: string): string {
  const theme = themeRegistry.getTheme(themeId)
  if (!theme) return getThemeId(themeId)

  const tokenMap: Array<[string, string]> = [
    ['comment', 'comment'],
    ['keyword', 'keyword'],
    ['string', 'string'],
    ['number', 'number'],
    ['regexp', 'regex'],
    ['function', 'function'],
    ['variable', 'variable'],
    ['constant', 'constant'],
    ['type', 'type'],
    ['class', 'type'],
    ['interface', 'type'],
    ['namespace', 'type'],
    ['tag', 'tag'],
    ['attribute.name', 'attribute'],
    ['delimiter', 'punctuation'],
    ['delimiter.bracket', 'punctuation'],
    ['operator', 'operator'],
    ['keyword.operator', 'operator'],
    ['keyword.json', 'property'],
    ['string.key.json', 'property'],
  ]

  const rules: Monaco.editor.ITokenThemeRule[] = tokenMap.flatMap(([token, syntaxName]) => {
    const foreground = syntaxTokenColor(theme, syntaxName)
    return foreground ? [{ token, foreground: stripHash(foreground) }] : []
  })

  const background = toMonacoColor(
    colorValue(theme, 'primary-bg', theme.isDark ? '#141413' : '#fcfcfd'),
    theme.isDark ? '#141413' : '#fcfcfd',
  )
  const foreground = toMonacoColor(
    colorValue(theme, 'text', theme.isDark ? '#faf9f5' : '#141413'),
    theme.isDark ? '#faf9f5' : '#141413',
  )
  const subtleForeground = toMonacoColor(
    colorValue(theme, 'text-lighter', theme.isDark ? '#b0aea5' : '#787d86'),
    theme.isDark ? '#b0aea5' : '#787d86',
  )
  const border = toMonacoColor(
    colorValue(theme, 'border', theme.isDark ? '#2f2d29' : '#e4e7ec'),
    theme.isDark ? '#2f2d29' : '#e4e7ec',
  )
  const selected = toMonacoColor(
    colorValue(theme, 'selected', theme.isDark ? '#2c2925' : '#e7ebf0'),
    theme.isDark ? '#2c2925' : '#e7ebf0',
  )
  const selection = toMonacoColor(
    colorValue(theme, 'selection-bg', 'rgba(106, 155, 204, 0.30)'),
    '#6a9bcc4d',
  )
  const accent = toMonacoColor(colorValue(theme, 'accent', '#4f8cff'), '#4f8cff')
  const cursor = toMonacoColor(colorValue(theme, 'cursor', foreground), foreground)

  const monacoThemeId = toMonacoThemeName(theme.id)
  monacoEditor.defineTheme(monacoThemeId, {
    base: theme.isDark ? 'vs-dark' : 'vs',
    inherit: true,
    rules,
    colors: {
      'editor.background': background,
      'editor.foreground': foreground,
      'editorCursor.foreground': cursor,
      'editor.selectionBackground': selection,
      'editor.inactiveSelectionBackground': selected,
      'editor.lineHighlightBackground': selected,
      'editorLineNumber.foreground': subtleForeground,
      'editorLineNumber.activeForeground': foreground,
      'editorIndentGuide.background1': border,
      'editorIndentGuide.activeBackground1': accent,
      'editorWhitespace.foreground': subtleForeground,
      'editor.findMatchBackground': selection,
      'editor.findMatchHighlightBackground': selected,
      'editorWidget.background': background,
      'editorWidget.foreground': foreground,
      'editorWidget.border': border,
      'editorSuggestWidget.background': background,
      'editorSuggestWidget.foreground': foreground,
      'editorSuggestWidget.border': border,
      'editorSuggestWidget.selectedBackground': selected,
      'input.background': background,
      'input.foreground': foreground,
      'input.border': border,
      focusBorder: accent,
    },
  })

  return monacoThemeId
}

export function MonacoBackedEditor({
  paneId: propPaneId,
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
}: MonacoBackedEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const editorRef = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null)
  const modelRef = useRef<Monaco.editor.ITextModel | null>(null)
  const applyingExternalChangeRef = useRef(false)
  const previousContentRef = useRef('')
  const decorationCollectionRef = useRef<Monaco.editor.IEditorDecorationsCollection | null>(null)
  const latestContentChangeRef = useRef(onContentChange)
  const workspaceStore = useWorkspaceStore()
  const editorManager = workspaceStore.editorManager
  // The retained per-pane widget (owned by EditorManager) is used ONLY for real
  // workspace panes, which always pass an explicit `paneId`. Standalone consumers
  // of this component — notably the git diff viewer, which renders MANY read-only
  // editors concurrently and shares source paths across split left/right surfaces —
  // do NOT pass a paneId. Those keep the legacy create-per-instance path because
  // (a) they would all collide on a single pane widget, and (b) the manager keys
  // models by file path, so split editors sharing a path would share one model.
  const useManagedWidget = !!propPaneId
  const resolvedPaneId = propPaneId ?? ''
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
  const baseFontSize = useEditorSettingsStore.use.fontSize()
  const fontFamily = useEditorSettingsStore.use.fontFamily()
  const editorLineHeight = useEditorSettingsStore.use.lineHeight()
  const tabSize = useEditorSettingsStore.use.tabSize()
  const wordWrap = useEditorSettingsStore.use.wordWrap()
  const lineNumbers = useEditorSettingsStore.use.lineNumbers()
  const renderWhitespace = useEditorSettingsStore.use.renderWhitespace()
  const renderIndentGuides = useEditorSettingsStore.use.renderIndentGuides()
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

  const updateVisibleLineRange = useCallback(
    (editor: Monaco.editor.IStandaloneCodeEditor) => {
      const visibleRanges = editor.getVisibleRanges()
      const firstRange = visibleRanges[0]
      const lastRange = visibleRanges[visibleRanges.length - 1] ?? firstRange
      if (!firstRange || !lastRange) return

      latestOnVisibleLineRangeChangeRef.current?.({
        startLine: Math.max(0, firstRange.startLineNumber - 1 - 30),
        endLine: Math.max(0, lastRange.endLineNumber - 1 + 30),
      })
    },
    [],
  )

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

  // ── Mount effect ────────────────────────────────────────────────────────
  // Mounts the pane's RETAINED Monaco widget (owned by the EditorManager) into
  // this container exactly once per (pane, container) lifetime. The widget is
  // NOT created/destroyed on tab switches — those are handled as model swaps by
  // the controller effect below. ResizeObserver + pane-resize-end drive layout
  // through the manager (the widget runs with automaticLayout:false).
  useEffect(() => {
    if (!useManagedWidget) return
    const container = containerRef.current
    if (!container) return

    editorManager.mountPane(resolvedPaneId, container)
    editorAPI.setTextareaRef(null)
    editorAPI.setViewportRef(container)

    const raw = editorManager.getRawEditor(resolvedPaneId) as
      | Monaco.editor.IStandaloneCodeEditor
      | null

    // Apply the active theme to the freshly-created widget. (Subsequent theme
    // changes are handled by the theme effect below.)
    const s = latestEditorSettingsRef.current
    raw?.updateOptions({ theme: defineMonacoTheme(s.settingsTheme || s.theme) })

    // Select-all command + Cmd/Ctrl-A keybinding. Registered once on the
    // retained widget (addCommand returns an id, not a disposable, so it must
    // NOT be re-registered per tab switch). Targets the widget's CURRENT model.
    const selectEntireModel = () => {
      const ed = editorManager.getRawEditor(resolvedPaneId) as
        | Monaco.editor.IStandaloneCodeEditor
        | null
      const m = ed?.getModel()
      if (!ed || !m) return
      ed.setSelection(m.getFullModelRange())
      ed.focus()
      syncCursorAndSelection()
    }
    raw?.addCommand(KeyMod.CtrlCmd | KeyCode.KeyA, selectEntireModel)
    const keyDownDisposable = raw?.onKeyDown((event) => {
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
    })

    // rAF-debounced ResizeObserver — fires layout once per resize burst.
    // During a pane or sidebar drag (data-pane-resizing attribute), layout is
    // suppressed; a single layout runs when pane-resize-end fires.
    let layoutRafId: number | null = null
    let needsLayoutAfterResize = false
    const runLayout = () => editorManager.layoutPane(resolvedPaneId)
    const resizeObserver = new ResizeObserver(() => {
      if (document.documentElement.hasAttribute('data-pane-resizing')) {
        needsLayoutAfterResize = true
        return
      }
      if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
      layoutRafId = requestAnimationFrame(() => {
        layoutRafId = null
        runLayout()
      })
    })
    resizeObserver.observe(container)

    const handlePaneResizeEnd = () => {
      if (!needsLayoutAfterResize) return
      needsLayoutAfterResize = false
      if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
      layoutRafId = requestAnimationFrame(() => {
        layoutRafId = null
        runLayout()
      })
    }
    window.addEventListener('pane-resize-end', handlePaneResizeEnd)

    return () => {
      resizeObserver.disconnect()
      if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
      window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
      keyDownDisposable?.dispose()
      editorRef.current = null
      modelRef.current = null
      decorationCollectionRef.current = null
      editorAPI.setViewportRef(null)
      editorManager.unmountPane(resolvedPaneId)
    }
    // syncCursorAndSelection is a stable useCallback; settings/theme are read via
    // refs so the retained widget is mounted exactly once per (pane, container).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [useManagedWidget, editorManager, resolvedPaneId])

  // ── Controller effect ───────────────────────────────────────────────────
  // Swaps the model into the retained widget for the active editor buffer and
  // (re)binds content/cursor/scroll listeners + the editorAPI cursor/selection
  // subscriptions to that widget's current model. NO editor/model creation or
  // disposal happens here — the manager owns the widget and the ModelRegistry
  // owns the models. Re-runs on buffer change (model swap), never on settings.
  useEffect(() => {
    if (!useManagedWidget) return
    if (!buffer) return
    const container = containerRef.current
    if (!container) return

    editorManager.showBuffer(resolvedPaneId, fileUri(filePath))
    const editor = editorManager.getRawEditor(resolvedPaneId) as
      | Monaco.editor.IStandaloneCodeEditor
      | null
    const model = editor?.getModel() ?? null
    if (!editor || !model) return

    // Keep the model's tab settings in sync on swap (settings effect also runs).
    model.updateOptions({ tabSize: latestEditorSettingsRef.current.tabSize, insertSpaces: true })

    decorationCollectionRef.current = editor.createDecorationsCollection([])
    editorRef.current = editor
    modelRef.current = model
    previousContentRef.current = content
    lastAppliedContentRef.current = content

    // Model-authoritative content: the Monaco model is the source of truth for
    // text. Keystrokes are coalesced through a trailing-debounce ContentSink and
    // written to the Zustand buffer store fire-and-forget (no synchronous
    // store→setValue round-trip per keystroke). The sink's write is the single
    // place that forwards to onContentChange and advances the "last value we
    // pushed" markers so the external-change effect below can tell our own
    // writes apart from genuine external changes.
    const sink = new ContentSink({
      delayMs: 150,
      write: (value) => {
        const previousContent = previousContentRef.current
        const editorState = useEditorStateStore.getState()
        previousContentRef.current = value
        lastAppliedContentRef.current = value
        latestContentChangeRef.current?.(
          value,
          previousContent,
          editorState.cursorPosition,
          editorState.selection,
        )
      },
    })

    // The FIRST edit after a (re)bind is flushed synchronously so the dirty
    // indicator and preview-tab promote-on-first-edit fire immediately rather
    // than after the trailing window. Sustained typing thereafter is throttled.
    let firstEditFlushed = false

    const disposables = [
      editor.onDidChangeModelContent(() => {
        if (applyingExternalChangeRef.current) return
        if (editor.getModel() !== model) return
        // Throttled, fire-and-forget write of the model text to the store.
        sink.push(model.getValue())
        if (!firstEditFlushed) {
          firstEditFlushed = true
          sink.flush()
        }
        // Cursor/selection drive the status bar — keep them synchronous.
        syncCursorAndSelection()
      }),
      editor.onDidBlurEditorText(() => sink.flush()),
      editor.onDidChangeCursorSelection(syncCursorAndSelection),
      // Managed panes rely on Monaco-native view-state (saved/restored by the
      // EditorManager on model swap), so we do NOT write scroll to the manual
      // view-state cache here. We still forward the offset for LSP overlay sync
      // and recompute the visible line range.
      editor.onDidScrollChange((event) => {
        latestOnScrollOffsetChangeRef.current?.(event.scrollTop, event.scrollLeft)
        updateVisibleLineRange(editor)
      }),
      editor.onDidLayoutChange((info) => {
        setViewportHeight(info.height)
        updateVisibleLineRange(editor)
      }),
    ]

    const unsubscribeCursor = editorAPI.on('cursorChange', (position) => {
      if (editorRef.current !== editor || editor.getModel() !== model) return
      const monacoPosition = toClampedMonacoPosition(model, position)
      editor.setPosition(monacoPosition)
      editor.revealPositionInCenterIfOutsideViewport(monacoPosition)
    })
    const unsubscribeSelection = editorAPI.on('selectionChange', (selection) => {
      if (editorRef.current !== editor || editor.getModel() !== model) return
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

    return () => {
      // Persist any pending throttled edit before we tear down listeners, then
      // drop the timer — covers switching tabs / unmounting mid-typing so the
      // last keystrokes are never lost.
      sink.flush()
      sink.dispose()
      onCoordinateResolverChange?.(null)
      onModelPositionResolverChange?.(null)
      unsubscribeCursor()
      unsubscribeSelection()
      for (const disposable of disposables) {
        disposable.dispose()
      }
      if (decorationCollectionRef.current) {
        decorationCollectionRef.current.clear()
        decorationCollectionRef.current = null
      }
      // NOTE: the editor + model are owned by the EditorManager / ModelRegistry
      // and intentionally NOT disposed here. Tab switches are model swaps.
    }
    // Intentionally narrow deps: settings (fontFamily, fontSize, theme, wordWrap, etc.) are
    // handled by the updateOptions effect below — including them here would re-bind listeners
    // on every settings change. isActiveSurface is excluded deliberately. Adapter registration
    // and auto-focus are handled by the effects below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    useManagedWidget,
    editorManager,
    resolvedPaneId,
    activeBufferId,
    filePath,
    isPreviewMode,
    readOnly,
    setViewportHeight,
    viewStateKey,
  ])

  // ── Legacy creation effect (non-managed / standalone consumers) ──────────
  // Used when no `paneId` is supplied (git diff viewer, etc.). Creates a private
  // Monaco editor + model per instance and disposes them on unmount — the exact
  // behavior that shipped before the retained-widget refactor. Pane editors use
  // the manager path above instead.
  useEffect(() => {
    if (useManagedWidget) return
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
      occurrencesHighlight: s.highlightOccurrences ? 'singleFile' : 'off',
      selectionHighlight: s.highlightOccurrences,
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
      },
    })

    decorationCollectionRef.current = editor.createDecorationsCollection([])
    editorRef.current = editor
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
      editor.dispose()
      model.dispose()
      editorAPI.setViewportRef(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    useManagedWidget,
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
      e.executeEdits('athas-api', [{ range, text, forceMoveMarkers: true }])
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
        editorRef.current?.trigger('athas-api', 'undo', null)
        syncCursorAndSelection()
      },
      redo: () => {
        editorRef.current?.trigger('athas-api', 'redo', null)
        syncCursorAndSelection()
      },
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
  //   (a) WE just pushed local typing through the ContentSink (managed) or the
  //       synchronous change handler (legacy) — the model ALREADY holds it.
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

    if (useManagedWidget) {
      // Managed panes are model-authoritative: do NOT round-trip local typing
      // through setValue. Genuine external changes are applied via an undo-
      // friendly pushEditOperations edit on the held model (preserves the undo
      // stack, unlike setValue). Guard so our own edit doesn't re-enter the
      // change handler and bounce back to the store.
      applyingExternalChangeRef.current = true
      const selection = editor.getSelection()
      editorManager.applyExternalEdit(resolvedPaneId, fileUri(filePath), content)
      if (selection) editor.setSelection(selection)
      previousContentRef.current = content
      lastAppliedContentRef.current = content
      applyingExternalChangeRef.current = false
      return
    }

    applyingExternalChangeRef.current = true
    const selection = editor.getSelection()
    model.setValue(content)
    if (selection) editor.setSelection(selection)
    previousContentRef.current = content
    lastAppliedContentRef.current = content
    applyingExternalChangeRef.current = false
  }, [content, useManagedWidget, editorManager, resolvedPaneId, filePath])

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
      occurrencesHighlight: highlightOccurrences ? 'singleFile' : 'off',
      selectionHighlight: highlightOccurrences,
      quickSuggestions: autoCompletion,
      suggestOnTriggerCharacters: autoCompletion,
      parameterHints: { enabled: parameterHints },
      cursorStyle: 'line',
      cursorBlinking: 'blink',
      scrollbar: {
        vertical: scrollable ? 'auto' : 'hidden',
        horizontal: scrollable ? 'auto' : 'hidden',
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

  // Restore scroll + cursor when this surface becomes active or the buffer changes,
  // and focus the editor when it becomes the active surface.
  //
  // Managed panes do NOT restore from the manual view-state cache here: the
  // EditorManager already saves/restores Monaco-native view-state on every model
  // swap (keyed by paneId+uri), which covers scroll and cursor/selection. Only the
  // legacy create-per-instance path (no paneId) reads the manual cache. Focus is
  // applied for both paths when the surface becomes active.
  useEffect(() => {
    const editor = editorRef.current
    if (!editor || !isActiveSurface) return

    if (!useManagedWidget) {
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
    }

    if (!readOnly && !isPreviewMode) {
      setTimeout(() => editorRef.current?.focus(), 0)
    }
  }, [activeBufferId, isActiveSurface, isPreviewMode, readOnly, useManagedWidget, viewStateKey])

  // LSP diagnostics: open the document so the server analyzes it, then paint
  // its diagnostics as Monaco markers (squiggles) for this file.
  useEffect(() => {
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
      void client.documentClose(filePath)
      const current = modelRef.current
      if (current) monacoEditor.setModelMarkers(current, 'crowbar-lsp', [])
    }
  }, [filePath, languageId])

  // Re-analyze on edits (debounced — the server wants the full buffer text).
  useEffect(() => {
    if (!filePath) return
    const timer = setTimeout(() => {
      void LspClient.getInstance().documentChange(filePath, content)
    }, 400)
    return () => clearTimeout(timer)
  }, [content, filePath])

  if (!buffer) return null

  return (
    <div
      className={`monaco-editor-shell absolute inset-0 min-h-0 bg-background ${className ?? ''}`}
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
    </div>
  )
}

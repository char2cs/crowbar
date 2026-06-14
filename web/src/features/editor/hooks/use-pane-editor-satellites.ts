/**
 * usePaneEditorSatellites — per-pane retained-editor "satellite" concerns.
 *
 * The core buffer switch is driven imperatively by {@link usePaneEditorController}
 * (model swap + content/cursor seam). THIS hook owns everything else the old
 * `monaco-editor.tsx` managed path attached to the retained widget, WITHOUT
 * remounting on a tab switch:
 *
 *  - Widget-level, bound once per pane (read the editor's CURRENT model):
 *      • settings `updateOptions` (font, tabSize, wordWrap, minimap, …)
 *      • theme (`setTheme` + themeRegistry subscriptions)
 *      • editorAPI cursor/selection adapter (insert/delete/replace/undo/redo)
 *      • scroll-offset forwarding, layout (viewport height) + visible line range
 *  - Model-dependent, rebound on each swap via the active-editor registry:
 *      • decorations collection (search-match highlights)
 *      • coordinate / model-position resolvers (LSP overlays)
 *      • external-change → model sync (disk reload / format-on-save / undo-redo)
 *      • LSP diagnostics document lifecycle + markers
 *      • language id sync
 *
 * It subscribes to the active-editor registry for `{ editor, model, filePath }`
 * so the model-dependent pieces retarget on swap; the widget-level pieces read
 * `editorRef`/`modelRef` (kept current by the same subscription) so they survive
 * swaps without re-running.
 */

import type React from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  editor as monacoEditor,
  KeyCode,
  KeyMod,
  Range as MonacoRange,
} from 'monaco-editor'
import type * as Monaco from 'monaco-editor'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { useEditorSettingsStore } from '../stores/settings-store'
import { useEditorStateStore } from '../stores/state-store'
import { useEditorUIStore } from '../stores/ui-store'
import type { Position } from '../types/editor'
import { getLanguageIdFromPath } from '../utils/language-id'
import { calculateLineHeight } from '../utils/lines'
import { editorAPI } from '../extensions/api'
import { LspClient, type LspDiagnostic } from '../lsp/lsp-client'
import type {
  EditorCoordinateResolver,
  EditorModelPositionResolver,
} from '../view-model/view-layout'
import { toMonacoLanguageId } from '../monaco/language'
import {
  toEditorPosition,
  toEditorRange,
  toMonacoRange,
  toClampedMonacoPosition,
  clampMonacoPosition,
  toMonacoMarker,
  pathsMatch,
} from '../monaco/editor-conversions'
import { defineMonacoTheme } from '../monaco/define-theme'

type StandaloneEditor = Monaco.editor.IStandaloneCodeEditor

export interface PaneEditorSatelliteDeps {
  highlightMatches?: Array<{ start: number; end: number }>
  currentHighlightIndex?: number
  lineNumberStart?: number
  lineNumberMap?: Array<number | null>
  onScrollOffsetChange?: (scrollTop: number, scrollLeft: number) => void
  onCoordinateResolverChange?: (resolver: EditorCoordinateResolver | null) => void
  onModelPositionResolverChange?: (resolver: EditorModelPositionResolver | null) => void
  readOnly?: boolean
  scrollable?: boolean
  isActiveSurface?: boolean
  /**
   * Shared latch the surface reads in its content-change write seam to ignore
   * the model-change event that a GENUINE external edit (applied here via the
   * manager) re-fires — preventing it from bouncing back to the buffer store.
   * Set to the applied text immediately before the edit.
   */
  externalApplyRef?: React.MutableRefObject<string | null>
}

/**
 * Bind the retained widget's satellite concerns for `paneId`. The retained
 * editor + active model are sourced from the active-editor registry (published
 * by the controller on every swap), so this hook never reads `activeBufferId`
 * through React render and never remounts the widget.
 */
export function usePaneEditorSatellites(
  paneId: string,
  deps: PaneEditorSatelliteDeps,
): void {
  const {
    highlightMatches,
    currentHighlightIndex,
    lineNumberStart,
    lineNumberMap,
    onScrollOffsetChange,
    onCoordinateResolverChange,
    onModelPositionResolverChange,
    readOnly = false,
    scrollable = true,
    isActiveSurface = true,
    externalApplyRef,
  } = deps

  const workspaceStore = useWorkspaceStore()
  const registry = workspaceStore.activeEditorRegistry
  const editorManager = workspaceStore.editorManager

  // Narrow selectors: the active editor buffer's content + language override for
  // THIS pane. Each returns a PRIMITIVE so the snapshot is referentially stable
  // (returning a fresh object here would make useSyncExternalStore re-render
  // every commit → "Maximum update depth exceeded"). Only the active buffer's
  // text changes re-run external-sync/LSP.
  const activeContent = useWorkspaceStoreContext(
    useCallback(
      (state) => {
        const bufferId = state.panes[paneId]?.activeBufferId ?? null
        const buffer = bufferId
          ? state.buffers.find((candidate) => candidate.id === bufferId)
          : null
        return buffer && hasTextContent(buffer) ? buffer.content : ''
      },
      [paneId],
    ),
  )
  const languageOverride = useWorkspaceStoreContext(
    useCallback(
      (state) => {
        const bufferId = state.panes[paneId]?.activeBufferId ?? null
        const buffer = bufferId
          ? state.buffers.find((candidate) => candidate.id === bufferId)
          : null
        return buffer && hasTextContent(buffer) && 'languageOverride' in buffer
          ? buffer.languageOverride
          : undefined
      },
      [paneId],
    ),
  )

  // The retained editor + its current model, kept fresh by the registry
  // subscription below. Widget-level effects read these refs; model-dependent
  // effects re-run via the `swapVersion` state bumped on each swap.
  const editorRef = useRef<StandaloneEditor | null>(null)
  const modelRef = useRef<Monaco.editor.ITextModel | null>(null)
  const filePathRef = useRef('')
  const decorationCollectionRef =
    useRef<Monaco.editor.IEditorDecorationsCollection | null>(null)

  // ── Settings (latest in a ref; applied by the updateOptions effect) ───────
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
  const { setCursorPosition, setSelection, setViewportHeight } =
    useEditorStateStore.use.actions()
  const searchMatches = useEditorUIStore.use.searchMatches()
  const currentSearchMatchIndex = useEditorUIStore.use.currentMatchIndex()

  const fontSize = baseFontSize * zoomLevel
  const lineHeight = calculateLineHeight(fontSize, editorLineHeight)

  const lineNumberFormatter = useCallback(
    (lineNumber: number) => {
      const mappedLine = lineNumberMap?.[lineNumber - 1]
      if (typeof mappedLine === 'number') return String(mappedLine)
      return String((lineNumberStart ?? 1) + lineNumber - 1)
    },
    [lineNumberMap, lineNumberStart],
  )

  // Latest-callback refs (read inside once-bound listeners).
  const latestOnScrollOffsetChangeRef = useRef(onScrollOffsetChange)
  latestOnScrollOffsetChangeRef.current = onScrollOffsetChange
  const onCoordinateResolverChangeRef = useRef(onCoordinateResolverChange)
  onCoordinateResolverChangeRef.current = onCoordinateResolverChange
  const onModelPositionResolverChangeRef = useRef(onModelPositionResolverChange)
  onModelPositionResolverChangeRef.current = onModelPositionResolverChange

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
  const syncCursorAndSelection = useCallback(
    () => syncCursorAndSelectionRef.current(),
    [],
  )

  const updateVisibleLineRange = useCallback((editor: StandaloneEditor) => {
    const visibleRanges = editor.getVisibleRanges()
    const firstRange = visibleRanges[0]
    const lastRange = visibleRanges[visibleRanges.length - 1] ?? firstRange
    if (!firstRange || !lastRange) return
    // Reserved for future overlay virtualization; kept for parity.
    void firstRange
    void lastRange
  }, [])

  // ── Registry subscription: keep editor/model refs current + retarget ──────
  // Bumps `swapTick` to re-run the model-dependent effects on each swap.
  const [swapTick, setSwapTick] = useState(0)
  useEffect(() => {
    const unsubscribe = registry.subscribe(paneId, (ctx) => {
      editorRef.current = (ctx?.editor as StandaloneEditor | undefined) ?? null
      modelRef.current = (ctx?.model as Monaco.editor.ITextModel | undefined) ?? null
      filePathRef.current = ctx?.filePath ?? ''
      setSwapTick((t) => t + 1)
    })
    return unsubscribe
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [paneId])

  // ── Once-per-pane: select-all command + scroll/layout/visible-range ───────
  // Bound when the editor first becomes available; reads the CURRENT model.
  const boundEditorRef = useRef<StandaloneEditor | null>(null)
  useEffect(() => {
    const editor = editorRef.current
    if (!editor || boundEditorRef.current === editor) return
    boundEditorRef.current = editor

    const selectEntireModel = () => {
      const ed = editorRef.current
      const m = ed?.getModel()
      if (!ed || !m) return
      ed.setSelection(m.getFullModelRange())
      ed.focus()
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
      editor.onDidScrollChange((event) => {
        latestOnScrollOffsetChangeRef.current?.(event.scrollTop, event.scrollLeft)
        updateVisibleLineRange(editor)
      }),
      editor.onDidLayoutChange((info) => {
        setViewportHeight(info.height)
        updateVisibleLineRange(editor)
      }),
    ]

    return () => {
      for (const d of disposables) d.dispose()
      if (boundEditorRef.current === editor) boundEditorRef.current = null
    }
    // syncCursorAndSelection is stable; re-run only when the editor instance appears.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [swapTick, setViewportHeight, updateVisibleLineRange])

  // ── Per-swap: decorations collection bound to the live model ──────────────
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    decorationCollectionRef.current = editor.createDecorationsCollection([])
    return () => {
      decorationCollectionRef.current?.clear()
      decorationCollectionRef.current = null
    }
  }, [swapTick])

  // ── editorAPI adapter (only while this surface is active) ─────────────────
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const adapterOwnerId = useMemo(() => `${paneId}:${filePathRef.current}`, [paneId, swapTick])
  useEffect(() => {
    if (!isActiveSurface || readOnly) {
      editorAPI.clearActiveEditorAdapter(adapterOwnerId)
      return
    }
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return

    editorAPI.setTextareaRef(null)

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

    return () => editorAPI.clearActiveEditorAdapter(adapterOwnerId)
    // syncCursorAndSelection is stable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adapterOwnerId, isActiveSurface, readOnly, swapTick])

  // ── Focus on swap / when this surface becomes active ──────────────────────
  useEffect(() => {
    const editor = editorRef.current
    if (!editor || !isActiveSurface) return
    if (!readOnly) {
      setTimeout(() => editorRef.current?.focus(), 0)
    }
  }, [isActiveSurface, readOnly, swapTick])

  // ── Language id sync (per-swap; also covers languageOverride changes) ─────
  const languageId = languageOverride ?? getLanguageIdFromPath(filePathRef.current)
  const monacoLanguageId = toMonacoLanguageId(languageId)
  useEffect(() => {
    const model = modelRef.current
    if (!model) return
    monacoEditor.setModelLanguage(model, monacoLanguageId)
  }, [monacoLanguageId, swapTick])

  // ── External content → model sync ─────────────────────────────────────────
  // Managed panes are model-authoritative; only GENUINE external changes (disk
  // reload, format-on-save, undo/redo applied to the store) are pushed into the
  // held model via the manager's undo-friendly edit.
  useEffect(() => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return
    const path = filePathRef.current
    if (!path) return
    if (model.getValue() === activeContent) return
    const selection = editor.getSelection()
    // Latch the applied text so the surface ignores the model-change event this
    // edit re-fires (otherwise it would bounce straight back to the store).
    if (externalApplyRef) externalApplyRef.current = activeContent
    editorManager.applyExternalEdit(paneId, fileUri(path), activeContent)
    if (selection) editor.setSelection(selection)
  }, [activeContent, editorManager, externalApplyRef, paneId, swapTick])

  // ── Settings: theme (separate so font/layout changes don't redefine theme) ─
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    const applyTheme = () =>
      monacoEditor.setTheme(defineMonacoTheme(settingsTheme || theme))
    applyTheme()
    const unsubscribeRegistry = themeRegistry.onRegistryChange(applyTheme)
    const unsubscribeTheme = themeRegistry.onThemeChange(applyTheme)
    return () => {
      unsubscribeRegistry()
      unsubscribeTheme()
    }
  }, [settingsTheme, theme, swapTick])

  // ── Settings: all non-theme editor options ────────────────────────────────
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    editor.getModel()?.updateOptions({ tabSize, insertSpaces: true })
    editor.updateOptions({
      fontFamily,
      fontSize,
      lineHeight,
      tabSize,
      readOnly,
      domReadOnly: readOnly,
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
    swapTick,
  ])

  // ── Search-match decorations ──────────────────────────────────────────────
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
  }, [
    currentHighlightIndex,
    currentSearchMatchIndex,
    highlightMatches,
    searchMatches,
    swapTick,
  ])

  // ── Coordinate / model-position resolvers (LSP overlays) ──────────────────
  useEffect(() => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) {
      onCoordinateResolverChangeRef.current?.(null)
      onModelPositionResolverChangeRef.current?.(null)
      return
    }

    onCoordinateResolverChangeRef.current?.((clientX, clientY) => {
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

    onModelPositionResolverChangeRef.current?.((line, column) => {
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
      onCoordinateResolverChangeRef.current?.(null)
      onModelPositionResolverChangeRef.current?.(null)
    }
  }, [lineHeight, swapTick])

  // ── LSP diagnostics: open document + paint markers ────────────────────────
  useEffect(() => {
    const model = modelRef.current
    const filePath = filePathRef.current
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
  }, [languageId, swapTick])

  // ── LSP re-analyze on edits (debounced) ───────────────────────────────────
  useEffect(() => {
    const filePath = filePathRef.current
    if (!filePath) return
    const timer = setTimeout(() => {
      void LspClient.getInstance().documentChange(filePath, activeContent)
    }, 400)
    return () => clearTimeout(timer)
  }, [activeContent, swapTick])
}

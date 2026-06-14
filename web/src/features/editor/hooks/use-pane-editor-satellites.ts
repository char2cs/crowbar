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
import { scheduleIdleTask } from '../lib/idle-task'
import { createRafCoalescer, type RafCoalescer } from '../lib/raf-coalesce'

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

  // Active buffer CONTENT is read IMPERATIVELY (U5b) — NOT subscribed into
  // render. A render subscription here re-rendered EditorSurface on every
  // keystroke (content flows model → sink → store every ~150ms). Instead a
  // vanilla `workspaceStore.subscribe` (below) watches THIS pane's active-buffer
  // text and drives the external-sync + LSP-didChange effects off-render,
  // reading the new content + model imperatively when it actually changes.
  const readActiveContent = useCallback(() => {
    const state = workspaceStore.getState()
    const bufferId = state.panes[paneId]?.activeBufferId ?? null
    const buffer = bufferId
      ? state.buffers.find((candidate) => candidate.id === bufferId)
      : null
    return buffer && hasTextContent(buffer) ? buffer.content : ''
  }, [workspaceStore, paneId])

  // `languageOverride` changes RARELY (a manual language pick), so it stays a
  // render subscription — it feeds `setModelLanguage` + the LSP document
  // lifecycle, both keyed on `languageId`/`swapTick`, not on keystrokes. It
  // returns a PRIMITIVE so the snapshot is referentially stable.
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
  const { setCursorAndSelection, setViewportHeight } =
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

  // Cursor/selection sync is rAF-COALESCED: the editorAPI adapter actions
  // (select-all/undo/redo/insert/…) call `syncCursorAndSelection()`, which only
  // schedules a single trailing frame that reads the editor's CURRENT
  // position+selection once and writes them in ONE batched store update.
  const flushCursorSyncRef = useRef<() => void>(() => {})
  flushCursorSyncRef.current = () => {
    const editor = editorRef.current
    const model = modelRef.current
    if (!editor || !model) return
    const position = editor.getPosition()
    if (!position) return
    const selection = editor.getSelection()
    setCursorAndSelection(
      toEditorPosition(model, position),
      selection ? toEditorRange(model, selection) : undefined,
      { ensureVisible: false },
    )
  }
  const cursorSyncerRef = useRef<RafCoalescer | null>(null)
  if (!cursorSyncerRef.current) {
    cursorSyncerRef.current = createRafCoalescer(() => flushCursorSyncRef.current())
  }
  useEffect(() => {
    const syncer = cursorSyncerRef.current
    return () => syncer?.cancel()
  }, [])
  const syncCursorAndSelection = useCallback(
    () => cursorSyncerRef.current?.schedule(),
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

  // ── Imperative active-content change signal (U5b) ──────────────────────────
  // A single vanilla store subscription watches THIS pane's active-buffer text
  // and fans out to the two content-driven concerns (external→model sync, LSP
  // didChange) WITHOUT re-rendering. The latest content is mirrored into
  // `activeContentRef`; subscribers register a callback that fires only when the
  // content actually changes. This is rebound on swap (refs/effects re-key on
  // `swapTick`), but the subscription itself is paneId-scoped and reads the live
  // active buffer, so it survives swaps.
  const activeContentRef = useRef('')
  activeContentRef.current = readActiveContent()
  const externalSyncRef = useRef<(content: string) => void>(() => {})
  const lspDidChangeRef = useRef<(content: string) => void>(() => {})
  useEffect(() => {
    let previous = readActiveContent()
    activeContentRef.current = previous
    // Apply any change that landed between render and this effect's commit.
    externalSyncRef.current(previous)
    return workspaceStore.subscribe(() => {
      const next = readActiveContent()
      if (next === previous) return
      previous = next
      activeContentRef.current = next
      externalSyncRef.current(next)
      lspDidChangeRef.current(next)
    })
  }, [workspaceStore, readActiveContent])

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

  // ── Per-swap MODEL-level options: language + tabSize ──────────────────────
  // These are the ONLY things that legitimately must reapply on a model swap
  // (a fresh model has default language + tab settings). The heavy, widget-level
  // `editor.updateOptions({...})` below is deliberately NOT keyed on swapTick.
  const languageId = languageOverride ?? getLanguageIdFromPath(filePathRef.current)
  const monacoLanguageId = toMonacoLanguageId(languageId)
  useEffect(() => {
    const model = modelRef.current
    if (!model) return
    monacoEditor.setModelLanguage(model, monacoLanguageId)
    model.updateOptions({ tabSize, insertSpaces: true })
  }, [monacoLanguageId, tabSize, swapTick])

  // ── External content → model sync (imperative, U5b) ───────────────────────
  // Managed panes are model-authoritative; only GENUINE external changes (disk
  // reload, format-on-save, undo/redo applied to the store) are pushed into the
  // held model via the manager's undo-friendly edit. This installs the handler
  // the content-change subscription calls (and re-keys on swap so it captures the
  // current model/path). Local typing does NOT bounce: it flows model → sink →
  // store, so by the time the store fires `model.getValue() === content` and the
  // edit is skipped. The `externalApplyRef` latch additionally guards the one
  // model-change event a genuine external edit re-fires.
  useEffect(() => {
    const applyExternal = (content: string) => {
      const editor = editorRef.current
      const model = modelRef.current
      if (!editor || !model) return
      const path = filePathRef.current
      if (!path) return
      if (model.getValue() === content) return
      const selection = editor.getSelection()
      // Latch the applied text so the surface ignores the model-change event this
      // edit re-fires (otherwise it would bounce straight back to the store).
      if (externalApplyRef) externalApplyRef.current = content
      editorManager.applyExternalEdit(paneId, fileUri(path), content)
      if (selection) editor.setSelection(selection)
    }
    externalSyncRef.current = applyExternal
    // Reconcile immediately on (re)bind — e.g. a swap to a buffer whose store
    // content already diverges from the freshly shown model.
    applyExternal(activeContentRef.current)
    return () => {
      externalSyncRef.current = () => {}
    }
  }, [editorManager, externalApplyRef, paneId, swapTick])

  // ── Settings: theme (separate so font/layout changes don't redefine theme) ─
  // Runs on mount, when theme inputs change, AND once when the editor instance
  // first becomes available — but NOT on every model swap. `themeBoundEditorRef`
  // tracks the editor instance the theme subscriptions are bound to; we rebind
  // only when that instance changes (editor created/replaced), so a tab switch
  // (swapTick bump with the SAME retained editor) is a cheap no-op.
  const themeBoundEditorRef = useRef<StandaloneEditor | null>(null)
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    const applyTheme = () =>
      monacoEditor.setTheme(defineMonacoTheme(settingsTheme || theme))
    // Always reapply the theme value (cheap) when theme inputs change; rebind the
    // registry subscriptions only when the editor instance itself changed.
    applyTheme()
    if (themeBoundEditorRef.current === editor) return
    themeBoundEditorRef.current = editor
    const unsubscribeRegistry = themeRegistry.onRegistryChange(applyTheme)
    const unsubscribeTheme = themeRegistry.onThemeChange(applyTheme)
    return () => {
      unsubscribeRegistry()
      unsubscribeTheme()
      if (themeBoundEditorRef.current === editor) themeBoundEditorRef.current = null
    }
    // swapTick is intentionally a dep so this re-evaluates when the editor first
    // appears / is replaced, but the subscription rebind is gated by the ref.
  }, [settingsTheme, theme, swapTick])

  // ── Settings: all non-theme editor options (widget-level) ─────────────────
  // Keyed on actual settings values only — NOT swapTick — so a tab switch does
  // not re-run this ~20-option `updateOptions`. `swapTick` is still a dep purely
  // so the effect re-evaluates when the editor instance first becomes available;
  // the body short-circuits to a no-op once it has applied the current settings
  // to the current editor instance (guarded by `optionsBoundStateRef`).
  const optionsBoundStateRef = useRef<{ editor: StandaloneEditor; key: string } | null>(null)
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    // Identity of "these settings on this editor". If unchanged (e.g. a pure tab
    // switch bumped swapTick but nothing settings-related moved), skip the work.
    const settingsKey = JSON.stringify([
      autoCompletion,
      fontFamily,
      fontSize,
      highlightOccurrences,
      lineHeight,
      lineNumbers,
      // line-number formatting inputs (formatter identity is derived from these)
      lineNumberStart,
      lineNumberMap,
      minimapEnabled,
      parameterHints,
      readOnly,
      renderIndentGuides,
      renderWhitespace,
      scrollable,
      tabSize,
      wordWrap,
    ])
    const bound = optionsBoundStateRef.current
    if (bound && bound.editor === editor && bound.key === settingsKey) return
    optionsBoundStateRef.current = { editor, key: settingsKey }
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
    lineNumberMap,
    lineNumberStart,
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
  // The diagnostics subscription is set up synchronously (cheap) so markers paint
  // as soon as the server reports them. The EXPENSIVE part — `documentOpen` — is
  // deferred to an idle tick so it never blocks the content paint on open/switch.
  //
  // Lifecycle safety: the deferred open is cancelled if this effect cleans up
  // (swap/unmount) before it fires, so a rapid open→open→open never leaks an open
  // document and ends with exactly one open (the last). The previous document is
  // closed synchronously on cleanup regardless of whether its open already fired
  // (`documentClose` is a no-op for a document that was never opened).
  useEffect(() => {
    const model = modelRef.current
    const filePath = filePathRef.current
    if (!model || !filePath) return
    const client = LspClient.getInstance()

    const applyMarkers = (fp: string, diagnostics: LspDiagnostic[]) => {
      if (!pathsMatch(fp, filePath)) return
      const current = modelRef.current
      if (!current) return
      monacoEditor.setModelMarkers(current, 'crowbar-lsp', diagnostics.map(toMonacoMarker))
    }
    const unsubscribe = client.onDiagnosticsUpdate(applyMarkers)

    let opened = false
    const openHandle = scheduleIdleTask(() => {
      opened = true
      void client.documentOpen(filePath, model.getValue(), languageId ?? 'plaintext')
    })

    return () => {
      // Cancel the pending open if it has not fired yet (prevents a leaked open
      // for a buffer we are already switching away from).
      openHandle.cancel()
      unsubscribe()
      // Close only if we actually opened it; harmless otherwise.
      if (opened) void client.documentClose(filePath)
      const current = modelRef.current
      if (current) monacoEditor.setModelMarkers(current, 'crowbar-lsp', [])
    }
  }, [languageId, swapTick])

  // ── LSP re-analyze on edits (debounced, imperative — U5b) ─────────────────
  // Driven by the content-change signal, not a render dep. Each change (re)arms a
  // trailing 400ms timer that reads the model's CURRENT text imperatively
  // (`model.getValue()`) and sends `documentChange`. Re-keyed on swap so it
  // targets the live model/path; the pending timer is cleared on rebind/unmount.
  useEffect(() => {
    const filePath = filePathRef.current
    if (!filePath) {
      lspDidChangeRef.current = () => {}
      return
    }
    let timer: ReturnType<typeof setTimeout> | null = null
    lspDidChangeRef.current = () => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        timer = null
        const content = modelRef.current?.getValue() ?? activeContentRef.current
        void LspClient.getInstance().documentChange(filePath, content)
      }, 400)
    }
    return () => {
      if (timer) clearTimeout(timer)
      lspDidChangeRef.current = () => {}
    }
  }, [swapTick])
}

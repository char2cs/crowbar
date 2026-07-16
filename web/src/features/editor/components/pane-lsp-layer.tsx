/**
 * PaneLspLayer — the per-pane "rich editor services" subtree.
 *
 * This is the part of the editor that genuinely depends on the ACTIVE BUFFER
 * (its `filePath` + `value`): LSP integration (hover / go-to-definition /
 * completion / signature help / code-lens / rename / diagnostics document
 * lifecycle) and the in-file find (search effects + match navigation +
 * go-to-line).
 *
 * It is mounted ONCE per pane by {@link EditorSurface} and reads the active
 * file/content through its OWN narrow subscriptions:
 *  - `filePath` from the per-pane active-editor registry (published by the
 *    controller on every swap), via a tiny `useState` leaf — so a tab switch
 *    re-renders only THIS component, not EditorSurface or the Monaco slot.
 *  - the active buffer's text is read IMPERATIVELY (a vanilla store subscription
 *    keeps a ref fresh; LSP reads it on demand via `getValue`, in-file search at
 *    its debounced run) — content changes do NOT re-render this component.
 *
 * The container-level mouse handlers (hover / cmd-hover link / cmd-click) are
 * published into a parent-owned ref so {@link EditorSurface} can keep STABLE
 * handler identities on the container div without re-rendering on swap.
 *
 * Overlays render as a fragment nested inside EditorSurface's positioned
 * `editor-container`, so their absolute/fixed positioning is unchanged.
 */

import type React from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useLspIntegration } from '@/features/editor/hooks/use-lsp-integration'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useEditorUIStore } from '@/features/editor/stores/ui-store'
import { calculateLineHeight } from '@/features/editor/utils/lines'
import { resolveGoToLineTarget } from '@/features/editor/utils/go-to-line'
import { calculateCursorPositionFromContent } from '@/features/editor/utils/position'
import { buildSearchRegex, findLimitedMatchesCooperative } from '@/features/editor/utils/search'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { useUIState } from '@/features/window/stores/ui-state-store'
import type {
  EditorCoordinateResolver,
  EditorModelPositionResolver,
} from '@/features/editor/view-model/view-layout'
import { editorAPI } from '../extensions/api'
import CodeLensOverlay from '../lsp/code-lens-overlay'
import { HoverTooltip } from '../lsp/hover-tooltip'
import { LspClient } from '../lsp/lsp-client'
import RenameInput from '../lsp/rename-input'
import { SignatureHelpTooltip } from '../lsp/signature-help-tooltip'
import { useCodeLens } from '../lsp/use-code-lens'
import { scheduleIdleTask } from '../lib/idle-task'
import { useRename } from '../lsp/use-rename'
import { CompletionDropdown } from '../completion/completion-dropdown'
import { useEditorViewStore } from '../stores/view-store'
import type { PaneOverlayMouseHandlers } from './pane-overlay-handlers'

const SEARCH_DEBOUNCE_MS = 300
const MAX_FILE_SEARCH_MATCHES = 20_000

interface GoToLineEventDetail {
  line?: number
  column?: number
  path?: string
}

export interface PaneLspLayerProps {
  paneId: string
  /** Active surface gate (interactive + rich services). Stable per pane mount. */
  isActiveSurface: boolean
  /** The positioned `editor-container` div (overlay parent + scroll source). */
  overlayContainerRef: React.RefObject<HTMLDivElement | null>
  /** Parent-owned ref the parent uses to keep stable container mouse handlers. */
  mouseHandlersRef: React.MutableRefObject<PaneOverlayMouseHandlers | null>
  /** Resolvers (live model coords) supplied by the satellites layer via refs. */
  resolveEditorPosition: EditorCoordinateResolver
  resolveModelPosition: EditorModelPositionResolver
  /** Sync LSP overlay containers to the editor scroll offset (owned by parent). */
  syncLspOverlayTransform: (scrollTop: number, scrollLeft: number) => void
  /** Refs for the scroll-transformed overlay containers. */
  codeLensRef: React.RefObject<HTMLDivElement | null>
  renameInputRef: React.RefObject<HTMLDivElement | null>
}

/**
 * Mounted once per pane inside the editor container; owns everything that
 * depends on the active buffer's path/content so a buffer switch only re-renders
 * this leaf.
 */
// react-doctor-disable-next-line no-giant-component -- accepted: cohesive LSP overlay — hover/completion/diagnostics layers all bind to one editor's LSP client and position math; not separable without duplicating that binding.
export function PaneLspLayer({
  paneId,
  isActiveSurface,
  overlayContainerRef,
  mouseHandlersRef,
  resolveEditorPosition,
  resolveModelPosition,
  syncLspOverlayTransform,
  codeLensRef,
  renameInputRef,
}: PaneLspLayerProps) {
  const valueRef = useRef('')
  const searchTimerRef = useRef<NodeJS.Timeout | null>(null)
  const searchRunIdRef = useRef(0)
  const lspScrollRafRef = useRef<number | null>(null)

  const enableInteractiveServices = isActiveSurface
  const enableRichEditorServices = enableInteractiveServices

  // ── Active file path: from the registry (re-renders only this leaf on swap) ─
  const workspaceStore = useWorkspaceStore()
  const registry = workspaceStore.activeEditorRegistry
  const [filePath, setFilePath] = useState(() => registry.get(paneId)?.filePath ?? '')
  useEffect(() => {
    return registry.subscribe(paneId, (ctx) => setFilePath(ctx?.filePath ?? ''))
  }, [registry, paneId])

  // ── Deferred rich-services gate ────────────────────────────────────────────
  // The heavy LSP work (`useLspIntegration` → documentOpen; code-lens fetch) is
  // gated behind a short post-swap idle flag so it does NOT run on the synchronous
  // switch/open frame — the content paints first, then these enable a beat later.
  // The overlays themselves stay mounted (gated only by `enableRichEditorServices`);
  // we just defer the EXPENSIVE work. On each swap the flag resets to false and
  // re-arms; the pending idle task is cancelled on the next swap/unmount, so the
  // LSP document lifecycle (documentOpen/Close in `useLspIntegration`, routed
  // through its `openedDocumentsRef` guards) is never double-fired or leaked.
  const [richServicesReady, setRichServicesReady] = useState(false)
  useEffect(() => {
    if (!enableRichEditorServices) {
      setRichServicesReady(false)
      return
    }
    setRichServicesReady(false)
    const handle = scheduleIdleTask(() => setRichServicesReady(true))
    return () => handle.cancel()
  }, [enableRichEditorServices, filePath])

  const enableDeferredRichServices = enableRichEditorServices && richServicesReady

  // ── Active content: IMPERATIVE subscription (no render dependency) ──────────
  // U5: PaneLspLayer must NOT re-render on content change. We read the active
  // buffer's text via a vanilla store subscription that keeps `valueRef` fresh
  // and bumps a NON-render `contentVersionRef` + re-arms the dependent debounced
  // work (in-file search) — without subscribing content into render. LSP reads
  // content on demand through `getValue` (passed to useLspIntegration).
  const readActiveContent = useCallback(() => {
    const state = workspaceStore.getState()
    const bufferId = state.panes[paneId]?.activeBufferId ?? null
    const buffer = bufferId ? state.buffers.find((b) => b.id === bufferId) : null
    return buffer && hasTextContent(buffer) ? buffer.content : ''
  }, [workspaceStore, paneId])
  const getValue = useCallback(() => valueRef.current, [])
  // Seed the ref synchronously on first render so the first LSP/search run has
  // the current content even before the subscription's first emit.
  valueRef.current = readActiveContent()

  // Imperative content change signal: re-arm in-file search off-render.
  const onContentChangeRef = useRef<(() => void) | null>(null)
  useEffect(() => {
    valueRef.current = readActiveContent()
    let previous = valueRef.current
    return workspaceStore.subscribe(() => {
      const next = readActiveContent()
      if (next === previous) return
      previous = next
      valueRef.current = next
      onContentChangeRef.current?.()
    })
  }, [workspaceStore, readActiveContent])

  const zoomLevel = useZoomStore.use.editorZoomLevel()
  const settings = useSettingsStore((s) => s.settings)
  const zoomedFontSize = settings.fontSize * zoomLevel
  const zoomedLineHeight = calculateLineHeight(zoomedFontSize, settings.editorLineHeight)

  const lspClient = LspClient.getInstance()

  const isFindVisible = useUIState((state) => state.isFindVisible)
  const searchQuery = useEditorUIStore.use.searchQuery()
  const searchMatches = useEditorUIStore.use.searchMatches()
  const currentMatchIndex = useEditorUIStore.use.currentMatchIndex()
  const searchOptions = useEditorUIStore.use.searchOptions()
  const { setSearchResults } = useEditorUIStore.use.actions()

  // ── LSP integration + overlay hooks ───────────────────────────────────────
  const { hoverHandlers, goToDefinitionHandlers, definitionLinkHandlers } = useLspIntegration({
    // Gate the EXPENSIVE document lifecycle behind the post-swap idle flag so the
    // synchronous switch frame stays light. Hover/go-to-def handler IDENTITIES are
    // still produced (they read the live value), so interactivity is unaffected.
    enabled: enableDeferredRichServices,
    filePath,
    getValue,
    editorRef: overlayContainerRef,
    resolveEditorPosition,
  })
  const rename = useRename(enableRichEditorServices ? filePath : undefined)
  const codeLenses = useCodeLens(
    enableDeferredRichServices ? filePath : undefined,
    enableDeferredRichServices,
  )

  const handleCodeLensExecute = useCallback(
    (lens: { title: string; command?: string; arguments?: unknown[] }) => {
      if (!filePath || !lens.command) return
      void lspClient.applyCodeAction(filePath, {
        title: lens.title,
        command: lens.command,
        arguments: lens.arguments ?? [],
      })
    },
    [filePath, lspClient],
  )

  // ── Publish stable container mouse handlers to the parent-owned ref ────────
  useEffect(() => {
    if (!enableInteractiveServices) {
      mouseHandlersRef.current = null
      return
    }
    mouseHandlersRef.current = {
      handleMouseMove: (e) => {
        hoverHandlers.handleHover(e)
        definitionLinkHandlers.handleMouseMove(e)
      },
      handleMouseLeave: () => {
        hoverHandlers.handleMouseLeave()
        definitionLinkHandlers.handleMouseLeave()
      },
      handleMouseEnter: hoverHandlers.handleMouseEnter,
      handleClick: goToDefinitionHandlers.handleClick,
    }
    return () => {
      mouseHandlersRef.current = null
    }
  }, [
    enableInteractiveServices,
    mouseHandlersRef,
    hoverHandlers,
    definitionLinkHandlers,
    goToDefinitionHandlers,
  ])

  // ── LSP overlay scroll-offset transform (follows the textarea scroll) ──────
  useEffect(() => {
    const container = overlayContainerRef.current
    if (!container) return
    const textarea = container.querySelector('textarea')
    if (!textarea) return
    const handleScroll = () => {
      if (lspScrollRafRef.current !== null) return
      lspScrollRafRef.current = requestAnimationFrame(() => {
        syncLspOverlayTransform(textarea.scrollTop, textarea.scrollLeft)
        lspScrollRafRef.current = null
      })
    }
    textarea.addEventListener('scroll', handleScroll, { passive: true })
    syncLspOverlayTransform(textarea.scrollTop, textarea.scrollLeft)
    return () => {
      textarea.removeEventListener('scroll', handleScroll)
      if (lspScrollRafRef.current !== null) {
        cancelAnimationFrame(lspScrollRafRef.current)
        lspScrollRafRef.current = null
      }
    }
  }, [overlayContainerRef, syncLspOverlayTransform])

  // ── Go-to-line events ──────────────────────────────────────────────────────
  useEffect(() => {
    if (!isActiveSurface) return
    const goToLine = (lineNumber: number, columnNumber?: number) => {
      if (!overlayContainerRef.current) return false
      const currentContent = valueRef.current
      if (!currentContent) return false
      const target = resolveGoToLineTarget({
        content: currentContent,
        lineNumber,
        columnNumber,
        lineCount: useEditorViewStore.getState().actions.getLineCount(),
      })
      editorAPI.setSelection(undefined)
      editorAPI.setCursorPosition({
        line: target.line,
        column: target.column,
        offset: target.offset,
      })
      return true
    }
    const handleGoToLine = (event: CustomEvent<GoToLineEventDetail>) => {
      const lineNumber = event.detail?.line
      const columnNumber = event.detail?.column
      const targetPath = event.detail?.path
      if (targetPath && targetPath !== filePath) return
      if (!lineNumber) return
      if (!goToLine(lineNumber, columnNumber)) {
        setTimeout(() => goToLine(lineNumber, columnNumber), 150)
      }
    }
    window.addEventListener('menu-go-to-line', handleGoToLine as EventListener)
    return () => window.removeEventListener('menu-go-to-line', handleGoToLine as EventListener)
  }, [filePath, isActiveSurface, overlayContainerRef])

  // ── In-file search ─────────────────────────────────────────────────────────
  // Reads content IMPERATIVELY (`valueRef.current`) at debounced run time, so it
  // does not depend on `value` in render (U5). It is re-armed both when the
  // search config changes (effect deps) and when content changes while find is
  // visible (via `onContentChangeRef`).
  const runSearchRef = useRef<() => void>(() => {})
  runSearchRef.current = () => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    const content = valueRef.current
    if (!enableInteractiveServices || !isFindVisible || !searchQuery.trim() || !content) {
      searchRunIdRef.current += 1
      setSearchResults([], -1)
      return
    }
    const searchRunId = searchRunIdRef.current + 1
    searchRunIdRef.current = searchRunId
    searchTimerRef.current = setTimeout(() => {
      const regex = buildSearchRegex(searchQuery, searchOptions)
      if (!regex) {
        setSearchResults([], -1)
        return
      }
      void findLimitedMatchesCooperative(content, regex, MAX_FILE_SEARCH_MATCHES, {
        shouldCancel: () => searchRunIdRef.current !== searchRunId,
      }).then((result) => {
        if (!result || searchRunIdRef.current !== searchRunId) return
        setSearchResults(result.matches, result.matches.length > 0 ? 0 : -1, result.limited)
      })
    }, SEARCH_DEBOUNCE_MS)
  }
  useEffect(() => {
    runSearchRef.current()
    return () => {
      searchRunIdRef.current += 1
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    }
  }, [enableInteractiveServices, isFindVisible, searchQuery, searchOptions, setSearchResults])
  // Re-run search when content changes (driven imperatively, off-render).
  useEffect(() => {
    onContentChangeRef.current = () => runSearchRef.current()
    return () => {
      onContentChangeRef.current = null
    }
  }, [])

  // ── Search navigation ──────────────────────────────────────────────────────
  useEffect(() => {
    if (!enableInteractiveServices) return
    if (searchMatches.length > 0 && currentMatchIndex >= 0) {
      const match = searchMatches[currentMatchIndex]
      if (!match) return
      const startPosition = calculateCursorPositionFromContent(match.start, valueRef.current)
      const endPosition = calculateCursorPositionFromContent(match.end, valueRef.current)
      editorAPI.setSelection({ start: startPosition, end: endPosition })
      editorAPI.setCursorPosition(endPosition)
    }
  }, [currentMatchIndex, enableInteractiveServices, searchMatches])

  if (!enableRichEditorServices) return null

  return (
    <>
      <HoverTooltip />
      <CompletionDropdown />
      {codeLenses.length > 0 && (
        <CodeLensOverlay
          ref={codeLensRef}
          lenses={codeLenses}
          fontSize={zoomedFontSize}
          lineHeight={zoomedLineHeight}
          scrollTop={overlayContainerRef.current?.querySelector('textarea')?.scrollTop ?? 0}
          viewportHeight={overlayContainerRef.current?.clientHeight ?? 600}
          onExecute={handleCodeLensExecute}
          resolveModelPosition={resolveModelPosition}
        />
      )}
      <SignatureHelpTooltip
        editorRef={overlayContainerRef}
        filePath={filePath}
        resolveModelPosition={resolveModelPosition}
      />
      {rename.renameState && (
        <RenameInput
          ref={renameInputRef}
          symbol={rename.renameState.symbol}
          line={rename.renameState.line}
          column={rename.renameState.column}
          fontSize={zoomedFontSize}
          lineHeight={zoomedLineHeight}
          charWidth={zoomedFontSize * 0.6}
          resolveModelPosition={resolveModelPosition}
          inputRef={rename.inputRef}
          onSubmit={(newName) => void rename.executeRename(newName)}
          onCancel={rename.cancelRename}
        />
      )}
    </>
  )
}

export default PaneLspLayer

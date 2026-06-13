import '../monaco/monaco-environment'
import '../monaco/language-contributions'
import 'monaco-editor/min/vs/editor/editor.main.css'
import '../styles/monaco-editor.css'

import type React from 'react'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useLspIntegration } from '@/features/editor/hooks/use-lsp-integration'
import { useEditorScroll } from '@/features/editor/hooks/use-scroll'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { useSettingsStore } from '@/features/settings/store'
import { useEditorSettingsStore } from '@/features/editor/stores/settings-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useEditorUIStore } from '@/features/editor/stores/ui-store'
import { useEditorViewStore } from '@/features/editor/stores/view-store'
import { useEditorAppStore } from '@/features/editor/stores/editor-app-store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
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
import { useRename } from '../lsp/use-rename'
import { CompletionDropdown } from '../completion/completion-dropdown'
import { ScrollDebugOverlay } from './debug/scroll-debug-overlay'
import { EditorStylesheet } from './stylesheet'
import Breadcrumb, { type BreadcrumbProps } from './toolbar/breadcrumb'
import FindBar from './toolbar/find-bar'
import { usePaneEditorController } from '../hooks/use-pane-editor-controller'
import { usePaneEditorSatellites } from '../hooks/use-pane-editor-satellites'
import { defineMonacoTheme } from '../monaco/define-theme'
import { toEditorPosition, toEditorRange } from '../monaco/editor-conversions'
import type * as Monaco from 'monaco-editor'

const SEARCH_DEBOUNCE_MS = 300
const MAX_FILE_SEARCH_MATCHES = 20_000

interface GoToLineEventDetail {
  line?: number
  column?: number
  path?: string
}

export interface EditorSurfaceProps {
  paneId: string
  bufferId: string
  isActiveSurface?: boolean
  isPreview?: boolean
  onPromote?: () => void
  showToolbar?: boolean
  className?: string
  breadcrumbProps?: BreadcrumbProps
}

/**
 * Stable per-pane editor shell. Mounted ONCE per pane (keyed by `paneId` by the
 * parent). A buffer/tab switch is driven imperatively by
 * {@link usePaneEditorController} (model swap + content/cursor seam) and the
 * retained widget's satellite concerns are retargeted by
 * {@link usePaneEditorSatellites} — NOT by remounting this subtree. The Monaco
 * slot, breadcrumb, find bar and LSP overlays mount once; their leaves update
 * via narrow store / active-editor-registry subscriptions.
 */
export function EditorSurface({
  paneId,
  bufferId,
  isActiveSurface = true,
  isPreview = false,
  onPromote,
  showToolbar = true,
  className,
  breadcrumbProps,
}: EditorSurfaceProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const overlayContainerRef = useRef<HTMLDivElement>(null)
  const codeLensRef = useRef<HTMLDivElement>(null)
  const renameInputRef = useRef<HTMLDivElement>(null)
  const searchTimerRef = useRef<NodeJS.Timeout | null>(null)
  const searchRunIdRef = useRef(0)
  const valueRef = useRef('')
  const lspScrollRafRef = useRef<number | null>(null)
  const editorCoordinateResolverRef = useRef<EditorCoordinateResolver | null>(null)
  const editorModelPositionResolverRef = useRef<EditorModelPositionResolver | null>(null)

  const workspaceStore = useWorkspaceStore()
  const editorManager = workspaceStore.editorManager
  const registry = workspaceStore.activeEditorRegistry

  const { setRefs, setContent, setFileInfo, setActiveEditorViewKey, setCursorPosition, setSelection } =
    useEditorStateStore.use.actions()

  // Narrow per-pane selectors — these update only the leaves that read them.
  const activeBufferId = useWorkspaceStoreContext(
    useCallback((state) => state.panes[paneId]?.activeBufferId ?? null, [paneId]),
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

  const value = activeBuffer && hasTextContent(activeBuffer) ? activeBuffer.content : ''
  valueRef.current = value
  const filePath = activeBuffer?.path || ''
  const editorViewKey = activeBufferId ? `${paneId}:${activeBufferId}` : null

  const zoomLevel = useZoomStore.use.editorZoomLevel()
  const settings = useSettingsStore((s) => s.settings)
  const zoomedFontSize = settings.fontSize * zoomLevel
  const zoomedLineHeight = calculateLineHeight(zoomedFontSize, settings.editorLineHeight)

  const isFindVisible = useUIState((state) => state.isFindVisible)
  const lspClient = useMemo(() => LspClient.getInstance(), [])

  const { handleContentChange } = useEditorAppStore.use.actions()
  const searchQuery = useEditorUIStore.use.searchQuery()
  const searchMatches = useEditorUIStore.use.searchMatches()
  const currentMatchIndex = useEditorUIStore.use.currentMatchIndex()
  const searchOptions = useEditorUIStore.use.searchOptions()
  const { setSearchResults } = useEditorUIStore.use.actions()

  const enableInteractiveServices = isActiveSurface
  const enableRichEditorServices = enableInteractiveServices

  // ── Imperative buffer-switch controller (model swap + content seam) ───────
  const syncCursorAndSelection = useCallback(() => {
    const editor = editorManager.getRawEditor(paneId) as
      | Monaco.editor.IStandaloneCodeEditor
      | null
    const model = editor?.getModel()
    if (!editor || !model) return
    const position = editor.getPosition()
    if (position) {
      setCursorPosition(toEditorPosition(model, position), { ensureVisible: false })
    }
    const selection = editor.getSelection()
    setSelection(selection ? toEditorRange(model, selection) : undefined)
  }, [editorManager, paneId, setCursorPosition, setSelection])

  // mountPane wires the retained widget into the slot, sets the viewport ref,
  // applies the initial theme and installs the rAF-debounced ResizeObserver +
  // pane-resize-end layout (suppressed during a drag) — once per pane.
  const resizeCleanupRef = useRef<(() => void) | null>(null)
  const mountPane = useCallback(
    (container: HTMLElement) => {
      editorManager.mountPane(paneId, container)
      editorAPI.setTextareaRef(null)
      editorAPI.setViewportRef(container as HTMLDivElement)

      const raw = editorManager.getRawEditor(paneId) as
        | Monaco.editor.IStandaloneCodeEditor
        | null
      // Initial theme to avoid a flash; the satellites theme effect (which also
      // subscribes to theme changes) is authoritative right after mount.
      const editorSettingsTheme = useEditorSettingsStore.getState().theme
      raw?.updateOptions({
        theme: defineMonacoTheme(useSettingsStore.getState().settings.theme || editorSettingsTheme),
      })

      let layoutRafId: number | null = null
      let needsLayoutAfterResize = false
      const runLayout = () => editorManager.layoutPane(paneId)
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

      resizeCleanupRef.current = () => {
        resizeObserver.disconnect()
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
      }
    },
    // Theme is read fresh via getState(); mount-once per pane.
    [editorManager, paneId],
  )

  const unmountPane = useCallback(() => {
    resizeCleanupRef.current?.()
    resizeCleanupRef.current = null
    editorAPI.setViewportRef(null)
    editorManager.unmountPane(paneId)
  }, [editorManager, paneId])

  // Persist a content change, folding preview-promote-on-first-edit in. Skipped
  // when this surface isn't active (matches legacy CodeEditor) or when the edit
  // is the echo of a genuine external change applied by the satellites.
  const isPreviewRef = useRef(isPreview)
  isPreviewRef.current = isPreview
  const onPromoteRef = useRef(onPromote)
  onPromoteRef.current = onPromote
  const isActiveSurfaceRef = useRef(isActiveSurface)
  isActiveSurfaceRef.current = isActiveSurface
  const externalApplyRef = useRef<string | null>(null)
  const onContentChange = useCallback(
    (content: string) => {
      if (externalApplyRef.current === content) {
        externalApplyRef.current = null
        return
      }
      if (!isActiveSurfaceRef.current) return
      if (isPreviewRef.current) onPromoteRef.current?.()
      void handleContentChange(content, undefined, undefined, undefined, {
        contentAlreadyApplied: false,
      })
    },
    [handleContentChange],
  )

  const selectActiveBuffer = useCallback(
    (state: import('@/features/workspace/stores/workspace-store.types').WorkspaceState) => {
      const id = state.panes[paneId]?.activeBufferId ?? null
      const buffer = id ? state.buffers.find((b) => b.id === id) : null
      if (!buffer || !hasTextContent(buffer)) return null
      return { filePath: buffer.path }
    },
    [paneId],
  )

  usePaneEditorController(paneId, containerRef, {
    store: workspaceStore,
    selectActiveBuffer,
    manager: editorManager,
    registry,
    mountPane,
    unmountPane,
    onContentChange,
    syncCursorAndSelection,
  })

  // ── Retained-widget satellite concerns (settings, theme, decorations, LSP) ─
  const syncLspOverlayTransform = useCallback((scrollTop: number, scrollLeft: number) => {
    const transform = `translate(-${scrollLeft}px, -${scrollTop}px)`
    for (const ref of [codeLensRef, renameInputRef]) {
      if (ref.current) ref.current.style.transform = transform
    }
  }, [])

  const handleCoordinateResolverChange = useCallback(
    (resolver: EditorCoordinateResolver | null) => {
      editorCoordinateResolverRef.current = resolver
    },
    [],
  )
  const handleModelPositionResolverChange = useCallback(
    (resolver: EditorModelPositionResolver | null) => {
      editorModelPositionResolverRef.current = resolver
    },
    [],
  )

  usePaneEditorSatellites(paneId, {
    onScrollOffsetChange: syncLspOverlayTransform,
    onCoordinateResolverChange: handleCoordinateResolverChange,
    onModelPositionResolverChange: handleModelPositionResolverChange,
    isActiveSurface,
    readOnly: false,
    scrollable: true,
    externalApplyRef,
  })

  // ── Editor-state store wiring (active surface only) ────────────────────────
  useEffect(() => {
    if (!isActiveSurface) return
    setRefs({ editorRef: overlayContainerRef })
  }, [isActiveSurface, setRefs])

  useEffect(() => {
    if (!isActiveSurface) return
    setActiveEditorViewKey(editorViewKey)
  }, [editorViewKey, isActiveSurface, setActiveEditorViewKey])

  useEffect(() => {
    if (!isActiveSurface) return
    setContent('', onContentChange)
  }, [isActiveSurface, onContentChange, setContent])

  useEffect(() => {
    if (!isActiveSurface) return
    setFileInfo(filePath)
  }, [filePath, isActiveSurface, setFileInfo])

  // ── LSP integration + overlays ────────────────────────────────────────────
  const resolveEditorPosition = useCallback<EditorCoordinateResolver>(
    (clientX, clientY) => editorCoordinateResolverRef.current?.(clientX, clientY) ?? null,
    [],
  )
  const resolveModelPosition = useCallback<EditorModelPositionResolver>(
    (line, column) => editorModelPositionResolverRef.current?.(line, column) ?? null,
    [],
  )

  const { hoverHandlers, goToDefinitionHandlers, definitionLinkHandlers } = useLspIntegration({
    enabled: enableRichEditorServices,
    filePath,
    value,
    editorRef: overlayContainerRef,
    resolveEditorPosition,
  })
  const rename = useRename(enableRichEditorServices ? filePath : undefined)
  const codeLenses = useCodeLens(
    enableRichEditorServices ? filePath : undefined,
    enableRichEditorServices,
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
  }, [syncLspOverlayTransform])

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!enableInteractiveServices) return
    hoverHandlers.handleHover(e)
    definitionLinkHandlers.handleMouseMove(e)
  }
  const handleMouseLeave = () => {
    if (!enableInteractiveServices) return
    hoverHandlers.handleMouseLeave()
    definitionLinkHandlers.handleMouseLeave()
  }

  useEditorScroll(overlayContainerRef, null)

  // Go-to-line events
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
  }, [filePath, isActiveSurface])

  // In-file search
  useEffect(() => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    if (!enableInteractiveServices || !isFindVisible) {
      searchRunIdRef.current += 1
      setSearchResults([], -1)
      return
    }
    if (!searchQuery.trim() || !value) {
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
      void findLimitedMatchesCooperative(value, regex, MAX_FILE_SEARCH_MATCHES, {
        shouldCancel: () => searchRunIdRef.current !== searchRunId,
      }).then((result) => {
        if (!result || searchRunIdRef.current !== searchRunId) return
        setSearchResults(result.matches, result.matches.length > 0 ? 0 : -1, result.limited)
      })
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      searchRunIdRef.current += 1
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    }
  }, [enableInteractiveServices, isFindVisible, searchQuery, searchOptions, value, setSearchResults])

  // Search navigation
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

  // `bufferId` is the parent's stable-mount hint (the surface keys off paneId and
  // reads the active buffer reactively); referenced to satisfy noUnusedParameters.
  void bufferId

  return (
    <>
      <EditorStylesheet />
      <div className="absolute inset-0 flex flex-col overflow-hidden">
        {showToolbar && (
          <Breadcrumb
            {...breadcrumbProps}
            editorViewKey={editorViewKey}
            bufferId={activeBufferId ?? undefined}
            filePathOverride={breadcrumbProps?.filePathOverride ?? filePath}
          />
        )}

        {showToolbar && enableInteractiveServices && <FindBar />}

        <div
          ref={overlayContainerRef}
          className={`editor-container relative min-h-0 flex-1 overflow-hidden ${className || ''}`}
          data-zoom-level={zoomLevel}
          style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
          onMouseEnter={enableRichEditorServices ? hoverHandlers.handleMouseEnter : undefined}
          onClick={enableRichEditorServices ? goToDefinitionHandlers.handleClick : undefined}
        >
          {enableRichEditorServices && <HoverTooltip />}
          {enableRichEditorServices && <CompletionDropdown />}
          {enableRichEditorServices && codeLenses.length > 0 && (
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
          {enableRichEditorServices && (
            <SignatureHelpTooltip
              editorRef={overlayContainerRef}
              filePath={filePath}
              resolveModelPosition={resolveModelPosition}
            />
          )}
          {enableRichEditorServices && rename.renameState && (
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

          {/* Stable Monaco slot — the retained per-pane widget mounts here. */}
          <div className="absolute inset-0 bg-background">
            <div
              ref={containerRef}
              className="absolute inset-0"
              data-monaco-editor-scroll
            />
          </div>
        </div>
      </div>

      {enableInteractiveServices && <ScrollDebugOverlay />}
    </>
  )
}

export default EditorSurface

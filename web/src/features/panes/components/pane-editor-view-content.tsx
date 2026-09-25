import { Suspense, useEffect, useMemo, useRef } from 'react'
import { useBuffersByIds } from '@/features/workspace/stores/hooks/use-buffer-store'
import type { EditorContent, PaneContent, TerminalContent } from '../types/pane-content'
import { clearEditorPortalEntry, setEditorPortalEntry } from '../lib/editor-portal-registry'
import { NewTabView } from './new-tab-view'
import { renderPaneContent } from './pane-content-registry'
import { TerminalPane } from './terminal-pane'

type EditorBufferShell = Pick<EditorContent, 'id' | 'path' | 'name' | 'type' | 'isPreview'>
type PaneRenderBuffer = Exclude<PaneContent, EditorContent> | EditorBufferShell

interface PaneEditorViewContentProps {
  paneId: string
  editorTabIds: string[]
  activeEditorTabId: string | null | undefined
  /** The editor view is hidden (the chat fills the pane, or a tabs-mode chat tab is up). */
  editorViewHidden: boolean
  isActivePane: boolean
  /** Whether this pane's view is on screen at all. */
  showing: boolean
}

/**
 * Everything `editorTabIds` holds — files, terminals, branch review — the same
 * whether or not the pane also holds a chat.
 */
export function PaneEditorViewContent({
  paneId,
  editorTabIds,
  activeEditorTabId,
  editorViewHidden,
  isActivePane,
  showing,
}: PaneEditorViewContentProps) {
  const rawPaneBuffers = useBuffersByIds(editorTabIds)
  const paneBuffers = useMemo(
    (): PaneRenderBuffer[] =>
      rawPaneBuffers.map((buffer) =>
        buffer.type === 'editor'
          ? ({
              id: buffer.id,
              path: buffer.path,
              name: buffer.name,
              type: buffer.type,
              isPreview: buffer.isPreview,
            } satisfies EditorBufferShell)
          : buffer,
      ),
    [rawPaneBuffers],
  )
  const activeBuffer = useMemo(() => {
    if (!activeEditorTabId) return null
    return paneBuffers.find((b) => b.id === activeEditorTabId) || null
  }, [paneBuffers, activeEditorTabId])

  // The editor widget lives in EditorHostRegistry, outside this component;
  // this pane only publishes WHERE to portal it and what it should show.
  const editorPortalTargetRef = useRef<HTMLDivElement>(null)
  const hasOpenEditorBuffer = paneBuffers.some((b) => b.type === 'editor')
  const isEditorTabActive = activeBuffer?.type === 'editor' && !editorViewHidden
  const activeEditorBuffer = activeBuffer?.type === 'editor' ? activeBuffer : null
  useEffect(() => {
    const node = editorPortalTargetRef.current
    if (!hasOpenEditorBuffer || !node) {
      clearEditorPortalEntry(paneId)
      return
    }
    setEditorPortalEntry(paneId, {
      node,
      activeEditorBufferId: activeEditorBuffer?.id ?? null,
      isPreview: activeEditorBuffer?.isPreview ?? false,
      isActiveSurface: isEditorTabActive && isActivePane,
    })
    return () => clearEditorPortalEntry(paneId)
  }, [
    paneId,
    hasOpenEditorBuffer,
    activeEditorBuffer?.id,
    activeEditorBuffer?.isPreview,
    isEditorTabActive,
    isActivePane,
  ])

  return (
    <>
      {/* A fallback for a pane holding nothing — deliberately inert; see
          new-tab-view.tsx. */}
      {!activeBuffer && <NewTabView paneId={paneId} />}

      {/* Terminals stay mounted to keep their PTYs, outside the Suspense
          boundary so a cold chunk load never unmounts them. Visible only as
          the active tab of an editor view that is not hidden. */}
      {paneBuffers
        .filter((b): b is TerminalContent => b.type === 'terminal')
        .map((b) => {
          const isActive = b.id === activeBuffer?.id && !editorViewHidden
          return (
            <div
              key={b.id}
              className="absolute inset-0"
              style={isActive ? undefined : { visibility: 'hidden' }}
            >
              <TerminalPane
                sessionId={b.sessionId}
                bufferId={b.id}
                paneId={paneId}
                workspaceId={b.workspaceId}
                initialCommand={b.initialCommand}
                workingDirectory={b.workingDirectory}
                isActive={isActive && isActivePane}
                // xterm gates its render loop on this: a parked view's
                // terminal must stop drawing, not merely be covered.
                isVisible={isActive && showing}
              />
            </div>
          )
        })}

      {/* Editor-portal target (editor-host-registry.tsx): the live widget is
          reparented into whichever target node is registered, so a tab
          switch, split or fullscreen never remounts it. */}
      {hasOpenEditorBuffer && (
        <div
          ref={editorPortalTargetRef}
          className="absolute inset-0"
          style={isEditorTabActive ? undefined : { visibility: 'hidden' }}
        />
      )}

      <Suspense fallback={null}>
        {activeBuffer &&
          activeBuffer.type !== 'terminal' &&
          activeBuffer.type !== 'editor' &&
          renderPaneContent(activeBuffer, { isActivePane })}
      </Suspense>
    </>
  )
}

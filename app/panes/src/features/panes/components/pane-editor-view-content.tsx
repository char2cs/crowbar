import { lazy, Suspense, useEffect, useMemo, useRef } from 'react'
import { useBuffersByIds } from '@/features/workspace/stores/hooks/use-buffer-store'
import type {
  BrowserContent,
  EditorContent,
  PaneContent,
  TerminalContent,
} from '../types/pane-content'
import { clearEditorPortalEntry, setEditorPortalEntry } from '../lib/editor-portal-registry'
import { NewTabView } from './new-tab-view'
import { renderPaneContent } from './pane-content-registry'
import { sidebarSurfaceOf, sidebarSurfaceRoot } from '../lib/sidebar-surface'
import { TerminalPane } from './terminal-pane'

const BrowserPane = lazy(() =>
  import('@/features/browser/components/browser-pane').then((m) => ({ default: m.BrowserPane })),
)

type EditorBufferShell = Pick<EditorContent, 'id' | 'path' | 'name' | 'type'>
type PaneRenderBuffer = Exclude<PaneContent, EditorContent> | EditorBufferShell

interface PaneEditorViewContentProps {
  paneId: string
  wsId: string | null
  editorTabIds: string[]
  activeEditorTabId: string | null | undefined
  editorViewHidden: boolean
  isActivePane: boolean
  showing: boolean
  dropTargeted?: boolean
}

export function PaneEditorViewContent({
  paneId,
  wsId,
  editorTabIds,
  activeEditorTabId,
  editorViewHidden,
  isActivePane,
  showing,
  dropTargeted = false,
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
            } satisfies EditorBufferShell)
          : buffer,
      ),
    [rawPaneBuffers],
  )
  const activeBuffer = useMemo(() => {
    if (!activeEditorTabId) return null
    return paneBuffers.find((b) => b.id === activeEditorTabId) || null
  }, [paneBuffers, activeEditorTabId])

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
      isActiveSurface: isEditorTabActive && isActivePane,
    })
    return () => clearEditorPortalEntry(paneId)
  }, [paneId, hasOpenEditorBuffer, activeEditorBuffer?.id, isEditorTabActive, isActivePane])

  return (
    <>
      {!activeBuffer && <NewTabView paneId={paneId} wsId={wsId} />}

      {paneBuffers
        .filter((b): b is TerminalContent => b.type === 'terminal')
        .map((b) => {
          const isActive = b.id === activeBuffer?.id && !editorViewHidden
          return (
            <div
              key={b.id}
              {...sidebarSurfaceRoot(sidebarSurfaceOf(b.type))}
              className="absolute inset-0 outline-none"
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
                isVisible={isActive && showing}
              />
            </div>
          )
        })}

      {hasOpenEditorBuffer && (
        <div
          ref={editorPortalTargetRef}
          {...sidebarSurfaceRoot(sidebarSurfaceOf('editor'))}
          className="absolute inset-0 outline-none"
          style={isEditorTabActive ? undefined : { visibility: 'hidden' }}
        />
      )}

      {paneBuffers
        .filter((b): b is BrowserContent => b.type === 'browser')
        .map((b) => {
          const isActive = b.id === activeBuffer?.id && !editorViewHidden
          return (
            <div
              key={b.id}
              {...sidebarSurfaceRoot(sidebarSurfaceOf(b.type))}
              className="absolute inset-0 outline-none"
              style={isActive ? undefined : { visibility: 'hidden' }}
            >
              <Suspense fallback={null}>
                <BrowserPane
                  paneId={paneId}
                  bufferId={b.id}
                  visible={isActive && showing && !dropTargeted}
                />
              </Suspense>
            </div>
          )
        })}

      <Suspense fallback={null}>
        {activeBuffer &&
          activeBuffer.type !== 'terminal' &&
          activeBuffer.type !== 'browser' &&
          activeBuffer.type !== 'editor' && (
            <div
              {...sidebarSurfaceRoot(sidebarSurfaceOf(activeBuffer.type))}
              className="absolute inset-0 outline-none"
            >
              {renderPaneContent(activeBuffer, { isActivePane, paneId, wsId })}
            </div>
          )}
      </Suspense>
    </>
  )
}

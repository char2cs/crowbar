import { memo, useEffect, useState } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import type { WorkspaceStore } from '../stores/workspace-store'
import { hydrateWorkspace } from '@/lib/persistence/hydrate'
import { markStart, markEnd } from '@/lib/perf/instrumentation'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'
import { useWorkspaceAgentChatsStream } from '../stores/hooks/use-workspace-agent-chats-stream'
import { useSaveKeyboard } from '@/features/keymaps/hooks/use-save-keyboard'
import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'
import { useZoomKeyboard } from '@/features/keymaps/hooks/use-zoom-keyboard'

interface WorkspaceViewProps {
  wsId: string
  /** Mounted by `WorkspaceHost` before this renders (C6) — never minted here. */
  store: WorkspaceStore
  /**
   * Whether this workspace is the one currently in view. WorkspaceHost keeps
   * recently-visited workspaces mounted (hidden via `display:none`) so switching
   * back is instant; only the active one owns the keyboard handlers and the
   * file/git watchers. (The active-workspace id is the host's to write.)
   */
  active: boolean
}

/**
 * A workspace's LIFECYCLE — its store, its hydration, its streams and its
 * active-only watchers. It renders no surface of its own: the pane tree it used
 * to host is window-level (Task 26) and now lives once, in `WorkspaceHost`.
 *
 * MEMOIZED because `WorkspaceHost`'s parent (`IDEShell`) re-renders on plenty
 * that has nothing to do with any workspace, and both props here are primitives
 * — `wsId` is fixed for the life of the instance, so this bails out on exactly
 * the renders that change nothing and still runs the two slots whose `active`
 * actually flips on a workspace switch.
 */
export const WorkspaceView = memo(function WorkspaceView({
  wsId,
  store,
  active,
}: WorkspaceViewProps) {
  // wsId is stable for a given WorkspaceView instance — WorkspaceHost keys each
  // retained workspace by id — so this hydrates exactly once per mount and never
  // re-hydrates on a warm re-activation.
  const [hydrated, setHydrated] = useState(false)

  // Live for as long as this workspace is mounted, not just while active:
  // Recents, "anything running has a row" and panes holding a hidden
  // workspace's chat all read this workspace's chats while it is hidden.
  useWorkspaceAgentChatsStream(wsId)

  // Cold path: hydrate once on mount. A workspace only mounts when it first
  // becomes active, so this also opens the workspace.switch span for the cold
  // switch that brought it into view (closed below once hydration paints).
  // Destruction is NOT wired here anymore — WorkspaceHost destroys the store on
  // eviction/close.
  useEffect(() => {
    markStart('workspace.switch')
    let cancelled = false
    hydrateWorkspace(wsId)
      .then(() => {
        if (!cancelled) setHydrated(true)
      })
      .catch(() => {
        if (!cancelled) setHydrated(true)
      })
    return () => {
      cancelled = true
    }
  }, [wsId])

  // Cold markEnd: rAF defers past the first paint after hydration lands so the
  // span covers hydrate-to-pixels (M4 cold switch).
  useEffect(() => {
    if (!hydrated) return
    const raf = requestAnimationFrame(() => markEnd('workspace.switch'))
    return () => cancelAnimationFrame(raf)
  }, [hydrated])

  // Warm path (M4 warm switch): a hidden -> active flip is only a prop change.
  // No re-hydration, remount or refetch happens; the `workspace.switch` span for
  // it is owned by WorkspaceHost, which alone knows the target was retained.

  if (!hydrated) return null

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceDataFeeds wsId={wsId} active={active} />
      {active && <WorkspaceKeyboard />}
    </WorkspaceStoreContext.Provider>
  )
})

// Each workspace's tree and git slice stay loaded and subscribed while it is
// retained, so a switch changes which store the UI reads and nothing else.
function WorkspaceDataFeeds({ wsId, active }: Pick<WorkspaceViewProps, 'wsId' | 'active'>) {
  useWorkspaceEffects(wsId, active)
  return null
}

function WorkspaceKeyboard() {
  useSaveKeyboard()
  usePaneKeyboard()
  useSidebarTabKeyboard()
  useZoomKeyboard()
  return null
}

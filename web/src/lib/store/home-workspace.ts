import { create } from 'zustand'
import { fetchHomeWorkspace } from '@/lib/api'
import { wsManager } from '@/lib/ws/manager'
import type { WorkspaceDTO } from '@/lib/types'

// The project-home workspace is the ONE workspace the live workspaces entity
// stream cannot carry. That stream is mounted per-REPO
// (/v0/projects/:p/repos/:r/workspaces) and the Workspaces topic namespaces a
// frame as `projectId/repoId/id` — but a home workspace has no repoId, so its
// frames ("p//id") can never prefix-match a repo-scoped "p/r" subscription.
// The daemon does broadcast them; no client could ever receive them. The home
// route therefore read the DTO exactly once, and its `working` flag stayed
// frozen at page-load value — leaving the context pill and the sidebar home row
// with no spinner while an agent worked in project home.
//
// The signal is an AGENT TURN, and only an agent turn: `working` is the OR of
// the daemon's inflight-mutation overlay and its agent-turn overlay, and the
// inflight overlay only ever covers worktree git mutations (create / delete /
// sync / merge / reparent) — none of which exist for a home workspace. The home
// agent-chat lifecycle WS is already mounted project-scoped
// (/v0/projects/:p/home/chats/ws) and already pushes turn_started /
// turn_stopped for exactly this workspace, so it is the signal we listen on.
//
// On each turn transition we RE-READ the authoritative DTO rather than deriving
// working from the chat frames ourselves: GET /home stamps `working` from the
// very same overlay the WS frames are enriched from, so a re-read agrees with
// the daemon by construction and cannot drift into a second, divergent notion
// of "working".

interface HomeWorkspaceStore {
  /** The active project's home workspace; null until the first read resolves. */
  workspace: WorkspaceDTO | null
  setHomeWorkspace: (workspace: WorkspaceDTO | null) => void
}

export const useHomeWorkspaceStore = create<HomeWorkspaceStore>()((set) => ({
  workspace: null,
  setHomeWorkspace: (workspace) => set({ workspace }),
}))

// Agent-chat lifecycle kinds that can move the workspace's `working` overlay.
// turn_started/turn_stopped are the transitions themselves; `deleted` matters
// because the daemon clears the overlay for a chat forgotten MID-TURN, so a
// delete can be what drops the workspace back to idle (without it the spinner
// would wedge on until the next turn).
const WORKING_KINDS = new Set(['turn_started', 'turn_stopped', 'deleted'])

/**
 * Keep `useHomeWorkspaceStore` tracking the given project's home workspace:
 * seed it with a GET, then re-read on every agent-chat frame that can flip the
 * working overlay. Returns the teardown (used per active project by
 * AppSyncProvider, which re-subscribes on every project switch).
 */
export function subscribeHomeWorkspace(projectId: string): () => void {
  let disposed = false
  // A turn fires two frames in quick succession (turn_started, turn_stopped), each
  // issuing its own GET. Those GETs can resolve OUT OF ORDER, and a re-read "agrees with
  // the daemon by construction" only for the response's own value — not against a second,
  // still-in-flight read. If the started-read's response lands last it would settle
  // working=true after the stopped-read already settled working=false, wedging the home
  // spinner on until the next turn. Only the most-recently ISSUED read may write.
  let latestRead = 0

  async function read(): Promise<void> {
    const seq = ++latestRead
    try {
      const workspace = await fetchHomeWorkspace(projectId)
      if (disposed || seq !== latestRead) return
      useHomeWorkspaceStore.getState().setHomeWorkspace(workspace)
    } catch {
      // A transient failure leaves the last known value in place; the next
      // frame (or the next reconnect reseed) re-reads.
    }
  }

  void read()

  const unsubscribe = wsManager.subscribe(`/v0/projects/${projectId}/home/chats/ws`, (frame) => {
    if (disposed) return
    // The reconnect sentinel is not a lifecycle frame — turns may have both
    // started and stopped while the socket was down, so re-read unconditionally.
    if (frame && typeof frame === 'object' && 'reconnected' in frame) {
      void read()
      return
    }
    const kind = (frame as { kind?: string } | null)?.kind
    if (kind !== undefined && WORKING_KINDS.has(kind)) void read()
  })

  return () => {
    disposed = true
    unsubscribe()
    // Drop the previous project's home so a switch can't leave its spinner (or
    // its name) on screen under the new project.
    useHomeWorkspaceStore.getState().setHomeWorkspace(null)
  }
}

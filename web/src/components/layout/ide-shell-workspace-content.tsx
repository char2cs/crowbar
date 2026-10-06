import { Outlet } from '@tanstack/react-router'
import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { getKnownHomeWorkspaceIds } from '@/features/workspace/lib/home-workspace-resolver'

/**
 * `IDEShell`'s main content column — `WorkspaceHost` plus the router
 * `Outlet` — split out of `IDEShell` itself since this whole subtree only
 * ever needs the three resolved workspace-id sets, none of the sidebar/route
 * state the rest of the shell carries.
 */
export function IdeShellWorkspaceContent({
  effectiveActiveWorkspaceId,
  paneWsIds,
  viewWsIds,
}: {
  effectiveActiveWorkspaceId: string | null
  paneWsIds: string[]
  viewWsIds: string[]
}) {
  return (
    // Every pane in here is its OWN drop target now (PaneContainer spreads
    // `PANE_DROP_ATTR` per pane, keyed by pane id) — spec §8.1's four-target
    // table, not the one whole-content "drop anywhere in here to remove it"
    // zone this div used to be (Task 22 deleted that dwell-to-remove gesture
    // along with `editor-removal-overlay.tsx`).
    <div className="relative z-[1] flex h-full min-w-0 flex-col bg-transparent">
      <ErrorBoundary>
        {/* WorkspaceHost stays mounted for the whole IDE session — including on
            the project-home route. Unmounting the host on every home visit
            destroyed all keep-alive retention (stores, terminals, Monaco
            models) — so returning to a workspace was a full COLD re-mount
            every time. Keeping the host mounted lets it retain
            recently-visited workspaces (all hidden) across home transits, so
            the return is warm.

            On the home route, `effectiveActiveWorkspaceId` is the resolved
            home workspace (once known) — the host renders ITS WorkspaceView
            too, as just another retained slot, instead of HomeRoute
            cold-mounting a fresh one on every visit (that used to be ~2x the
            frame cost of a normal warm switch; see
            home-workspace-resolver.ts). `homeWsIds` protects every home
            workspace resolved so far this session from the existence-prune —
            home is a project-level concept, not in the sidebar's repo/
            workspace id set, so without this it would look "closed" the
            instant it goes hidden and get destroyed instead of retained.
            `paneWsIds` (see use-chat-workspace-id.ts and
            use-ide-shell-workspace-retention.ts, which unions it with
            `usePaneEditorWorkspaceIds`) does the same for every workspace a
            PANE currently holds a chat OR editor tab for — not just the one
            that's "active" — so a split's other pane(s) always get a real
            store instead of falling back to the wrong ambient one; a pane
            naming neither was invisible to retention entirely, which is what
            let `planRetention` destroy a workspace still displaying an open
            file/terminal split ("Editor failed to load"). `viewWsIds`
            (`useViewWorkspaceIds`) is the host's actual retention test now:
            every workspace with a chat somewhere in Recents stays mounted,
            and dropping out of `viewWsIds` is what gets a workspace evicted
            — no more time-based keep-alive window. HomeRoute itself renders
            null (or the error state); the Outlet still stays mounted so
            workspace-route components' route-level guards keep running. */}
        <WorkspaceHost
          activeWsId={effectiveActiveWorkspaceId}
          homeWsIds={getKnownHomeWorkspaceIds()}
          paneWsIds={paneWsIds}
          viewWsIds={viewWsIds}
        />
        <Outlet />
      </ErrorBoundary>
    </div>
  )
}

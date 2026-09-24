import { hydrateSidebar, hydrateWindowPaneLayout } from '@/lib/persistence/hydrate'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { dataOf } from '@/lib/loadable'

/**
 * Hydrates whatever this window's pane/buffer layout and sidebar tree were
 * last time, from IndexedDB — NOT the network. `main.tsx` awaits this BEFORE
 * `renderApp()` is ever called, outside React entirely, so React's very
 * first commit already has the real layout in hand instead of mounting once
 * against empty defaults and again a frame later against the restored one.
 *
 * That "mount empty, then swap" transition is not just a visual flash: a
 * `WorkspaceView`/`EditorSurface` that mounts while `windowPaneStore` is
 * still at its just-booted defaults sees no workspace, no buffer — then
 * `hydrateWindowPaneLayout`'s `setState` replaces panes/buffers/layout out
 * from under it. Caught live as a wave of "Editor failed to load"
 * ErrorBoundary trips, every one of them `EditorSurface` throwing on a
 * workspace store that hadn't been armed for the pane it was already
 * rendering — the instant that swap landed a frame after an old,
 * gate-everything-on-the-network HydrationGate was replaced with rendering
 * immediately and hydrating in the background.
 *
 * Every step here is a plain local IndexedDB read — `hydrateWindowPaneLayout`
 * (the window layout row), and
 * `useWorkspaceListStore`'s own `fetch()` (`readVisibleRepoTree`, which reads
 * the entity cache — see project-visibility.ts — never the network). None of
 * this is a real backend round trip, so awaiting it here costs single-digit-
 * to low-tens of milliseconds, not the 600-900ms a network-gated boot used to
 * spend blank. `hydrateSidebar` alone has a genuine ordering dependency (it
 * applies hierarchy overrides on top of `s.repos` — see its own body), so it
 * waits on `setRepos` completing, not on anything else here.
 */
export async function hydrateCriticalStores(): Promise<void> {
  await hydrateWindowPaneLayout()
  await useWorkspaceListStore.getState().fetch()
  useSidebarStore.getState().setRepos(dataOf(useWorkspaceListStore.getState().data) ?? [])
  await hydrateSidebar()
}

/**
 * The one genuinely slow boot step — a real `/v0/projects` network round
 * trip — reconciles into `useProjectStore` reactively once it resolves, same
 * as any other live update reaching it. Nothing waits on this, and nothing
 * needs to: no component's first mount depends on the project LIST existing,
 * only on the single active project id (already resolved from the route).
 */
export function hydrateProjectsInBackground(): void {
  void useProjectDataStore
    .getState()
    .fetch()
    .then(() => {
      useProjectStore.getState().setProjects(dataOf(useProjectDataStore.getState().data) ?? [])
    })
}

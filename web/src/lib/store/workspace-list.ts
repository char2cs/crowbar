import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import type { Repo } from '@/lib/store/sidebar'
import { readVisibleRepoTree } from '@/lib/store/project-visibility'

// §6/§7: the sidebar tree is now derived from the WS-driven entity cache, not a
// flat cross-project GET. `fetch` reads the per-entity object stores
// (crowbar_repos + crowbar_workspaces) and groups them into the nested Repo[]
// the sidebar renders. The per-repo GET seed + live WS subscription that keep
// the cache fresh live in app-sync-provider's §7 startup (subscribeEntityStream
// over `/v0/projects/:p/repos/:r/workspaces`), so this store no longer owns a
// `/v0/ws/workspaces` subscription — `startSync` is a no-op and the live tree
// is refreshed by calling `fetch()` whenever the cache changes.
//
// The entity cache is intentionally CROSS-PROJECT and long-lived: each project's
// repo stream prunes only its own scope, so a project the user is not currently
// looking at keeps its cached rows and re-renders instantly (and offline) when
// expanded. The tree is therefore scoped to the VISIBLE projects — the active
// one plus any the user has expanded — rather than to the single active project
// it used to be locked to. See lib/store/project-visibility.ts.
export const useWorkspaceListStore = create<LoadableSlice<Repo[], []>>()((set, get) =>
  createLoadableSlice<Repo[], []>({
    store: 'workspaces-data',
    fetcher: readVisibleRepoTree,
    cacheKey: () => 'workspaces',
  })(set, get),
)

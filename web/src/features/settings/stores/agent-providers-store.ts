import { create } from 'zustand'
import { listProviders } from '@/features/agent/api/agent-api'
import type { AgentProvider } from '@/features/agent/api/agent-api'

/**
 * What the app knows about the agent providers RIGHT NOW — kept apart from
 * `providers` and `[]` because those two answers together cannot say anything
 * useful. `ready` + empty is the daemon reporting none; `failed` is Crowbar not
 * having been able to ask. Rendering "No providers available." for both was
 * actively misleading — the live report that produced this store was a user
 * reading that sentence with both providers installed and enabled on disk.
 */
export type AgentProvidersStatus = 'idle' | 'loading' | 'ready' | 'failed'

interface AgentProvidersState {
  status: AgentProvidersStatus
  providers: AgentProvider[]
  /**
   * Adopt a list resolved somewhere else — a workspace's own seed, or the
   * response to a preferences PUT (which returns the freshly resolved set).
   */
  setProviders: (providers: AgentProvider[]) => void
  /**
   * Fetch the list through `wsId`. The daemon exposes providers only under a
   * workspace or project-home scope, even though the data itself is global
   * ("workspaceID is only used to resolve crowbar home" — usecases/agent), so a
   * caller with no scope cannot load and calls markUnavailable instead.
   *
   * Returns the resolved list so the caller can mirror it into the workspace
   * store the chat surfaces read, or null when this load did not win.
   */
  load: (wsId: string) => Promise<AgentProvider[] | null>
  /** No scope to ask through. Say we could not tell — never that there are none. */
  markUnavailable: () => void
}

// Only the most-recently ISSUED load may write, exactly as loadable-slice's
// `latestFetch` does: resolution order is not issue order, and an older answer
// landing last would reinstate a list the user has already moved past.
let latestLoad = 0

/**
 * The GLOBAL provider list. Providers are machine-level, but the frontend's only
 * copy used to live inside the per-WORKSPACE store — so every surface that is
 * not workspace-scoped (the Settings dialog, most of all) had to guess. With no
 * active workspace there was nothing to read and the Providers tab said the
 * daemon had none.
 *
 * The per-workspace copy still exists and still feeds the chat surfaces; this is
 * the one that survives having no workspace in view. Both are written from the
 * same two events — a workspace seed and a preferences PUT — so they cannot
 * disagree about what the server last said.
 */
export const useAgentProvidersStore = create<AgentProvidersState>((set, get) => ({
  status: 'idle',
  providers: [],

  setProviders: (providers) => set({ providers, status: 'ready' }),

  load: async (wsId) => {
    const seq = ++latestLoad
    // Only a load with no settled answer behind it may show a spinner. `ready`
    // is settled even when the list is EMPTY — "the daemon has none" is an
    // answer — so a refresh keeps showing what it says instead of strobing
    // between a spinner and a sentence. A retry after `failed` does show it.
    if (get().status !== 'ready') set({ status: 'loading' })
    try {
      const providers = await listProviders(wsId)
      if (seq !== latestLoad) return null
      set({ providers, status: 'ready' })
      return providers
    } catch {
      if (seq !== latestLoad) return null
      // A failed REFRESH must not empty a good list — the data we hold is still
      // the last thing the server said.
      set({ status: get().providers.length > 0 ? 'ready' : 'failed' })
      return null
    }
  },

  markUnavailable: () => {
    if (get().providers.length > 0) return
    set({ status: 'failed' })
  },
}))

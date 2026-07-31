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
 * THE WRITE GENERATION. Bumped once per preference WRITE, at the moment the
 * write is issued, and read by everything that publishes a provider list.
 *
 * It exists because sequencing writes against writes is only half the problem.
 * A GET is a snapshot of the server BEFORE the PUT it overlaps, so a read issued
 * first and answered second re-installs the pre-write list — the switch the user
 * just moved visibly flips back, and the next write PUTs that stale flag. The
 * mount refetch and the workspace-stream reseed are both such reads.
 *
 * A READ therefore never bumps this; it captures the value it started under and
 * publishes only if nothing has been written since. That direction is the whole
 * design: a read that bumped would invalidate the in-flight write it raced and
 * then install its own stale answer, which is the same bug with the roles
 * swapped.
 */
let providerWrites = 0

/** Claim a generation for a write about to be issued. */
export const beginProviderWrite = (): number => ++providerWrites

/** The generation a read is starting under; pair with isLatestProviderWrite. */
export const providerWriteGeneration = (): number => providerWrites

/**
 * Whether `seq` is still current — for a write, that it has not been superseded;
 * for a read, that no write has been issued since it asked.
 */
export const isLatestProviderWrite = (seq: number): boolean => seq === providerWrites

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
    // The write generation this read is a snapshot of. Anything written after
    // this line describes a server this answer has not seen.
    const writes = providerWriteGeneration()
    // Only a load with no settled answer behind it may show a spinner. `ready`
    // is settled even when the list is EMPTY — "the daemon has none" is an
    // answer — so a refresh keeps showing what it says instead of strobing
    // between a spinner and a sentence. A retry after `failed` does show it.
    if (get().status !== 'ready') set({ status: 'loading' })
    try {
      const providers = await listProviders(wsId)
      if (seq !== latestLoad) return null
      // A write landed while this was in flight. Its own optimistic state — and
      // the response it reconciles from — are newer than this snapshot, so
      // publishing here would undo it in front of the user: the switch they just
      // moved flips back, and the next write PUTs the flag this list carries.
      // The status still settles: we did reach the daemon, and leaving it on
      // `loading` would strand the tab's spinner.
      if (!isLatestProviderWrite(writes)) {
        set({ status: 'ready' })
        return null
      }
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

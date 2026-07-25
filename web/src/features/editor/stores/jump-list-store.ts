import isEqual from 'fast-deep-equal'
import { immer } from 'zustand/middleware/immer'
import { createWithEqualityFn } from 'zustand/traditional'
import { createSelectors } from '@/utils/zustand-selectors'

export interface JumpListEntry {
  bufferId: string
  /**
   * The workspace this entry was recorded in.
   *
   * This store is a process-GLOBAL singleton that outlives every workspace
   * switch (`resetWorkspaceScopedStores` does not clear it), while `filePath`
   * is workspace-RELATIVE. Linked worktrees of one repo hold the same relative
   * paths with different content, so an entry that does not name its own
   * workspace is ambiguous: resolving it against whatever is active at
   * navigation time opens a sibling checkout's file under the right tab title.
   * `navigateToJumpEntry` refuses any entry whose workspace is not the active
   * one rather than guessing.
   */
  workspaceId: string
  filePath: string
  line: number
  column: number
  offset: number
  scrollTop: number
  scrollLeft: number
  timestamp: number
}

interface JumpListActions {
  pushEntry: (entry: Omit<JumpListEntry, 'timestamp'>) => void
  goBack: (currentPosition?: Omit<JumpListEntry, 'timestamp'>) => JumpListEntry | null
  goForward: () => JumpListEntry | null
  canGoBack: () => boolean
  canGoForward: () => boolean
  clear: () => void
  /**
   * Consume the pending "this activation came from Back/Forward" marker.
   * Returns true if `bufferId` is the buffer history navigation just moved to,
   * in which case the caller must NOT record it as a new navigation.
   */
  consumeNavigationTarget: (bufferId: string) => boolean
  /**
   * Re-point the pending marker at the buffer the target file was actually
   * reopened under.
   *
   * The marker names a BUFFER id, but an entry whose tab was closed can only be
   * satisfied by reading the file back from disk — and that mints a NEW id. The
   * recorder would then fail to recognise the activation it sees, record it as a
   * fresh navigation, and `pushEntry` would truncate the forward branch: one
   * Back into a closed file permanently disabled Forward and sent the next Back
   * to the file the user started from. Only moves the marker — the entry, and
   * therefore its workspace stamp, is untouched.
   *
   * A no-op when no navigation is pending, so an ordinary open can't fabricate
   * a handshake that swallows the next genuine record.
   */
  retargetNavigation: (bufferId: string) => void
  /**
   * Drop the pending marker. Navigation that could not be completed must not
   * leave one behind: it would silently suppress the next genuine visit to that
   * buffer id, long after the failed Back is forgotten.
   */
  clearNavigationTarget: () => void
}

interface JumpListState {
  entries: JumpListEntry[]
  currentIndex: number
  maxEntries: number
  /**
   * Buffer that goBack/goForward last navigated to, pending acknowledgement by
   * the history recorder.
   *
   * Back/Forward activate a buffer, which the recorder observes one render later
   * and would otherwise push as a brand-new navigation — making it impossible to
   * go back twice. A boolean "isNavigating" flag can't express this: it would
   * have to be cleared on a timer racing that render. Naming the target buffer
   * makes the handshake explicit and timing-independent.
   */
  navigationTargetBufferId: string | null
  actions: JumpListActions
}

const DEFAULT_MAX_ENTRIES = 100
const DUPLICATE_LINE_THRESHOLD = 5

export const useJumpListStore = createSelectors(
  createWithEqualityFn<JumpListState>()(
    immer((set, get) => ({
      entries: [],
      currentIndex: -1,
      maxEntries: DEFAULT_MAX_ENTRIES,
      navigationTargetBufferId: null,

      actions: {
        pushEntry: (entry) => {
          set((state) => {
            const newEntry: JumpListEntry = {
              ...entry,
              timestamp: Date.now(),
            }

            // If we're in the middle of history, truncate future entries
            if (state.currentIndex >= 0 && state.currentIndex < state.entries.length - 1) {
              state.entries = state.entries.slice(0, state.currentIndex + 1)
            }

            // Check for duplicate (same file and within line threshold)
            const lastEntry = state.entries[state.entries.length - 1]
            if (lastEntry) {
              const isSameFile = lastEntry.filePath === newEntry.filePath
              const isNearbyLine =
                Math.abs(lastEntry.line - newEntry.line) <= DUPLICATE_LINE_THRESHOLD

              if (isSameFile && isNearbyLine) {
                // Update the existing entry instead of adding a duplicate
                state.entries[state.entries.length - 1] = newEntry
                state.currentIndex = -1
                return
              }
            }

            // Add the new entry
            state.entries.push(newEntry)

            // Enforce max size
            if (state.entries.length > state.maxEntries) {
              state.entries.shift()
            }

            // Reset to present (not navigating history)
            state.currentIndex = -1
          })
        },

        goBack: (currentPosition) => {
          const state = get()

          if (state.entries.length === 0) {
            return null
          }

          let newIndex: number
          if (state.currentIndex === -1) {
            // Currently at present - save current position so we can go forward to it
            if (currentPosition) {
              set((s) => {
                s.entries.push({
                  ...currentPosition,
                  timestamp: Date.now(),
                })
                // Enforce max size
                if (s.entries.length > s.maxEntries) {
                  s.entries.shift()
                }
              })
            }
            // Go to second-to-last entry (last entry is now where we just were)
            newIndex = get().entries.length - 2
          } else if (state.currentIndex > 0) {
            // Go to previous entry
            newIndex = state.currentIndex - 1
          } else {
            // Already at the beginning
            return null
          }

          if (newIndex < 0) return null

          const entry = get().entries[newIndex]
          if (!entry) return null

          set((s) => {
            s.currentIndex = newIndex
            s.navigationTargetBufferId = entry.bufferId
          })

          return entry
        },

        goForward: () => {
          const state = get()

          if (state.currentIndex === -1 || state.currentIndex >= state.entries.length - 1) {
            return null
          }

          const newIndex = state.currentIndex + 1
          const entry = state.entries[newIndex]
          if (!entry) return null

          set((s) => {
            s.currentIndex = newIndex
            s.navigationTargetBufferId = entry.bufferId
          })

          return entry
        },

        canGoBack: () => {
          const state = get()
          if (state.entries.length === 0) return false
          if (state.currentIndex === -1) return true
          return state.currentIndex > 0
        },

        canGoForward: () => {
          const state = get()
          if (state.currentIndex === -1) return false
          return state.currentIndex < state.entries.length - 1
        },

        clear: () => {
          set((state) => {
            state.entries = []
            state.currentIndex = -1
            state.navigationTargetBufferId = null
          })
        },

        consumeNavigationTarget: (bufferId) => {
          if (get().navigationTargetBufferId !== bufferId) return false
          set((state) => {
            state.navigationTargetBufferId = null
          })
          return true
        },

        retargetNavigation: (bufferId) => {
          if (get().navigationTargetBufferId === null) return
          set((state) => {
            state.navigationTargetBufferId = bufferId
          })
        },

        clearNavigationTarget: () => {
          if (get().navigationTargetBufferId === null) return
          set((state) => {
            state.navigationTargetBufferId = null
          })
        },
      },
    })),
    isEqual,
  ),
)

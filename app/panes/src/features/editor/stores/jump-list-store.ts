import isEqual from 'fast-deep-equal'
import { immer } from 'zustand/middleware/immer'
import { createWithEqualityFn } from 'zustand/traditional'
import { createSelectors } from '@/utils/zustand-selectors'
import type { DocumentRef } from '@/lib/api/documents'
import { sameDocument } from '@/features/editor/documents/document-ref'

interface JumpListEntry {
  bufferId: string
  doc: DocumentRef
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
  consumeNavigationTarget: (bufferId: string) => boolean
  retargetNavigation: (bufferId: string) => void
  clearNavigationTarget: () => void
}

interface JumpListState {
  entries: JumpListEntry[]
  currentIndex: number
  maxEntries: number
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

            if (state.currentIndex >= 0 && state.currentIndex < state.entries.length - 1) {
              state.entries = state.entries.slice(0, state.currentIndex + 1)
            }

            const lastEntry = state.entries[state.entries.length - 1]
            if (lastEntry) {
              const isSameFile = sameDocument(lastEntry.doc, newEntry.doc)
              const isNearbyLine =
                Math.abs(lastEntry.line - newEntry.line) <= DUPLICATE_LINE_THRESHOLD

              if (isSameFile && isNearbyLine) {
                state.entries[state.entries.length - 1] = newEntry
                state.currentIndex = -1
                return
              }
            }

            state.entries.push(newEntry)

            if (state.entries.length > state.maxEntries) {
              state.entries.shift()
            }

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
            if (currentPosition) {
              set((s) => {
                s.entries.push({
                  ...currentPosition,
                  timestamp: Date.now(),
                })
                if (s.entries.length > s.maxEntries) {
                  s.entries.shift()
                }
              })
            }
            newIndex = get().entries.length - 2
          } else if (state.currentIndex > 0) {
            newIndex = state.currentIndex - 1
          } else {
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

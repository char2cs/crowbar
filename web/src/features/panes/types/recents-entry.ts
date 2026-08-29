// Store-safe home for RecentsEntry: pane-slice.ts (a store) holds
// `dormantArrangements: RecentsEntry[]`, and stores must not import from
// components/ — see CLAUDE.md. `components/sidebar/recents-band.tsx`
// re-exports these for its existing importers.

export type RecentsEntryState = 'live' | 'working' | 'set' | 'dormant'

export interface RecentsEntry {
  /** Keyed by the view's identity, not by state — spec §5.6. */
  id: string
  /** One chat id for a lone entry, 2+ for a set. */
  chatIds: string[]
  state: RecentsEntryState
}

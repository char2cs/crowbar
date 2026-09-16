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
  /**
   * Whether this entry's view is the one ON SCREEN right now.
   *
   * `state: 'live'` means the entry has a pane; with views, several entries
   * can be live at once and only one of them is showing. That distinction is
   * what makes the band a switcher rather than a list — without it every open
   * view would wear the active row's treatment and "you are here" would be
   * unreadable.
   *
   * A POSITIVE MARKER: present and `true` for the one view on screen, absent
   * for every other entry. Not a `false` anybody has to write — a stored
   * `dormantArrangements` record is a record of what is NOT up and would go
   * stale the instant the user switched views, so it carries no answer at all,
   * and neither does an entry derived without an active view to compare
   * against. Read it as `entry.showing === true`.
   *
   * DERIVED AT READ TIME ONLY, by `deriveRecentsEntries`.
   */
  showing?: boolean
}

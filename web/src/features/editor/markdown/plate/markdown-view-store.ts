import { create } from 'zustand'

export type MarkdownView = 'rich' | 'source'

interface MarkdownViewState {
  views: Record<string, MarkdownView>
  setView: (bufferId: string, view: MarkdownView) => void
  toggleView: (bufferId: string) => void
  clearView: (bufferId: string) => void
}

export const useMarkdownViewStore = create<MarkdownViewState>((set) => ({
  views: {},
  setView: (bufferId, view) => set((s) => ({ views: { ...s.views, [bufferId]: view } })),
  toggleView: (bufferId) =>
    set((s) => ({
      views: {
        ...s.views,
        [bufferId]: (s.views[bufferId] ?? 'rich') === 'rich' ? 'source' : 'rich',
      },
    })),
  // Called when a buffer closes (buffer-slice's closeBuffer) so `views` doesn't
  // accumulate an entry per markdown file ever opened. Returns the SAME state
  // when there's nothing to drop, so the common "closed a non-markdown buffer"
  // case doesn't churn a new object through every subscriber.
  clearView: (bufferId) =>
    set((s) => {
      if (!(bufferId in s.views)) return s
      const next = { ...s.views }
      delete next[bufferId]
      return { views: next }
    }),
}))

/** Selector: the view mode for a buffer, defaulting to `rich`. */
export const selectMarkdownView =
  (bufferId: string) =>
  (state: MarkdownViewState): MarkdownView =>
    state.views[bufferId] ?? 'rich'

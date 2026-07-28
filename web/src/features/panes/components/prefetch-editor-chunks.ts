import { scheduleIdleTask } from '@/features/editor/lib/idle-task'

// Warm the editor + diff pane chunks during idle time so the first file open
// doesn't pay the dynamic-import cost. Kept out of the pane-container component
// file so that file stays Fast-Refresh-safe.
export function prefetchEditorChunks(): void {
  scheduleIdleTask(() => {
    void import('./editor-pane')
    void import('./commit-diff-pane')
  })
}

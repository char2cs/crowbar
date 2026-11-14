import { scheduleIdleTask } from '@/features/editor/lib/idle-task'

// Warm the editor pane chunk during idle time so the first file open
// doesn't pay the dynamic-import cost. Kept out of the pane-container component
// file so that file stays Fast-Refresh-safe.
export function prefetchEditorChunks(): void {
  scheduleIdleTask(() => {
    void prefetch(() => import('./editor-pane'))
  })
}

async function prefetch(load: () => Promise<unknown>): Promise<void> {
  try {
    await load()
  } catch {
    return // a prefetch that fails is only a cold first open; the pane's own load retries it
  }
}

import { scheduleIdleTask } from '@/features/editor/lib/idle-task'

export function prefetchEditorChunks(): void {
  scheduleIdleTask(() => {
    void prefetch(() => import('./editor-pane'))
  })
}

async function prefetch(load: () => Promise<unknown>): Promise<void> {
  try {
    await load()
  } catch {
    return
  }
}

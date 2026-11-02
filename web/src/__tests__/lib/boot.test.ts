import { describe, it, expect, vi, beforeEach } from 'vitest'
import { hydrateCriticalStores } from '@/lib/boot'

const { hydrateSidebar, hydrateWindowPaneLayout, placeRestoredPanesOnceSynced } = vi.hoisted(
  () => ({
    hydrateSidebar: vi.fn().mockResolvedValue(undefined),
    hydrateWindowPaneLayout: vi.fn().mockResolvedValue(undefined),
    placeRestoredPanesOnceSynced: vi.fn(),
  }),
)
vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateSidebar,
  hydrateWindowPaneLayout,
  placeRestoredPanesOnceSynced,
}))

// Regression: this ordering used to live inside a React component
// (`HydrationGate`) that gated first paint on ALL of it, including the one
// genuine network call (`/v0/projects`) — live-measured as a 600-900ms blank
// screen on every cold boot. Moving it here, awaited in main.tsx BEFORE
// `renderApp()` is ever called, fixed that AND a real crash the naive "just
// render immediately" fix introduced: a `WorkspaceView`/`EditorSurface` that
// mounts against `windowPaneStore`'s just-booted defaults, only to have
// `hydrateWindowPaneLayout` replace the whole layout out from under it a
// frame later, threw ("Editor failed to load") on every retained pane.
// `hydrateCriticalStores` must therefore fully resolve — synchronously, from
// the caller's perspective — before React ever mounts.
describe('hydrateCriticalStores', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('hydrates preferences and pane layout', async () => {
    await hydrateCriticalStores()

    expect(hydrateWindowPaneLayout).toHaveBeenCalledTimes(1)
  })

  // Restored members are placed once the projects scope answers, never before first paint.
  it('arms placing restored chat members after the layout', async () => {
    const order: string[] = []
    hydrateWindowPaneLayout.mockImplementation(async () => {
      order.push('layout')
    })
    placeRestoredPanesOnceSynced.mockImplementation(() => {
      order.push('place')
    })

    await hydrateCriticalStores()

    expect(order).toEqual(['layout', 'place'])
  })

  it('retires the storage earlier builds left behind', async () => {
    localStorage.setItem('crowbar:settings:editorEngine', '"monaco"')

    await hydrateCriticalStores()

    expect(localStorage.getItem('crowbar:settings:editorEngine')).toBeNull()
  })

  it('hydrates the sidebar UI after the layout', async () => {
    const order: string[] = []
    hydrateWindowPaneLayout.mockImplementation(async () => {
      order.push('layout')
    })
    hydrateSidebar.mockImplementation(async () => {
      order.push('hydrateSidebar')
    })

    await hydrateCriticalStores()

    expect(order).toEqual(['layout', 'hydrateSidebar'])
  })
})

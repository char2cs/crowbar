import { StrictMode } from 'react'
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useDrag } from 'react-dnd'
import { DndScope } from '@/features/agent/chat/dnd-scope'

function DragProbe() {
  useDrag(() => ({ type: 'thing' }))
  return null
}

/**
 * REGRESSION, reported live: opening a second thing that needs the drag-drop
 * context crashed the whole pane with "Cannot have two HTML5 backends at the
 * same time" — which also explains why attachment drag-reorder stopped
 * working entirely once it fired (the DndContext underneath both was gone).
 *
 * Root-caused by direct instrumentation (patching `HTML5BackendImpl.setup`
 * to log call counts and the flag it checks), not guessed: react-dnd's own
 * `DndProvider` (used with just `backend`, no explicit `manager`) tracks its
 * global-singleton `DragDropManager` via a ref-counted effect — and that
 * effect's OWN cleanup runs once, transiently, as part of every component's
 * FIRST mount under React 18 StrictMode's dev-only double-invoke (simulated
 * unmount, then remount, no real DOM change). That transient cleanup nulls
 * the singleton reference the instant refCount dips to 0, even though the
 * component is (from React's perspective) still mounting — and nothing ever
 * restores that reference afterward, since only render-time code (not the
 * effect) repopulates it. The component itself keeps working fine off its
 * own closed-over manager, so nothing looks wrong locally. The very next
 * DndProvider to mount anywhere, though, finds the singleton reference gone
 * and builds a BRAND NEW manager + HTML5Backend — whose `setup()` then finds
 * `window.__isReactDndBackendSetUp` already `true` from the first (still
 * alive, never torn down) backend, and throws.
 *
 * Confirmed via instrumentation that a single DndScope's own mount→unmount→
 * remount cycle settles fine (the backend's occupancy-based teardown happens
 * to reset the flag correctly when the same instance fully unmounts) — the
 * crash needs a genuinely SECOND DndScope mounting while the first is still
 * alive, which is exactly the shape of two chat panes/tabs in this app.
 */
describe('DndScope', () => {
  it('does not throw when a second DndScope mounts while the first is still alive', () => {
    render(
      <StrictMode>
        <DndScope>
          <DragProbe />
        </DndScope>
      </StrictMode>,
    )

    expect(() =>
      render(
        <StrictMode>
          <DndScope>
            <DragProbe />
          </DndScope>
        </StrictMode>,
      ),
    ).not.toThrow()
  })

  // AgentChatPane keeps other chats' surfaces mounted (hidden) rather than
  // unmounting them on tab switch — several DndScopes are routinely alive at
  // the same instant, sharing one backend rather than colliding.
  it('supports two DndScope instances mounted at the same time', () => {
    expect(() => {
      render(
        <StrictMode>
          <DndScope>
            <DragProbe />
          </DndScope>
          <DndScope>
            <DragProbe />
          </DndScope>
        </StrictMode>,
      )
    }).not.toThrow()
  })

  it('mounts, unmounts, and mounts again without throwing', () => {
    const { unmount } = render(
      <StrictMode>
        <DndScope>
          <DragProbe />
        </DndScope>
      </StrictMode>,
    )
    unmount()

    expect(() =>
      render(
        <StrictMode>
          <DndScope>
            <DragProbe />
          </DndScope>
        </StrictMode>,
      ),
    ).not.toThrow()
  })
})

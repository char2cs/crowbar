/**
 * `use-chat-presentation.ts` drives two splits: the terminal split hosted
 * inside `agent-chat-pane.tsx` (chat ⇄ terminal, unchanged by this file —
 * that behaviour is covered end-to-end by `agent-chat-pane-split.test.tsx`)
 * and, generalized here, the pane-level chat-view ⇄ editor-view split spec
 * §7.2 describes, consumed by `pane-container.tsx`.
 *
 * `usePaneViewPresentation` is pure geometry over `editorOpen` (Task 1's
 * `PaneGroup.editorOpen`) — there is nothing to "choose", so unlike
 * `useChatPresentation` it has no `setPresentation`/`chosen` pair to test.
 */
import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  SPLIT_SIDE_BY_SIDE_MIN_PX,
  usePaneViewPresentation,
} from '@/features/agent/hooks/use-chat-presentation'

function ref(size: { clientWidth?: number; clientHeight?: number }) {
  return { current: size as unknown as HTMLElement }
}

describe('usePaneViewPresentation', () => {
  it('is tabs when the split is off, regardless of size', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(false, ref({ clientWidth: 2000, clientHeight: 2000 })),
    )
    expect(result.current).toBe('tabs')
  })

  it('below the side-by-side floor on both axes, tabs is the only presentation even with the split on', () => {
    const { result } = renderHook(() => usePaneViewPresentation(true, ref({ clientWidth: 600 })))
    expect(result.current).toBe('tabs')
  })

  it('wide enough and landscape presents side by side', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(true, ref({ clientWidth: SPLIT_SIDE_BY_SIDE_MIN_PX + 100 })),
    )
    expect(result.current).toBe('side-by-side')
  })

  it('tall enough and portrait presents stacked', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(
        true,
        ref({ clientWidth: 500, clientHeight: SPLIT_SIDE_BY_SIDE_MIN_PX + 200 }),
      ),
    )
    expect(result.current).toBe('stacked')
  })

  it('an unmeasured (0x0) container defaults to side-by-side rather than flashing to tabs first', () => {
    const { result } = renderHook(() => usePaneViewPresentation(true, { current: null }))
    expect(result.current).toBe('side-by-side')
  })

  it('a square pane at exactly the floor is side by side (width >= height wins ties)', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(
        true,
        ref({
          clientWidth: SPLIT_SIDE_BY_SIDE_MIN_PX,
          clientHeight: SPLIT_SIDE_BY_SIDE_MIN_PX,
        }),
      ),
    )
    expect(result.current).toBe('side-by-side')
  })
})

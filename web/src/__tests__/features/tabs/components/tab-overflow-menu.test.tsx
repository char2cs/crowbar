import { createRef } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, onTestFinished, vi } from 'vitest'
import { TabOverflowMenu } from '@/features/tabs/components/tab-overflow-menu'

const tabs = [
  { id: 'a', name: 'alpha.ts' },
  { id: 'b', name: 'beta.ts' },
  { id: 'c', name: 'gamma.ts' },
]

/** jsdom has no layout: a scroller whose content is `content` px wide in a `box` px box. */
function scroller(content: number, box: number) {
  const el = document.createElement('div')
  Object.defineProperty(el, 'scrollWidth', { configurable: true, get: () => content })
  Object.defineProperty(el, 'clientWidth', { configurable: true, get: () => box })
  document.body.append(el)
  onTestFinished(() => el.remove())
  const ref = createRef<HTMLDivElement>()
  ;(ref as { current: HTMLDivElement | null }).current = el
  return { el, ref }
}

describe('TabOverflowMenu', () => {
  it('draws nothing while every tab fits', () => {
    const { ref } = scroller(300, 300)
    render(<TabOverflowMenu scrollRef={ref} tabs={tabs} activeId="a" onSelect={vi.fn()} />)
    expect(screen.queryByRole('button', { name: 'More tabs' })).not.toBeInTheDocument()
  })

  it('lists every tab when they do not fit, and picking one selects it', async () => {
    const onSelect = vi.fn()
    const { ref } = scroller(900, 300)
    render(<TabOverflowMenu scrollRef={ref} tabs={tabs} activeId="a" onSelect={onSelect} />)

    await userEvent.click(screen.getByRole('button', { name: 'More tabs' }))
    // base-ui mounts the popup after the click resolves, so wait for it.
    expect((await screen.findAllByRole('menuitem')).map((i) => i.textContent)).toEqual([
      'alpha.ts',
      'beta.ts',
      'gamma.ts',
    ])
    await userEvent.click(screen.getByRole('menuitem', { name: 'gamma.ts' }))
    expect(onSelect).toHaveBeenCalledWith('c')
  })

  it('appears once a tab is added that no longer fits', async () => {
    const { el, ref } = scroller(300, 300)
    const { rerender } = render(
      <TabOverflowMenu scrollRef={ref} tabs={tabs.slice(0, 2)} activeId="a" onSelect={vi.fn()} />,
    )
    Object.defineProperty(el, 'scrollWidth', { configurable: true, get: () => 450 })
    act(() =>
      rerender(<TabOverflowMenu scrollRef={ref} tabs={tabs} activeId="a" onSelect={vi.fn()} />),
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'More tabs' })).toBeInTheDocument(),
    )
  })
})

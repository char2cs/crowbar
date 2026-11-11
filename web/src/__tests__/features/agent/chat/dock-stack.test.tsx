import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DockDissolve, DockStack } from '@/features/agent/chat/dock-stack'

function slots() {
  return {
    topTray: <i data-testid="top" />,
    pickers: <i data-testid="pickers" />,
    notes: <i data-testid="notes" />,
    pill: <i data-testid="pill" />,
    bottomTray: <i data-testid="bottom" />,
    footer: <i data-testid="footer" />,
    alert: <i data-testid="alert" />,
  }
}

describe('DockStack', () => {
  it('stacks the slots top tray to alert around the pill', () => {
    const { container } = render(<DockStack blank={false} dockRef={null} {...slots()} />)
    const order = Array.from(container.firstElementChild?.children ?? []).map((el) =>
      el.getAttribute('data-testid'),
    )
    expect(order).toEqual(['top', 'pickers', 'notes', 'pill', 'bottom', 'footer', 'alert'])
    expect(container.firstElementChild?.className).toBe('dock')
  })

  it('frames the blank chat as the document and attaches the dock ref only to the dock', () => {
    const ref = { current: null as HTMLDivElement | null }
    const { container, rerender } = render(<DockStack blank dockRef={ref} pill={<i />} />)
    expect(container.firstElementChild?.className).toBe('docframe')
    expect(ref.current).toBeNull()
    rerender(<DockStack blank={false} dockRef={ref} pill={<i />} />)
    expect(ref.current).toBe(container.firstElementChild)
  })

  it('keeps the pill node when the first send moves the blank chat down', () => {
    const { rerender } = render(<DockStack blank dockRef={null} pill={<i data-testid="pill" />} />)
    const before = screen.getByTestId('pill')
    rerender(
      <DockStack blank={false} dockRef={null} topTray={<b />} pill={<i data-testid="pill" />} />,
    )
    expect(screen.getByTestId('pill')).toBe(before)
  })
})

describe('DockDissolve', () => {
  it('draws the seven blur layers hidden from assistive tech', () => {
    const { container } = render(<DockDissolve />)
    expect(container.querySelector('.dissolve')?.getAttribute('aria-hidden')).toBe('true')
    expect(container.querySelectorAll('.dissolve-layer')).toHaveLength(7)
  })
})

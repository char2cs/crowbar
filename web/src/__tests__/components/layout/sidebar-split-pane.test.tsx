import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SidebarSplitPane } from '@/components/layout/sidebar-split-pane'

const rect = (width: number) => ({
  x: 0,
  y: 0,
  left: 0,
  top: 0,
  width,
  height: 700,
  right: width,
  bottom: 700,
  toJSON: () => ({}),
})

function pointerEvent(type: string, clientX: number) {
  return new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    button: 0,
    clientX,
  })
}

function renderSplit(
  overrides: Partial<Parameters<typeof SidebarSplitPane>[0]> = {},
  containerWidth = 1000,
) {
  const onOpenChange = vi.fn()
  const onWidthCommit = vi.fn()
  const props = {
    side: 'left' as const,
    open: true,
    preferredWidth: 300,
    minWidth: 250,
    maxWidth: 640,
    sidebar: <div data-testid="sidebar-child" />,
    children: <div data-testid="content-child" />,
    onOpenChange,
    onWidthCommit,
    ...overrides,
  }
  const result = render(<SidebarSplitPane {...props} />)
  const root = result.container.querySelector('[data-sidebar-split-pane]') as HTMLDivElement
  const sidebar = result.container.querySelector(
    '[data-sidebar-split-panel="sidebar"]',
  ) as HTMLDivElement
  const handle = screen.getByRole('separator', { name: 'Resize sidebar' })
  const rootRect = vi.fn(() => rect(containerWidth))
  const sidebarRect = vi.fn(() => {
    const raw = root.style.getPropertyValue('--sidebar-track-width')
    return rect(parseFloat(raw) || 0)
  })
  root.getBoundingClientRect = rootRect
  sidebar.getBoundingClientRect = sidebarRect
  return {
    ...result,
    props,
    root,
    sidebar,
    handle,
    rootRect,
    sidebarRect,
    onOpenChange,
    onWidthCommit,
  }
}

afterEach(() => {
  cleanup()
  document.documentElement.removeAttribute('data-pane-resizing')
})

describe('SidebarSplitPane idle pointer contract', () => {
  it('does not register a pointermove listener while idle', () => {
    const addListener = vi.spyOn(window, 'addEventListener')
    renderSplit()

    expect(
      addListener.mock.calls.filter(([type]) => (type as string) === 'pointermove'),
    ).toHaveLength(0)
    addListener.mockRestore()
  })

  it('performs no geometry reads for pointer movement over sidebar content', () => {
    const { rootRect, sidebarRect } = renderSplit()

    screen.getByTestId('sidebar-child').dispatchEvent(pointerEvent('pointermove', 120))

    expect(rootRect).not.toHaveBeenCalled()
    expect(sidebarRect).not.toHaveBeenCalled()
  })
})

describe('SidebarSplitPane pointer resizing', () => {
  it('measures once at drag start and never again across pointer moves', () => {
    const { handle, root, rootRect, sidebarRect, onOpenChange, onWidthCommit } = renderSplit()
    const resizeEnd = vi.fn()
    window.addEventListener('pane-resize-end', resizeEnd)

    handle.dispatchEvent(pointerEvent('pointerdown', 300))
    for (let x = 301; x <= 350; x += 1) window.dispatchEvent(pointerEvent('pointermove', x))

    expect(document.documentElement).toHaveAttribute('data-pane-resizing', '1')
    expect(rootRect).toHaveBeenCalledTimes(1)
    expect(sidebarRect).toHaveBeenCalledTimes(1)

    window.dispatchEvent(pointerEvent('pointerup', 350))

    expect(root.style.getPropertyValue('--sidebar-track-width')).toBe('350px')
    expect(document.documentElement).not.toHaveAttribute('data-pane-resizing')
    expect(onOpenChange).toHaveBeenCalledWith(true)
    expect(onWidthCommit).toHaveBeenCalledWith(350)
    expect(resizeEnd).toHaveBeenCalledTimes(1)
    expect(rootRect).toHaveBeenCalledTimes(1)
    expect(sidebarRect).toHaveBeenCalledTimes(1)
    window.removeEventListener('pane-resize-end', resizeEnd)
  })

  it('interprets physical movement from the right edge in the opposite direction', () => {
    const { handle, root, onWidthCommit } = renderSplit({ side: 'right' })

    handle.dispatchEvent(pointerEvent('pointerdown', 700))
    window.dispatchEvent(pointerEvent('pointermove', 750))
    window.dispatchEvent(pointerEvent('pointerup', 750))

    expect(root.style.getPropertyValue('--sidebar-track-width')).toBe('250px')
    expect(onWidthCommit).toHaveBeenCalledWith(250)
  })

  it('preserves the content minimum and hard sidebar maximum', () => {
    const wide = renderSplit()
    wide.handle.dispatchEvent(pointerEvent('pointerdown', 300))
    window.dispatchEvent(pointerEvent('pointermove', 900))
    window.dispatchEvent(pointerEvent('pointerup', 900))
    expect(wide.onWidthCommit).toHaveBeenCalledWith(640)
    wide.unmount()

    const narrow = renderSplit({}, 700)
    narrow.handle.dispatchEvent(pointerEvent('pointerdown', 300))
    window.dispatchEvent(pointerEvent('pointermove', 900))
    window.dispatchEvent(pointerEvent('pointerup', 900))
    // 700px container minus the 1px track, with 20% left for content.
    expect(narrow.onWidthCommit).toHaveBeenCalledWith(559)
  })

  it('collapses past the threshold without overwriting the remembered width', () => {
    const { handle, root, onOpenChange, onWidthCommit } = renderSplit()

    handle.dispatchEvent(pointerEvent('pointerdown', 300))
    window.dispatchEvent(pointerEvent('pointermove', 100))
    window.dispatchEvent(pointerEvent('pointerup', 100))

    expect(root.style.getPropertyValue('--sidebar-track-width')).toBe('0px')
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(onWidthCommit).not.toHaveBeenCalled()
  })

  it('treats a bare separator click as a complete no-op', () => {
    const { handle, rootRect, sidebarRect, onOpenChange, onWidthCommit } = renderSplit()

    handle.dispatchEvent(pointerEvent('pointerdown', 300))
    window.dispatchEvent(pointerEvent('pointerup', 300))

    expect(rootRect).toHaveBeenCalledTimes(1)
    expect(sidebarRect).toHaveBeenCalledTimes(1)
    expect(document.documentElement).not.toHaveAttribute('data-pane-resizing')
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(onWidthCommit).not.toHaveBeenCalled()
  })

  it('restores the starting width and commits nothing when the drag is cancelled', () => {
    const { handle, root, onOpenChange, onWidthCommit } = renderSplit()

    handle.dispatchEvent(pointerEvent('pointerdown', 300))
    window.dispatchEvent(pointerEvent('pointermove', 380))
    window.dispatchEvent(pointerEvent('pointercancel', 380))

    expect(root.style.getPropertyValue('--sidebar-track-width')).toBe('300px')
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(onWidthCommit).not.toHaveBeenCalled()
    expect(document.documentElement).not.toHaveAttribute('data-pane-resizing')
  })

  it('removes drag listeners and resize state when unmounted mid-drag', () => {
    const removeListener = vi.spyOn(window, 'removeEventListener')
    const { handle, unmount } = renderSplit()

    handle.dispatchEvent(pointerEvent('pointerdown', 300))
    window.dispatchEvent(pointerEvent('pointermove', 350))
    unmount()

    expect(document.documentElement).not.toHaveAttribute('data-pane-resizing')
    const removed = removeListener.mock.calls.map(([type]) => type)
    expect(removed).toEqual(expect.arrayContaining(['pointermove', 'pointerup', 'pointercancel']))
    removeListener.mockRestore()
  })
})

describe('SidebarSplitPane retained layout and keyboard behavior', () => {
  it('moves regions through grid areas without replacing either subtree', () => {
    const { props, rerender, root } = renderSplit({ side: 'left' })
    const sidebarBefore = screen.getByTestId('sidebar-child')
    const contentBefore = screen.getByTestId('content-child')

    rerender(<SidebarSplitPane {...props} side="right" />)

    expect(root).toHaveAttribute('data-side', 'right')
    expect(root.style.gridTemplateAreas).toBe('"content handle sidebar"')
    expect(screen.getByTestId('sidebar-child')).toBe(sidebarBefore)
    expect(screen.getByTestId('content-child')).toBe(contentBefore)
  })

  it('collapses and restores the remembered width without replacing children', () => {
    const { props, rerender, root } = renderSplit()
    const sidebarBefore = screen.getByTestId('sidebar-child')

    rerender(<SidebarSplitPane {...props} open={false} />)
    expect(root.style.getPropertyValue('--sidebar-track-width')).toBe('0px')

    rerender(<SidebarSplitPane {...props} open preferredWidth={412} />)
    expect(root.style.getPropertyValue('--sidebar-track-width')).toBe('412px')
    expect(screen.getByTestId('sidebar-child')).toBe(sidebarBefore)
  })

  it('supports physical arrow keys on either side and Enter to collapse', () => {
    const left = renderSplit()
    fireEvent.keyDown(left.handle, { key: 'ArrowRight' })
    expect(left.onWidthCommit).toHaveBeenCalledWith(316)
    left.unmount()

    const right = renderSplit({ side: 'right' })
    fireEvent.keyDown(right.handle, { key: 'ArrowRight' })
    expect(right.onWidthCommit).toHaveBeenCalledWith(284)
    fireEvent.keyDown(right.handle, { key: 'Enter' })
    expect(right.onOpenChange).toHaveBeenCalledWith(false)
  })
})

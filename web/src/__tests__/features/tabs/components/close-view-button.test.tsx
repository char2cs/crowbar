import { fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import CloseViewButton from '@/features/tabs/components/close-view-button'
import TabAddButton from '@/features/tabs/components/tab-add-button'

const baseProps = {
  isBottomPane: false,
  disablePaneActions: false,
  canClose: true,
  onCloseView: vi.fn(),
}

describe('CloseViewButton', () => {
  it('renders when the view is closeable and pane actions are enabled', () => {
    render(<CloseViewButton {...baseProps} />)
    expect(screen.getByRole('button', { name: 'Close view' })).toBeDefined()
  })

  it('calls onCloseView on click', () => {
    const onCloseView = vi.fn()
    render(<CloseViewButton {...baseProps} onCloseView={onCloseView} />)

    fireEvent.click(screen.getByRole('button', { name: 'Close view' }))

    expect(onCloseView).toHaveBeenCalledTimes(1)
  })

  it('is absent in the bottom pane', () => {
    render(<CloseViewButton {...baseProps} isBottomPane={true} />)
    expect(screen.queryByRole('button', { name: 'Close view' })).toBeNull()
  })

  it('is absent when pane actions are disabled', () => {
    render(<CloseViewButton {...baseProps} disablePaneActions={true} />)
    expect(screen.queryByRole('button', { name: 'Close view' })).toBeNull()
  })

  // Regression: a solo (non-split) view used to show no close control at all,
  // because the old gate was "is there a second pane in this split" rather
  // than "is there something left to close" — closing one half of a split
  // then left the survivor with NO way to finish closing it from the pane
  // chrome (the user had to find Recents' × instead). A solo view renders the
  // SAME control a split one does; canClose is a WORKING gate now, not a
  // split-membership one.
  it('renders for a solo (non-split) view, not just a split one', () => {
    render(<CloseViewButton {...baseProps} />)
    expect(screen.getByRole('button', { name: 'Close view' })).toBeDefined()
  })

  // §5.4: every view has a close control except a working one — there is
  // nothing left to close, and that absence is the "still running" signal
  // (the same rule Recents' own × applies).
  it('is absent while the view is working', () => {
    render(<CloseViewButton {...baseProps} canClose={false} />)
    expect(screen.queryByRole('button', { name: 'Close view' })).toBeNull()
  })

  // Regression: the two affordances at either end of the SAME tab bar had
  // drifted apart — close was `icon-xs` + a Phosphor X, "+" was `icon-sm` + a
  // Lucide Plus — so they painted at different box sizes and stroke weights.
  it('wears the same chrome as the tab bar’s + button', () => {
    const { container: closeContainer } = render(<CloseViewButton {...baseProps} />)
    const { container: addContainer } = render(
      <TabAddButton isBottomPane={false} onNewTab={vi.fn()} />,
    )

    const closeButton = closeContainer.querySelector('button')
    const addButton = addContainer.querySelector('button')
    expect(closeButton?.className).toBe(addButton?.className)

    // Same icon family, so the strokes match; and neither carries an inline
    // size the button's own `[&_svg]:size-*` would silently override.
    const closeIcon = closeContainer.querySelector('svg')
    const addIcon = addContainer.querySelector('svg')
    expect(closeIcon?.classList.contains('lucide')).toBe(true)
    expect(addIcon?.classList.contains('lucide')).toBe(true)
    expect(closeIcon?.getAttribute('width')).toBe(addIcon?.getAttribute('width'))
    expect(closeIcon?.getAttribute('height')).toBe(addIcon?.getAttribute('height'))
  })
})

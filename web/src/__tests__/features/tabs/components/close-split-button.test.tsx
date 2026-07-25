import { fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import CloseSplitButton from '@/features/tabs/components/close-split-button'
import TabAddButton from '@/features/tabs/components/tab-add-button'

const baseProps = {
  isBottomPane: false,
  disablePaneActions: false,
  isInSplit: true,
  onClosePane: vi.fn(),
}

describe('CloseSplitButton', () => {
  it('renders when in a split and pane actions are enabled', () => {
    render(<CloseSplitButton {...baseProps} />)
    expect(screen.getByRole('button', { name: 'Close split pane' })).toBeDefined()
  })

  it('calls onClosePane on click', () => {
    const onClosePane = vi.fn()
    render(<CloseSplitButton {...baseProps} onClosePane={onClosePane} />)

    fireEvent.click(screen.getByRole('button', { name: 'Close split pane' }))

    expect(onClosePane).toHaveBeenCalledTimes(1)
  })

  it('is absent in the bottom pane', () => {
    render(<CloseSplitButton {...baseProps} isBottomPane={true} />)
    expect(screen.queryByRole('button', { name: 'Close split pane' })).toBeNull()
  })

  it('is absent when pane actions are disabled', () => {
    render(<CloseSplitButton {...baseProps} disablePaneActions={true} />)
    expect(screen.queryByRole('button', { name: 'Close split pane' })).toBeNull()
  })

  it('is absent when not in a split', () => {
    render(<CloseSplitButton {...baseProps} isInSplit={false} />)
    expect(screen.queryByRole('button', { name: 'Close split pane' })).toBeNull()
  })

  // Regression: the two affordances at either end of the SAME tab bar had
  // drifted apart — close was `icon-xs` + a Phosphor X, "+" was `icon-sm` + a
  // Lucide Plus — so they painted at different box sizes and stroke weights.
  it('wears the same chrome as the tab bar’s + button', () => {
    const { container: closeContainer } = render(<CloseSplitButton {...baseProps} />)
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

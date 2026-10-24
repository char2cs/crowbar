import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, describe, it, expect, vi } from 'vitest'
import TabAddButton from '@/features/tabs/components/tab-add-button'

/** The "+" of a top pane, with a no-op for every action the test does not watch. */
function renderAddButton(props: Partial<ComponentProps<typeof TabAddButton>> = {}) {
  render(
    <TabAddButton
      isBottomPane={false}
      onNewFile={vi.fn()}
      onNewTerminal={vi.fn()}
      onOpenFile={vi.fn()}
      {...props}
    />,
  )
}

describe('TabAddButton', () => {
  it('renders the + trigger button', () => {
    renderAddButton()
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })

  // More than one thing can land in a pane's editor view, so a single default
  // click can't pick one silently; the "+" opens a dropdown offering them.
  it('opens a dropdown offering New File and New Terminal', async () => {
    const user = userEvent.setup()
    renderAddButton()

    await user.click(screen.getByRole('button', { name: 'New tab' }))

    expect(await screen.findByRole('menuitem', { name: /New File/ })).toBeDefined()
    expect(screen.getByRole('menuitem', { name: /New Terminal/ })).toBeDefined()
  })

  it('calls onNewFile when "New File" is chosen', async () => {
    const user = userEvent.setup()
    const onNewFile = vi.fn()
    renderAddButton({ onNewFile })

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(await screen.findByRole('menuitem', { name: /New File/ }))

    expect(onNewFile).toHaveBeenCalledTimes(1)
  })

  it('calls onNewTerminal when "New Terminal" is chosen', async () => {
    const user = userEvent.setup()
    const onNewTerminal = vi.fn()
    renderAddButton({ onNewTerminal })

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(await screen.findByRole('menuitem', { name: /New Terminal/ }))

    expect(onNewTerminal).toHaveBeenCalledTimes(1)
  })

  it('is absent in the bottom pane', () => {
    renderAddButton({ isBottomPane: true })
    expect(screen.queryByRole('button', { name: 'New tab' })).toBeNull()
  })

  describe('Open File…', () => {
    type TauriWindow = Window & { __TAURI_INTERNALS__?: object }
    afterEach(() => {
      delete (window as TauriWindow).__TAURI_INTERNALS__
    })

    it('is offered on desktop and calls onOpenFile', async () => {
      ;(window as TauriWindow).__TAURI_INTERNALS__ = {}
      const user = userEvent.setup()
      const onOpenFile = vi.fn()
      renderAddButton({ onOpenFile })

      await user.click(screen.getByRole('button', { name: 'New tab' }))
      await user.click(await screen.findByRole('menuitem', { name: /Open File…/ }))

      expect(onOpenFile).toHaveBeenCalledTimes(1)
    })

    it('is not offered in the browser, which has no file dialog', async () => {
      const user = userEvent.setup()
      renderAddButton()

      await user.click(screen.getByRole('button', { name: 'New tab' }))

      expect(await screen.findByRole('menuitem', { name: /New File/ })).toBeDefined()
      expect(screen.queryByRole('menuitem', { name: /Open File/ })).toBeNull()
    })
  })
})

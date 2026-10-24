import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, it, expect, vi } from 'vitest'
import TabAddButton from '@/features/tabs/components/tab-add-button'

describe('TabAddButton', () => {
  it('renders the + trigger button', () => {
    render(
      <TabAddButton
        isBottomPane={false}
        onNewFile={vi.fn()}
        onNewTerminal={vi.fn()}
        onOpenFile={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })

  // Two things can land in a pane's editor view — a blank file, or a
  // terminal — so a single default click can't pick one silently; the "+"
  // opens a dropdown offering both.
  it('opens a dropdown offering New File and New Terminal', async () => {
    const user = userEvent.setup()
    render(
      <TabAddButton
        isBottomPane={false}
        onNewFile={vi.fn()}
        onNewTerminal={vi.fn()}
        onOpenFile={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'New tab' }))

    expect(await screen.findByRole('menuitem', { name: /New File/ })).toBeDefined()
    expect(screen.getByRole('menuitem', { name: /New Terminal/ })).toBeDefined()
  })

  it('calls onNewFile when "New File" is chosen', async () => {
    const user = userEvent.setup()
    const onNewFile = vi.fn()
    render(
      <TabAddButton
        isBottomPane={false}
        onNewFile={onNewFile}
        onNewTerminal={vi.fn()}
        onOpenFile={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(await screen.findByRole('menuitem', { name: /New File/ }))

    expect(onNewFile).toHaveBeenCalledTimes(1)
  })

  it('calls onNewTerminal when "New Terminal" is chosen', async () => {
    const user = userEvent.setup()
    const onNewTerminal = vi.fn()
    render(
      <TabAddButton
        isBottomPane={false}
        onNewFile={vi.fn()}
        onNewTerminal={onNewTerminal}
        onOpenFile={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(await screen.findByRole('menuitem', { name: /New Terminal/ }))

    expect(onNewTerminal).toHaveBeenCalledTimes(1)
  })

  it('is absent in the bottom pane', () => {
    render(
      <TabAddButton
        isBottomPane={true}
        onNewFile={vi.fn()}
        onNewTerminal={vi.fn()}
        onOpenFile={vi.fn()}
      />,
    )
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
      render(
        <TabAddButton
          isBottomPane={false}
          onNewFile={vi.fn()}
          onNewTerminal={vi.fn()}
          onOpenFile={onOpenFile}
        />,
      )

      await user.click(screen.getByRole('button', { name: 'New tab' }))
      await user.click(await screen.findByRole('menuitem', { name: /Open File…/ }))

      expect(onOpenFile).toHaveBeenCalledTimes(1)
    })

    it('is not offered in the browser, which has no file dialog', async () => {
      const user = userEvent.setup()
      render(
        <TabAddButton
          isBottomPane={false}
          onNewFile={vi.fn()}
          onNewTerminal={vi.fn()}
          onOpenFile={vi.fn()}
        />,
      )

      await user.click(screen.getByRole('button', { name: 'New tab' }))

      expect(await screen.findByRole('menuitem', { name: /New File/ })).toBeDefined()
      expect(screen.queryByRole('menuitem', { name: /Open File/ })).toBeNull()
    })
  })
})

import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'

afterEach(cleanup)

describe('ComposerPlusButton', () => {
  it('renders a trigger button labelled "Add to this message"', () => {
    render(<ComposerPlusButton onOpenExcalidraw={vi.fn()} onOpenAttachFile={vi.fn()} />)
    expect(screen.getByRole('button', { name: /add to this message/i })).toBeDefined()
  })

  it('opens a dropdown with exactly Excalidraw and Attach File, in that order', async () => {
    const user = userEvent.setup()
    render(<ComposerPlusButton onOpenExcalidraw={vi.fn()} onOpenAttachFile={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    const items = await screen.findAllByRole('menuitem')
    expect(items.map((el) => el.textContent)).toEqual(['Excalidraw', 'Attach File'])
  })

  it('calls onOpenExcalidraw for the Excalidraw entry, never onOpenAttachFile', async () => {
    const user = userEvent.setup()
    const onOpenExcalidraw = vi.fn()
    const onOpenAttachFile = vi.fn()
    render(
      <ComposerPlusButton
        onOpenExcalidraw={onOpenExcalidraw}
        onOpenAttachFile={onOpenAttachFile}
      />,
    )
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    expect(onOpenExcalidraw).toHaveBeenCalledTimes(1)
    expect(onOpenAttachFile).not.toHaveBeenCalled()
  })

  it('calls onOpenAttachFile for the Attach File entry, never onOpenExcalidraw', async () => {
    const user = userEvent.setup()
    const onOpenExcalidraw = vi.fn()
    const onOpenAttachFile = vi.fn()
    render(
      <ComposerPlusButton
        onOpenExcalidraw={onOpenExcalidraw}
        onOpenAttachFile={onOpenAttachFile}
      />,
    )
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /attach file/i }))
    expect(onOpenAttachFile).toHaveBeenCalledTimes(1)
    expect(onOpenExcalidraw).not.toHaveBeenCalled()
  })

  it('clears the trigger data-open flag after picking an entry', async () => {
    const user = userEvent.setup()
    render(<ComposerPlusButton onOpenExcalidraw={vi.fn()} onOpenAttachFile={vi.fn()} />)
    const trigger = screen.getByRole('button', { name: /add to this message/i })
    await user.click(trigger)
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    // The Base UI popup unmounts only after its CSS close animation ends — an
    // event jsdom never fires — so asserting on DOM presence of the menuitem
    // would depend on real animation timing. The controlled `open` state (and
    // the `data-open` attribute it drives) flips synchronously with the click,
    // so it is the deterministic signal that the menu was told to close.
    await vi.waitFor(() => expect(trigger).not.toHaveAttribute('data-open'))
  })

  it('marks the trigger data-open only while the dropdown is open', async () => {
    const user = userEvent.setup()
    render(<ComposerPlusButton onOpenExcalidraw={vi.fn()} onOpenAttachFile={vi.fn()} />)
    const trigger = screen.getByRole('button', { name: /add to this message/i })
    expect(trigger).not.toHaveAttribute('data-open')
    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: /excalidraw/i })).toBeDefined()
    expect(trigger).toHaveAttribute('data-open')
  })
})

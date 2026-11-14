import { useRef, useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { Dropdown } from '@/components/ui/dropdown'

function Harness() {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  return (
    <>
      <button ref={anchorRef} type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      <Dropdown isOpen={open} onClose={() => setOpen(false)} anchorRef={anchorRef} animated={false}>
        <button type="button">Inside</button>
      </Dropdown>
    </>
  )
}

describe('Dropdown', () => {
  // A keyboard user who backs out with Esc carries on from where they opened it.
  it('Esc closes it and hands focus back to its trigger', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('button', { name: 'Open' }))
    screen.getByRole('button', { name: 'Inside' }).focus()

    await user.keyboard('{Escape}')

    expect(screen.queryByRole('button', { name: 'Inside' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open' })).toHaveFocus()
  })
})

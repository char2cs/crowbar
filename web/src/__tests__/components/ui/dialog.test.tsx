import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'

function backdrops() {
  return document.querySelectorAll('[data-slot="dialog-backdrop"]')
}

describe('Dialog backdrop', () => {
  it('renders a backdrop for an open dialog', async () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Outer</DialogTitle>
        </DialogContent>
      </Dialog>,
    )

    await screen.findByRole('dialog', { name: 'Outer' })

    expect(backdrops()).toHaveLength(1)
  })

  // Settings opens "Add remote host" from inside its own dialog: the inner one must still dim.
  it('renders its own backdrop for a dialog opened inside another dialog', async () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Outer</DialogTitle>
          <Dialog open>
            <DialogContent>
              <DialogTitle>Inner</DialogTitle>
            </DialogContent>
          </Dialog>
        </DialogContent>
      </Dialog>,
    )

    await screen.findByRole('dialog', { name: 'Inner', hidden: true })

    expect(backdrops()).toHaveLength(2)
  })
})

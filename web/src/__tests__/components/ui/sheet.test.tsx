import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { Sheet, SheetPopup, SheetTitle } from '@/components/ui/sheet'

function backdrops() {
  return document.querySelectorAll('[data-slot$="backdrop"]')
}

describe('Sheet backdrop', () => {
  it('renders a backdrop for an open sheet', async () => {
    render(
      <Sheet open>
        <SheetPopup>
          <SheetTitle>Panel</SheetTitle>
        </SheetPopup>
      </Sheet>,
    )

    await screen.findByRole('dialog', { name: 'Panel' })

    expect(backdrops()).toHaveLength(1)
  })

  // A sheet raised from inside a dialog must still dim the dialog behind it.
  it('renders its own backdrop for a sheet opened inside a dialog', async () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Outer</DialogTitle>
          <Sheet open>
            <SheetPopup>
              <SheetTitle>Panel</SheetTitle>
            </SheetPopup>
          </Sheet>
        </DialogContent>
      </Dialog>,
    )

    await screen.findByRole('dialog', { name: 'Panel', hidden: true })

    expect(backdrops()).toHaveLength(2)
  })
})

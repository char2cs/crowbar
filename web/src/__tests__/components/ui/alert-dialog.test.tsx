import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AlertDialog, AlertDialogContent, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'

function backdrops() {
  return document.querySelectorAll('[data-slot$="backdrop"]')
}

describe('AlertDialog backdrop', () => {
  it('renders a backdrop for an open alert dialog', async () => {
    render(
      <AlertDialog open>
        <AlertDialogContent>
          <AlertDialogTitle>Confirm</AlertDialogTitle>
        </AlertDialogContent>
      </AlertDialog>,
    )

    await screen.findByRole('alertdialog', { name: 'Confirm' })

    expect(backdrops()).toHaveLength(1)
  })

  // A confirmation raised from inside a dialog must still dim the dialog behind it.
  it('renders its own backdrop for an alert dialog opened inside a dialog', async () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Outer</DialogTitle>
          <AlertDialog open>
            <AlertDialogContent>
              <AlertDialogTitle>Confirm</AlertDialogTitle>
            </AlertDialogContent>
          </AlertDialog>
        </DialogContent>
      </Dialog>,
    )

    await screen.findByRole('alertdialog', { name: 'Confirm', hidden: true })

    expect(backdrops()).toHaveLength(2)
  })
})

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Button } from '@/components/ui/button'
import {
  AppDialog,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

/**
 * Regression for the defect `native/oracle/blocked/
 * repo-import-dialog-duplicate-button-id.md` diagnoses: `button.tsx` sets
 * `data-oracle-id="button"` as the `Button` primitive's own default, and
 * both of `dialog.tsx`'s close buttons render as an unnamed `Button`
 * (`DialogPopup` line ~84, `AppDialog` line ~259). Any dialog whose body also
 * renders an unnamed `Button` therefore emitted two `button`-id anchors,
 * which the oracle differ refuses outright — it cannot say which of the two
 * it compared. The fix gives each close button its own
 * `data-oracle-id="dialog-close"`.
 *
 * Mutation actually run (both call sites, one at a time, then reverted —
 * `data-oracle-id="dialog-close"` deleted from the `render={<Button .../>}`
 * so the close button falls back to `button.tsx`'s own `"button"` default):
 *
 * ```
 * FAIL  src/__tests__/components/ui/dialog.test.tsx > the dialog close button
 * no longer collides with an unnamed body Button > DialogPopup: the close
 * button carries its own id, distinct from an unnamed body Button
 * Error: expect(element).toHaveAttribute("data-oracle-id", "dialog-close")
 * Expected the element to have attribute:
 *   data-oracle-id="dialog-close"
 * Received:
 *   data-oracle-id="button"
 * ```
 *
 * Identical failure shape for the `AppDialog` call site. Both reverted back
 * to the fix afterwards — `bun run test dialog.test.tsx` is green again.
 */
describe('the dialog close button no longer collides with an unnamed body Button', () => {
  it('DialogPopup: the close button carries its own id, distinct from an unnamed body Button', () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Title</DialogTitle>
            <DialogDescription>Description</DialogDescription>
          </DialogHeader>
          <Button>Body action</Button>
        </DialogContent>
      </Dialog>,
    )

    // The close button is reachable and still labelled "Close" for a11y.
    const close = screen.getByRole('button', { name: 'Close' })
    expect(close).toHaveAttribute('data-oracle-id', 'dialog-close')

    // Exactly one anchor per id: the unnamed body Button keeps the
    // primitive's own default, and the close button no longer shares it.
    expect(document.querySelectorAll('[data-oracle-id="button"]')).toHaveLength(1)
    expect(document.querySelectorAll('[data-oracle-id="dialog-close"]')).toHaveLength(1)
  })

  it('AppDialog: the close button carries its own id, distinct from an unnamed body Button', () => {
    render(
      <AppDialog title="Title">
        <Button>Body action</Button>
      </AppDialog>,
    )

    const close = screen.getByRole('button', { name: 'Close' })
    expect(close).toHaveAttribute('data-oracle-id', 'dialog-close')

    expect(document.querySelectorAll('[data-oracle-id="button"]')).toHaveLength(1)
    expect(document.querySelectorAll('[data-oracle-id="dialog-close"]')).toHaveLength(1)
  })
})

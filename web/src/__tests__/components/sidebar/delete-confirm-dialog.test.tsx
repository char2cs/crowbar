/**
 * Spec §9: "A working chat is not deletable. REFUSED, not confirmed." and
 * "An idle delete confirms, and the confirm names what goes." These pin both
 * halves of `DeleteConfirmDialog` — a working row never reaches the Dialog
 * primitive at all, an idle row's dialog fetches and shows the real counts —
 * through a small harness (`TrashHarness`) that mirrors exactly how
 * `sidebar-tree-surface.tsx` wires the real trash click: a click names the
 * pending row, and this component alone decides refuse-vs-confirm off
 * `working`.
 */
import { useState } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { DeleteConfirmDialog } from '@/components/sidebar/delete-confirm-dialog'

const { fetchDeletePreview } = vi.hoisted(() => ({
  fetchDeletePreview: vi.fn(),
}))
vi.mock('@/components/sidebar/lib/delete-preview-client', () => ({
  fetchDeletePreview,
}))

interface Row {
  id: string
  label: string
  working: boolean
}

const baseRow: Row = { id: 'ws-a', label: 'feature-x', working: false }

function TrashHarness({ row, onConfirm }: { row: Row; onConfirm: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        trash
      </button>
      <DeleteConfirmDialog
        open={open}
        label={row.label}
        working={row.working}
        projectId="p1"
        repoId="r1"
        chatId={row.id}
        onOpenChange={setOpen}
        onConfirm={onConfirm}
      />
    </>
  )
}

function renderTrashClick(row: Row, onConfirm: () => void = vi.fn()) {
  render(<TrashHarness row={row} onConfirm={onConfirm} />)
  return () => fireEvent.click(screen.getByRole('button', { name: 'trash' }))
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('DeleteConfirmDialog', () => {
  it('a working row is refused, not confirmed — no dialog opens', async () => {
    const onTrashClick = renderTrashClick({ ...baseRow, working: true })
    await onTrashClick()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByText(/refuses|working/i)).toBeInTheDocument()
  })

  it('does not fetch a preview for a working row', async () => {
    const onTrashClick = renderTrashClick({ ...baseRow, working: true })
    await onTrashClick()
    expect(fetchDeletePreview).not.toHaveBeenCalled()
  })

  it('an idle delete names what goes', async () => {
    fetchDeletePreview.mockResolvedValue({ chatCount: 3, fileCount: 6 })
    const onTrashClick = renderTrashClick({ ...baseRow, working: false })
    await onTrashClick()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText(/6 uncommitted files/)).toBeInTheDocument())
    expect(screen.getByText(/3 chats/)).toBeInTheDocument()
    expect(fetchDeletePreview).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'ws-a')
  })

  it('confirming calls onConfirm and closes the dialog', async () => {
    fetchDeletePreview.mockResolvedValue({ chatCount: 0, fileCount: 0 })
    const onConfirm = vi.fn()
    const onTrashClick = renderTrashClick(baseRow, onConfirm)
    await onTrashClick()
    await waitFor(() => expect(screen.getByText(/0 uncommitted files/)).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))
    expect(onConfirm).toHaveBeenCalledOnce()
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('cancelling does not confirm', async () => {
    fetchDeletePreview.mockResolvedValue({ chatCount: 0, fileCount: 0 })
    const onConfirm = vi.fn()
    const onTrashClick = renderTrashClick(baseRow, onConfirm)
    await onTrashClick()
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onConfirm).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('a preview fetch failure still lets the delete proceed', async () => {
    fetchDeletePreview.mockRejectedValue(new Error('daemon down'))
    const onConfirm = vi.fn()
    const onTrashClick = renderTrashClick(baseRow, onConfirm)
    await onTrashClick()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /delete/i })).toBeInTheDocument(),
    )

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))
    expect(onConfirm).toHaveBeenCalledOnce()
  })
})

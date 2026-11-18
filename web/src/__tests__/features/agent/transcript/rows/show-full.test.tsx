import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { showFull } from '@/features/agent/stores/chat-feed'
import { expectReported, watchToasts } from '@/__tests__/__fixtures__/reports'
import { ShowFull } from '@/features/agent/transcript/rows/show-full'

vi.mock('@/features/agent/stores/chat-feed', () => ({ showFull: vi.fn() }))

beforeEach(() => {
  vi.mocked(showFull).mockReset()
})

describe('ShowFull', () => {
  it('says nothing about an item that is whole', () => {
    const { container } = render(<ShowFull chatId="c1" id="t:x" fullSeq={undefined} what="diff" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('says the text is a preview and fetches the whole of it on request, once at a time', async () => {
    let done!: () => void
    vi.mocked(showFull).mockReturnValue(new Promise<void>((resolve) => (done = resolve)))
    render(<ShowFull chatId="c1" id="t:x" fullSeq={9} what="diff" />)
    expect(screen.getByText(/preview/i)).toBeInTheDocument()
    const button = screen.getByRole('button', { name: 'Show full diff' })
    fireEvent.click(button)
    fireEvent.click(button)
    expect(showFull).toHaveBeenCalledTimes(1)
    expect(showFull).toHaveBeenCalledWith('c1', 't:x')
    expect(button).toBeDisabled()
    done()
    await waitFor(() => expect(button).toBeEnabled())
  })

  it('stays ready to try again when the fetch fails', async () => {
    const toasts = watchToasts()
    vi.mocked(showFull).mockImplementation(() => Promise.reject(new ApiError('offline', 503)))
    render(<ShowFull chatId="c1" id="t:x" fullSeq={9} what="diff" />)
    fireEvent.click(screen.getByRole('button', { name: 'Show full diff' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Show full diff' })).toBeEnabled(),
    )
    expectReported(toasts, 'Load the full diff', 'offline')
  })
})

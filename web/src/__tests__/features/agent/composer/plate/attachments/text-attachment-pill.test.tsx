import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { TextAttachmentPill } from '@/features/agent/composer/plate/attachments/text-attachment-pill'

afterEach(() => {
  cleanup()
})

describe('TextAttachmentPill', () => {
  it('shows a line count and opens a modal with the raw text on click', () => {
    const text = 'line one\nline two\nline three'
    render(<TextAttachmentPill text={text} />)

    expect(screen.getByRole('button', { name: /pasted text/i })).toHaveTextContent('3 lines')
    expect(screen.queryByTestId('text-attachment-raw')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))

    expect(screen.getByTestId('text-attachment-raw').textContent).toBe(text)
  })

  it('renders the text with no markdown interpretation', () => {
    const text = '# Not a heading\n**not bold**'
    render(<TextAttachmentPill text={text} />)
    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    const raw = screen.getByTestId('text-attachment-raw')
    expect(raw.textContent).toBe(text)
    // The dialog chrome legitimately has its own <h2> title — what must be
    // absent is any heading/bold *produced from the pasted markdown-look-alike
    // content itself*, i.e. nothing but a plain text node inside the <pre>.
    expect(within(raw).queryByRole('heading')).toBeNull()
    expect(raw.children).toHaveLength(0)
    expect(raw.innerHTML).not.toContain('<strong>')
  })

  it('shows a singular "line" label for a single-line paste', () => {
    render(<TextAttachmentPill text="just one line" />)
    expect(screen.getByRole('button', { name: /pasted text/i })).toHaveTextContent('1 line')
    expect(screen.getByRole('button', { name: /pasted text/i })).not.toHaveTextContent('1 lines')
  })

  it('shows "0 lines" for empty text', () => {
    render(<TextAttachmentPill text="" />)
    expect(screen.getByRole('button', { name: /pasted text/i })).toHaveTextContent('0 lines')

    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    expect(screen.getByTestId('text-attachment-raw').textContent).toBe('')
  })

  it('counts many lines correctly', () => {
    const text = Array.from({ length: 50 }, (_, i) => `line ${i}`).join('\n')
    render(<TextAttachmentPill text={text} />)
    expect(screen.getByRole('button', { name: /pasted text/i })).toHaveTextContent('50 lines')
  })

  it('closes the modal and hides the raw text', () => {
    const text = 'line one\nline two'
    render(<TextAttachmentPill text={text} />)

    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    expect(screen.getByTestId('text-attachment-raw')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByTestId('text-attachment-raw')).toBeNull()
  })

  it('reopens the modal after being closed', () => {
    const text = 'a\nb'
    render(<TextAttachmentPill text={text} />)

    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByTestId('text-attachment-raw')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    expect(screen.getByTestId('text-attachment-raw').textContent).toBe(text)
  })

  it('shows the "Pasted text" dialog title', () => {
    render(<TextAttachmentPill text="hello" />)
    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    expect(screen.getByRole('heading', { name: 'Pasted text' })).toBeInTheDocument()
  })
})

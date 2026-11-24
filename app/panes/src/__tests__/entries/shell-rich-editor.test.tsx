import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { documentSession } from '@/entries/document-session'
import { ShellRichEditor } from '@/entries/shell-rich-editor'

afterEach(() => {
  cleanup()
  documentSession.setState({ text: null })
})

async function shown(text: string) {
  const client = { editDocument: vi.fn(), showSource: vi.fn(), saveDocument: vi.fn() }
  documentSession.setState({ text })
  render(
    <ShellRichEditor
      client={client}
      doc={{ authority: 'host', path: '/r/README.md' }}
      wsId="ws1"
    />,
  )
  await screen.findByText('Hello')
  return client
}

describe('ShellRichEditor', () => {
  it('takes text the shell sends later while it holds no edits', async () => {
    await shown('# Hello\n')
    act(() => documentSession.setState({ text: '# Again\n' }))
    expect(await screen.findByText('Again')).toBeTruthy()
  })

  it.each([
    [
      'the source view from its toggle',
      'showSource' as const,
      () => {
        fireEvent.click(screen.getByRole('button', { name: 'View markdown source' }))
      },
    ],
    [
      'a save on mod+s',
      'saveDocument' as const,
      () => {
        fireEvent.keyDown(window, { key: 's', ctrlKey: true })
      },
    ],
  ])('asks the shell for %s without writing an untouched document', async (_name, asked, act) => {
    const client = await shown('# Hello\n')
    act()
    expect(client[asked]).toHaveBeenCalledTimes(1)
    expect(client.editDocument).not.toHaveBeenCalled()
  })
})

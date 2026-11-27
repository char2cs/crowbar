import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { bufferSession } from '@/entries/buffer-session'
import { documentSession } from '@/entries/document-session'

vi.mock('@/features/agent/hooks/use-chat-entity', () => ({
  useChatField: (_chatId: string, pick: (chat: unknown) => unknown) => pick({ workspaceId: 'ws1' }),
}))

const { DocumentPaneHost } = await import('@/entries/document-pane-host')

afterEach(() => {
  cleanup()
  bufferSession.setState({ chatId: null, saved: null, active: false })
  documentSession.setState({ text: null })
})

const TOGGLE = { name: 'View markdown source' }

function shown(saved: object, text: string) {
  const client = { editDocument: vi.fn(), showSource: vi.fn(), saveDocument: vi.fn() }
  bufferSession.setState({ chatId: 'c1', saved, active: true })
  documentSession.setState({ text })
  render(<DocumentPaneHost client={client} />)
}

describe('DocumentPaneHost', () => {
  it.each([
    ['the rich view, with its source toggle', { path: 'README.md' }, true],
    ['a Markdown preview, read-only', { path: 'README.md', preview: 'markdown' }, false],
  ])('shows the text the shell owns in %s', async (_name, saved, toggles) => {
    shown(saved, '# Hello\n\nworld\n')
    expect(await screen.findByText('Hello')).toBeTruthy()
    expect(screen.queryByRole('button', TOGGLE) !== null).toBe(toggles)
  })

  it('renders an HTML preview in its sandboxed frame', () => {
    shown({ path: 'site/index.html', preview: 'html' }, '<h1>Hi</h1>')
    expect(screen.getByTitle('HTML Preview').getAttribute('sandbox')).toContain('allow-scripts')
  })
})

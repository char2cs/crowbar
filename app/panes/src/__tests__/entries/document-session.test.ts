import { afterEach, describe, expect, it, vi } from 'vitest'
import { documentSession, followDocumentText } from '@/entries/document-session'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'

afterEach(() => documentSession.setState({ text: null }))

function followed() {
  const onDocumentText = vi.fn((_listener: (text: string) => void) => () => {})
  const onLifecycle = vi.fn((_listener: (message: LifecycleMessage) => void) => () => {})
  followDocumentText({ onDocumentText, onLifecycle })
  return { text: onDocumentText.mock.calls[0]?.[0], life: onLifecycle.mock.calls[0]?.[0] }
}

describe('followDocumentText', () => {
  it.each([
    ['keeps it while the page is shown', 'activate', '# Title\n'],
    ['forgets it once the page is parked', 'park', null],
    ['forgets it once the page is destroyed', 'destroy', null],
  ])('holds the text the shell sends and %s', (_name, event, kept) => {
    const heard = followed()
    heard.text?.('# Title\n')
    heard.life?.({ event } as LifecycleMessage)
    expect(documentSession.getState().text).toBe(kept)
  })
})

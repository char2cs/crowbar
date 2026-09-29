import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  addPromptStash,
  createPromptStash,
  loadPromptStashes,
  matchesPromptStashShortcut,
  savePromptStashes,
} from '@/features/agent/composer/lib/prompt-stash-persistence'

const first = () =>
  createPromptStash('First draft', {
    id: 'first',
    now: new Date('2026-09-28T10:00:00.000Z'),
  })!

describe('prompt stash persistence', () => {
  beforeEach(() => localStorage.clear())

  it('round-trips rich markdown and settled attachment references in a scoped key', () => {
    const item = createPromptStash(
      'Review this:\n\n![screen](chats/c1/attachments/screen.png)\n\n```ts\nconst ok = true\n```',
      { id: 'rich', now: new Date('2026-09-28T11:00:00.000Z') },
    )!

    expect(savePromptStashes('workspace/one', 'chat:one', [item])).toBe(true)
    expect(localStorage.key(0)).toContain('workspace%2Fone:chat%3Aone')
    expect(loadPromptStashes('workspace/one', 'chat:one')).toEqual([item])
  })

  it('keeps workspace and chat stashes isolated', () => {
    savePromptStashes('w1', 'c1', [first()])

    expect(loadPromptStashes('w1', 'c1')).toHaveLength(1)
    expect(loadPromptStashes('w1', 'c2')).toEqual([])
    expect(loadPromptStashes('w2', 'c1')).toEqual([])
  })

  it('stores newest first and supports several stashes', () => {
    const older = first()
    const newer = createPromptStash('Second draft', {
      id: 'second',
      now: new Date('2026-09-28T11:00:00.000Z'),
    })!

    const items = addPromptStash(addPromptStash([], older), newer)
    expect(items.map((item) => item.id)).toEqual(['second', 'first'])
    expect(savePromptStashes('w1', 'c1', items)).toBe(true)
    expect(loadPromptStashes('w1', 'c1').map((item) => item.markdown)).toEqual([
      'Second draft',
      'First draft',
    ])
  })

  it('records pending and invalid attachments honestly instead of persisting fake completions', () => {
    const markdown = createPromptStash(
      'Before\n\n![upload.png](blob:https://crowbar.test/pending)\n\n[broken.txt](chats/c1/wrong/broken.txt)\n\n[done.txt](chats/c1/attachments/done.txt)',
    )!.markdown

    expect(markdown).not.toContain('blob:')
    expect(markdown).toContain('Image “upload.png” was still uploading')
    expect(markdown).toContain('Attachment “broken.txt” had an invalid Crowbar reference')
    expect(markdown).toContain('[done.txt](chats/c1/attachments/done.txt)')
  })

  it('removes corrupt persisted documents instead of partially reviving them', () => {
    expect(savePromptStashes('w1', 'c1', [first()])).toBe(true)
    const key = localStorage.key(0)!
    for (const bad of [
      '{broken',
      JSON.stringify({ version: 2, items: [first()] }),
      JSON.stringify({ version: 1, items: [{ ...first(), markdown: '' }] }),
    ]) {
      localStorage.setItem(key, bad)
      expect(loadPromptStashes('w1', 'c1')).toEqual([])
      expect(localStorage.getItem(key)).toBeNull()
    }
  })

  it('leaves the caller in control when persistence is unavailable', () => {
    const spy = vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })
    expect(savePromptStashes('w1', 'c1', [first()])).toBe(false)
    spy.mockRestore()
  })
})

describe('prompt stash keyboard shortcut', () => {
  const event = (overrides: Partial<KeyboardEvent> = {}) =>
    ({
      key: 's',
      metaKey: false,
      ctrlKey: false,
      shiftKey: false,
      altKey: false,
      repeat: false,
      ...overrides,
    }) as KeyboardEvent

  it('uses Cmd+S on macOS and Ctrl+S elsewhere', () => {
    expect(matchesPromptStashShortcut(event({ metaKey: true }), true)).toBe(true)
    expect(matchesPromptStashShortcut(event({ ctrlKey: true }), true)).toBe(false)
    expect(matchesPromptStashShortcut(event({ ctrlKey: true }), false)).toBe(true)
    expect(matchesPromptStashShortcut(event({ metaKey: true }), false)).toBe(false)
  })

  it('rejects shifted, alternate, repeated, and unrelated chords', () => {
    expect(matchesPromptStashShortcut(event({ ctrlKey: true, shiftKey: true }), false)).toBe(false)
    expect(matchesPromptStashShortcut(event({ ctrlKey: true, altKey: true }), false)).toBe(false)
    expect(matchesPromptStashShortcut(event({ ctrlKey: true, repeat: true }), false)).toBe(false)
    expect(matchesPromptStashShortcut(event({ ctrlKey: true, key: 'x' }), false)).toBe(false)
  })
})

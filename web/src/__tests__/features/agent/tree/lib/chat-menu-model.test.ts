/**
 * What the Chats tree's right-click menu offers.
 *
 * Pure, and tested pure: the labels are the whole user-facing contract of the
 * menu, and a count that says "2" over a gesture that moves 3 rows is the kind
 * of thing only a test of the model itself catches.
 */
import { describe, it, expect } from 'vitest'
import { chatMenuFor } from '@/features/agent/tree/lib/chat-menu-model'
import type { ChatDragSubject } from '@/features/agent/tree/lib/chat-drop'

const chat = (id: string, parentId = ''): ChatDragSubject => ({ kind: 'chat', id, parentId })
const folder = (id: string, parentId = ''): ChatDragSubject => ({
  kind: 'chatFolder',
  id,
  parentId,
})

const labels = (subjects: ChatDragSubject[]) => chatMenuFor(subjects).map((e) => e.label)
const ids = (subjects: ChatDragSubject[]) => chatMenuFor(subjects).map((e) => e.id)
/** Read by action rather than by index — the entry list is not a fixed length. */
const removeLabel = (subjects: ChatDragSubject[]) =>
  chatMenuFor(subjects).find((e) => e.id === 'remove')?.label

describe('chatMenuFor', () => {
  it('offers nothing for nothing', () => {
    // An empty list means no popup at all rather than a popup of disabled items.
    expect(chatMenuFor([])).toEqual([])
  })

  it('offers threading, grouping and deletion for one chat', () => {
    expect(labels([chat('c1')])).toEqual(['New thread', 'Group into a folder', 'Delete chat'])
    expect(ids([chat('c1')])).toEqual(['thread', 'group', 'remove'])
  })

  it('offers no thread over a FOLDER — a folder holds no turns to inherit', () => {
    expect(ids([folder('f1')])).toEqual(['group', 'remove'])
  })

  it('offers no thread over a multiselection — a thread is made from ONE parent', () => {
    // Absent rather than present and disabled: a greyed row invites the user to
    // work out what would enable it, and narrowing the selection is not a thing
    // this menu can say.
    expect(ids([chat('c1'), chat('c2')])).toEqual(['group', 'remove'])
    expect(ids([chat('c1'), folder('f1')])).toEqual(['group', 'remove'])
  })

  it('counts the rows a group would collect', () => {
    expect(labels([chat('c1'), chat('c2')])[0]).toBe('Group 2 into a folder')
    expect(labels([chat('c1'), chat('c2'), folder('f1')])[0]).toBe('Group 3 into a folder')
  })

  it('offers grouping for a folder too — everything holds everything here', () => {
    // Unlike the sidebar, where a repo is its own level with no folder to enter.
    expect(ids([folder('f1')])).toContain('group')
  })

  it('names a deletion for what it takes', () => {
    expect(removeLabel([chat('c1')])).toBe('Delete chat')
    expect(removeLabel([folder('f1')])).toBe('Delete folder')
    expect(removeLabel([chat('c1'), chat('c2')])).toBe('Delete 2 chats')
    expect(removeLabel([folder('f1'), folder('f2')])).toBe('Delete 2 folders')
  })

  it('falls back to "rows" for a mixed selection, which is neither', () => {
    expect(removeLabel([chat('c1'), folder('f1')])).toBe('Delete 2 rows')
    expect(removeLabel([folder('f1'), chat('c1'), chat('c2')])).toBe('Delete 3 rows')
  })

  it('offers no rename — both kinds already rename on double-click', () => {
    // The sidebar's menu has none either. A second path to the same editor is a
    // second thing to keep in step with the first.
    expect(ids([chat('c1')])).not.toContain('rename')
    expect(ids([folder('f1')])).not.toContain('rename')
  })
})

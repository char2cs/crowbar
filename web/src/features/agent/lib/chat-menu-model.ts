import type { ChatDragSubject } from './chat-drop'

/**
 * What the Chats tree's right-click menu offers, decided without rendering
 * anything.
 *
 * The same shape as the sidebar's `row-menu-model.ts`, and separate from it for
 * the same reason the drop policies are separate: the two trees agree completely
 * about the GESTURE — right-click a row, act on the selection it belongs to — and
 * not at all about what may be done to a row. There is no lock here, and there is
 * no kind that refuses to be filed.
 *
 * Deliberately NO rename entry. Both kinds of row already rename on double-click,
 * which is exactly what the sidebar does — its menu has no rename either — and a
 * second path to the same editor is a second thing to keep in step.
 */

export type ChatMenuAction = 'thread' | 'group' | 'remove'

export interface ChatMenuEntry {
  id: ChatMenuAction
  label: string
}

/**
 * The menu for `subjects` — the multiselection when the clicked row is part of
 * it, that row alone when it is not. Callers get that split from
 * `dragSubjectsFor()`, the same function the drag uses, so a right-click and a
 * drag can never disagree about what they are acting on.
 *
 * An empty list means no menu at all rather than a menu of disabled items:
 * opening an empty popup is a worse answer than the row simply not having one.
 */
export function chatMenuFor(subjects: readonly ChatDragSubject[]): ChatMenuEntry[] {
  if (subjects.length === 0) return []

  const n = subjects.length
  return [
    // A thread is made from ONE parent — it reads that chat's turns — so there is
    // no honest reading of "new thread" over a multiselection, and none over a
    // folder, which holds no turns for a thread to inherit. The entry is simply
    // absent in both cases rather than present and disabled: a greyed row invites
    // the user to work out what would enable it, and nothing here would.
    ...(n === 1 && subjects[0].kind === 'chat'
      ? [{ id: 'thread' as const, label: 'New thread' }]
      : []),
    // Everything holds everything here, so grouping is always on offer — a chat, a
    // folder, or any mix of them can go into one.
    { id: 'group', label: n > 1 ? `Group ${n} into a folder` : 'Group into a folder' },
    { id: 'remove', label: removeLabel(subjects) },
  ]
}

/**
 * What a delete is called.
 *
 * Named for what it TAKES, because the two kinds take different things: a chat
 * goes with every thread hanging off it, while a folder is only ever a way of
 * looking at chats and leaves them behind, promoted to where it sat.
 */
function removeLabel(subjects: readonly ChatDragSubject[]): string {
  const n = subjects.length
  if (n === 1) return subjects[0].kind === 'chatFolder' ? 'Delete folder' : 'Delete chat'
  if (subjects.every((s) => s.kind === 'chat')) return `Delete ${n} chats`
  if (subjects.every((s) => s.kind === 'chatFolder')) return `Delete ${n} folders`
  return `Delete ${n} rows`
}

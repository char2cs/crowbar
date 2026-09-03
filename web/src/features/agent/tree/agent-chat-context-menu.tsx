import { useEffect, useRef, type RefObject } from 'react'
import { ArrowElbowDownRight } from '@phosphor-icons/react'
import { FolderPlus, Trash2 } from 'lucide-react'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { dragSubjectsFor } from '@/components/tree-dnd/drop-core'
import { readChatRow, type ChatDragSubject } from '@/features/agent/tree/lib/chat-drop'
import { chatMenuFor, type ChatMenuAction } from '@/features/agent/tree/lib/chat-menu-model'

interface AgentChatContextMenuProps {
  /** The element the tree's rows are drawn inside — where the listener goes. */
  treeRef: RefObject<HTMLElement | null>
  /** The multiselection, in tree order. Read at click time, never at render time. */
  selectionSubjects: () => ChatDragSubject[]
  /** Start a chat UNDER `chatId` — a thread, which reads that chat's turns. */
  onNewThread: (chatId: string) => void
  onGroup: (subjects: readonly ChatDragSubject[]) => void
  onRemove: (subjects: readonly ChatDragSubject[]) => void
}

/**
 * The Chats tree's right-click menu.
 *
 * This is where a THREAD is made. "New thread" starts a chat under the one that
 * was right-clicked, which is the gesture that gives this panel its point — a
 * child chat reads its parent's turns — and until it existed the only way to
 * make one was to create a chat at the root and then drag it into another.
 *
 * A folder is made the way the sidebar makes one: AROUND a selection. Collect
 * rows, right-click, "Group into a folder", and the folder appears where they
 * were with them already inside it. There is no standing "New folder" affordance
 * — the panel had one and it was wrong, because the tree next to it has never
 * had one and the two panels must not invent separate gestures for the same
 * thing.
 *
 * What it acts on comes from `dragSubjectsFor()`, the same function the drag
 * uses: right-clicking a row inside the multiselection acts on the whole
 * selection, right-clicking one outside it acts on that row and leaves the
 * selection alone. Two implementations of that rule would eventually disagree,
 * and the drag's is the one that is already tested.
 *
 * A SIBLING of the tree rather than something inside it, and listening natively
 * rather than through an `onContextMenu` prop. Both for the same reason the
 * sidebar's menu records: with the open/closed state inside the tree, opening
 * this popup re-renders every row to draw a menu that is not part of the tree at
 * all. Out here it re-renders itself.
 */
export function AgentChatContextMenu({
  treeRef,
  selectionSubjects,
  onNewThread,
  onGroup,
  onRemove,
}: AgentChatContextMenuProps) {
  const menu = useContextMenu()
  const { openAt } = menu
  // The rows the open menu acts on.
  //
  // A ref rather than the hook's `data` slot: that slot is typed nullable, so
  // reading it back means a fallback for a case this component cannot produce —
  // it never opens the menu without subjects. Written before the state change
  // that draws the popup, so the render that follows always sees this one.
  const subjectsRef = useRef<readonly ChatDragSubject[]>([])

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const onContextMenu = (e: MouseEvent) => {
      // `Element`, not `HTMLElement`: a chat row leads with its provider's SVG
      // glyph, and an SVGElement is not an HTMLElement — testing for one meant
      // right-clicking the icon opened nothing at all.
      if (!(e.target instanceof Element)) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const row = el ? readChatRow(el) : null
      // Not one of this tree's rows — the sidebar next door publishes none of
      // this tree's attributes. Let the event through rather than opening a menu
      // about nothing.
      if (!row) return
      subjectsRef.current = dragSubjectsFor(
        { kind: row.kind, id: row.id, parentId: row.parentId },
        selectionSubjects(),
      )
      openAt({ x: e.clientX, y: e.clientY })
      e.preventDefault()
    }
    tree.addEventListener('contextmenu', onContextMenu)
    return () => tree.removeEventListener('contextmenu', onContextMenu)
  }, [treeRef, openAt, selectionSubjects])

  if (!menu.isOpen) return null

  const subjects = subjectsRef.current
  // One table, keyed by the model's action id, so adding an entry to the model
  // and forgetting to draw it is a type error rather than a menu that silently
  // renders it as a delete.
  const draw: Record<ChatMenuAction, () => Omit<ContextMenuItem, 'id' | 'label'>> = {
    thread: () => ({
      // The SAME mark the row's own control wears, Phosphor beside the menu's
      // Lucide neighbours: two paths to one action drawn with two different
      // glyphs read as two different actions, and that is the worse drift of the
      // two. (agent-chat-row.tsx records why the elbow and not a "+".)
      icon: <ArrowElbowDownRight data-thread-glyph="true" weight="bold" />,
      // `subjects` is length 1 and a chat wherever this entry exists — that is
      // exactly the condition chatMenuFor draws it under.
      onClick: () => onNewThread(subjects[0].id),
    }),
    group: () => ({ icon: <FolderPlus />, onClick: () => onGroup(subjects) }),
    remove: () => ({
      icon: <Trash2 />,
      className: 'text-destructive',
      onClick: () => onRemove(subjects),
    }),
  }
  const items: ContextMenuItem[] = chatMenuFor(subjects).map((entry) => ({
    id: entry.id,
    label: entry.label,
    ...draw[entry.id](),
  }))

  return <ContextMenu isOpen items={items} position={menu.position} onClose={menu.close} />
}

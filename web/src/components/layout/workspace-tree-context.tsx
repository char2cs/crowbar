import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  DragGhost,
  DragGhostRows,
  cloneGhostRows,
  ghostTransform,
  grabOffsetFrom,
  type GhostRows,
  type GrabOffset,
} from './drag-ghost'
import { capturePlacement, useSidebarStore, type SidebarPlacement } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { reparentWorkspace } from '@/lib/api/workspace'
import {
  createFolder,
  placeFolder,
  placeProject,
  placeRepo,
  placeWorkspace,
} from '@/lib/api/sidebar-placement'
import { postWorkspace, setWorkspaceLock, type WorkspacePlacement } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { projectIdForRepo, performRenameRow, performImportBranches } from './workspace-tree-actions'
import {
  dragSubjectsFor,
  findDrop,
  isRemovableByDrag,
  readSelectedSubjects,
  rowElementFor,
  sameDrop,
  type DropHit,
  type ResolvedDrop,
} from './drop-target-dom'
import { planDrop, type PlacementCall } from './drop-plan'
import { createEdgeScroller, findScrollParent, type EdgeScroller } from './edge-scroll'
import { DropIndicator, dropLineBox } from './drop-indicator'
import { paintRemovalOverlay } from './editor-removal-overlay'
import { describeRemoval, planRemoval } from './removal-plan'
import { watchReparent } from './reparent-settle'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import type { DragSubject } from './drop-rules'
import type { Project } from '@/lib/types'

interface CreatingState {
  repoId: string
  parentId: string
}

/** The rows one drag is carrying. Always at least one; more when the grabbed
 *  row was part of the selection. */
interface DraggingState {
  subjects: DragSubject[]
}

export interface PendingCreate {
  repoId: string
  parentId: string
  branch: string
  error?: string
}

/** Stable empties, so a render that is not dragging hands out one identity. */
const NO_IDS: ReadonlySet<string> = new Set<string>()

const DRAG_THRESHOLD_PX = 5

/**
 * How long the pointer must rest inside the editor pane before a drop there
 * means removal.
 *
 * Short, because the dwell is no longer the only thing standing between a
 * pointer and a deletion. It was 400ms when the zone was INVISIBLE until it
 * armed: the wait was the whole guard, and it had to be long enough to sit out
 * an accidental transit. Now the veil is up from the first frame of the drag,
 * naming what would go, so arriving over the pane confirms something the user
 * has already been told — and 400ms of nothing happening on arrival read as lag
 * rather than as caution.
 *
 * Not zero: a flick that clips the pane on its way somewhere else must still not
 * arm. 150ms is under the ~200ms most people register as a delay while still
 * being far longer than a pointer crossing an edge.
 */
export const PANE_ARM_MS = 150

/** What a folder is called until the user says otherwise. */
const NEW_FOLDER_NAME = 'New folder'

/** Stable identity, so add/removeEventListener pair up across a drag. */
const preventDefault = (e: Event) => e.preventDefault()

/**
 * The slow-changing slice: action callbacks (stable identities) plus the
 * create/rename UI state, which only flips on explicit user intent. Split out
 * from the drag slice so that a drag (which updates on every drop-boundary
 * crossing) does not recreate this value and re-render every row that only
 * needs the actions.
 */
interface WorkspaceTreeActionsContextValue {
  // Create
  creatingChildOf: CreatingState | null
  startCreating: (repoId: string, parentId: string) => void
  /** What was typed into the one create input: `name/` is a folder, else a branch. */
  confirmCreate: (value: string) => void
  cancelCreate: () => void
  // Pending creates (in-flight / errored)
  pendingCreates: Map<string, PendingCreate>
  clearPendingCreate: (tempId: string) => void
  // Import: optimistically add a spinner row per branch, then fire the batch import
  startImport: (repoId: string, branches: string[]) => void
  // Rename. One editor for the whole tree, so opening one row's closes any
  // other's, and one id space: `renamingId` names a workspace or a folder.
  renamingId: string | null
  startRenaming: (rowId: string) => void
  confirmRename: (name: string) => void
  cancelRename: () => void
  // Drag start (pointer-based) — stable callback, lives with the actions
  onPointerDownDrag: (subject: DragSubject, e: React.PointerEvent) => void
  /**
   * Send rows to the removal tray. Non-destructive: the rows are hidden and
   * held, and only the tray commits anything. The keyboard's Delete and the
   * context menu both come through here, so neither can take a shortcut past it.
   */
  removeRows: (subjects: readonly DragSubject[]) => void
  /** Put rows in a new folder, named by the user. */
  groupIntoFolder: (subjects: readonly DragSubject[]) => void
  /**
   * Lock or unlock rows — the user's own decision, which outranks the provider's
   * protected flag from here on and survives the next poll.
   */
  setRowsLocked: (subjects: readonly DragSubject[], locked: boolean) => void
}

/**
 * The fast-changing slice: the live drag state.
 *
 * Everything here changes only when a drag CROSSES A BOUNDARY — the ghost's
 * position never touches React at all. A row reading this re-renders when the
 * answer to "is the drop landing on me, and how" changes, and at no other time.
 */
interface WorkspaceTreeDragContextValue {
  /** Non-null while a drag is in flight. */
  draggingWs: DraggingState | null
  /** The dragged rows, for the "this row is in the air" treatment. */
  draggingIds: ReadonlySet<string>
  /** Where a drop would land right now, with before/after/into already decided. */
  dropTarget: ResolvedDrop | null
  /** Rows with a placement request in flight. */
  movingIds: ReadonlySet<string>
}

const WorkspaceTreeActionsContext = createContext<WorkspaceTreeActionsContextValue | null>(null)
const WorkspaceTreeDragContext = createContext<WorkspaceTreeDragContextValue | null>(null)

export function useWorkspaceTreeActions() {
  const ctx = useContext(WorkspaceTreeActionsContext)
  if (!ctx) throw new Error('useWorkspaceTreeActions must be used inside WorkspaceTreeProvider')
  return ctx
}

export function useWorkspaceTreeDrag() {
  const ctx = useContext(WorkspaceTreeDragContext)
  if (!ctx) throw new Error('useWorkspaceTreeDrag must be used inside WorkspaceTreeProvider')
  return ctx
}

// A completed drag still produces a click on the captured row (pointerdown +
// pointerup on the same element), which would select/navigate into the
// dragged workspace. Swallow that one click in the capture phase.
function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  // The click (if any) fires synchronously after pointerup; drop the trap
  // right after so a later real click is never swallowed.
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

/**
 * Move the hairline onto the slot `hit` names, or take it away.
 *
 * Positioned from the rect the hit test already measured — the line never
 * measures anything itself — and written only where a coordinate actually
 * differs, so holding the pointer still inside one band costs nothing.
 */
function paintDropLine(el: HTMLDivElement | null, hit: DropHit | null): void {
  if (!el) return
  if (hit?.kind !== 'row' || hit.row.mode === 'into') {
    el.style.display = 'none'
    return
  }
  const box = dropLineBox(hit.rect, hit.row.mode)
  el.style.display = 'block'
  if (el.dataset.dropIndicator !== hit.row.mode) el.dataset.dropIndicator = hit.row.mode
  const left = `${box.left}px`
  const top = `${box.top}px`
  const width = `${box.width}px`
  if (el.style.left !== left) el.style.left = left
  if (el.style.top !== top) el.style.top = top
  if (el.style.width !== width) el.style.width = width
}

/**
 * Whether the line is tracking a moving list rather than sliding between two
 * settled slots.
 *
 * An edge scroll invalidates the line's position every frame, and a 60ms ease
 * between those positions is 60ms of the line trailing the rows it is meant to
 * be marking. It has to follow the scroll exactly, so the transition comes off
 * for as long as one is running.
 */
function setDropLineTracking(el: HTMLDivElement | null, tracking: boolean): void {
  if (!el) return
  el.style.transition = tracking ? 'none' : ''
}

/**
 * Where a folder typed into `parentId`'s create input actually hangs.
 *
 * The repo header's "+" carries the repo-HOME workspace as its parent, because
 * that is what a new branch forks from — but repo home is the header row, not a
 * row in the tree, so a folder started there belongs at the repo root. Every
 * other parent is already a row, and the folder hangs off it directly.
 */
function folderParentFor(repoId: string, parentId: string): string {
  const repo = useSidebarStore.getState().repos.find((r) => r.id === repoId)
  return repo && parentId === repo.defaultWorkspaceId ? '' : parentId
}

/**
 * How a new workspace typed into `parentId`'s create input is placed.
 *
 * The row the "+" was pressed on is either a workspace or a folder, and the two
 * mean different things. A workspace is the FORK parent: the new branch is cut
 * from it. A folder has no branch to cut from, so it is placement only — the new
 * row is filed into it and forks from the repo's default branch, which is what
 * the daemon falls back to when no parent is named.
 */
function placementFor(repoId: string, parentId: string): WorkspacePlacement {
  const repo = useSidebarStore.getState().repos.find((r) => r.id === repoId)
  const isFolder = repo?.folders?.some((f) => f.id === parentId) ?? false
  return isFolder ? { folderId: parentId } : { parentId }
}

/** Fire one planned request. */
function sendPlacement(call: PlacementCall): Promise<void> {
  switch (call.kind) {
    case 'reparent': {
      // 202 Accepted means the daemon took the move, not that it made it: the
      // rebase runs behind the response. The next call in the chain orders the
      // row within its NEW level, so this promise only settles once the row is
      // actually in it. The watch is armed BEFORE the request, because a frame
      // can beat the 202 back (see reparent-settle.ts).
      const settled = watchReparent(call.id, call.parentId)
      return reparentWorkspace(call.projectId, call.repoId, call.id, call.parentId).then(settled)
    }
    case 'workspace':
      return placeWorkspace(call.projectId, call.repoId, call.id, {
        ...(call.folderId !== undefined && { folderId: call.folderId }),
        order: call.order,
      })
    case 'folder':
      return placeFolder(call.projectId, call.repoId, call.id, {
        parentId: call.parentId,
        order: call.order,
      })
    case 'repo':
      // Addressed under the project it is in TODAY; the body says where it goes.
      return placeRepo(call.fromProjectId, call.id, {
        projectId: call.projectId,
        order: call.order,
      })
    case 'project':
      return placeProject(call.id, call.order)
  }
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive context provider — create/rename/delete/drag flows share the same refs, drag ghost and sidebar-store coordination; the size is the provider's wiring, not independent sections.
export function WorkspaceTreeProvider({ children }: { children: ReactNode }) {
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingWs, setDraggingWs] = useState<DraggingState | null>(null)
  const [dropTarget, setDropTarget] = useState<ResolvedDrop | null>(null)
  const [pendingCreates, setPendingCreates] = useState<Map<string, PendingCreate>>(new Map())
  const [movingIds, setMovingIds] = useState<ReadonlySet<string>>(NO_IDS)
  // The cloned rows the ghost carries. State rather than a ref because it is
  // rendered: written once, at drag start, in the same pass as setDraggingWs.
  const [ghostRows, setGhostRows] = useState<GhostRows | null>(null)

  function addPendingCreate(tempId: string, entry: PendingCreate) {
    setPendingCreates((prev) => new Map(prev).set(tempId, entry))
  }
  function setPendingCreateError(tempId: string, error: string) {
    setPendingCreates((prev) => {
      const entry = prev.get(tempId)
      if (!entry) return prev
      return new Map(prev).set(tempId, { ...entry, error })
    })
  }
  function clearPendingCreate(tempId: string) {
    setPendingCreates((prev) => {
      const n = new Map(prev)
      n.delete(tempId)
      return n
    })
  }

  // Ghost div position is updated imperatively in pointermove — no React state,
  // no tree re-renders on every pixel of mouse movement.
  const ghostRef = useRef<HTMLDivElement | null>(null)
  // The reorder hairline. One element for the whole drag, MOVED between slots
  // rather than re-rendered into each target row — same reason as the ghost,
  // plus a mark that remounts per crossing can never animate between them.
  const dropLineRef = useRef<HTMLDivElement | null>(null)
  // The drop the line is currently drawn from, replayed when it mounts (which
  // is one render AFTER the drag begins).
  const lastHitRef = useRef<DropHit | null>(null)
  // Tracks the initial position for the ghost's first render (set just before
  // setDraggingWs so the ghost mounts at the correct location).
  const lastDragPosRef = useRef<{ x: number; y: number } | null>(null)
  // Mirror the rendered drag state for equality checks — this is what keeps a
  // pointermove that stays inside one row from writing React state at all.
  const dropTargetRef = useRef<ResolvedDrop | null>(null)
  // How far the pointer has got with the editor pane: 'off' when it is not over
  // it, 'waiting' while the arming dwell runs, 'armed' once a release would
  // actually remove something. Kept on a ref because arming is a pointer fact
  // and the veil it raises is painted through the DOM.
  const paneRef = useRef<{ state: 'off' | 'waiting' | 'armed'; timer: number | null }>({
    state: 'off',
    timer: null,
  })
  // The latest pointer position, for the edge scroller's re-aim after a frame
  // that moved the list under a held-still pointer.
  const pointerRef = useRef<{ x: number; y: number } | null>(null)
  const edgeScrollerRef = useRef<EdgeScroller | null>(null)
  // The scroller's horizontal span, measured once with its box at drag start.
  // Edge scroll is gated on it — see onPointerMove.
  const scrollerXRef = useRef<{ left: number; right: number } | null>(null)
  // Where the pointer took hold of the row, in that row's own box. Subtracted
  // from the pointer on every move so the ghost stays under the grab point.
  const grabRef = useRef<GrabOffset>({ dx: 0, dy: 0 })

  const pendingRef = useRef<{
    subject: DragSubject
    startX: number
    startY: number
    target: HTMLElement
    pointerId: number
  } | null>(null)
  // Mirrors draggingWs for use inside window event handlers without stale closures.
  // Set synchronously at the point of change, not via useEffect, to avoid one-render lag.
  const draggingRef = useRef<DraggingState | null>(null)

  // Refs so the pointer handlers, which subscribe once, never close over a
  // stale setter.
  const setMovingIdsRef = useRef(setMovingIds)
  setMovingIdsRef.current = setMovingIds

  const startCreating = useCallback((repoId: string, parentId: string) => {
    setCreatingChildOf({ repoId, parentId })
  }, [])

  const confirmCreate = useCallback(
    (value: string) => {
      if (!creatingChildOf) return
      const { repoId, parentId } = creatingChildOf

      // A trailing slash means folder — the same thing it means everywhere else
      // a path is typed. One input, no second button, no right-click-only path.
      if (value.endsWith('/')) {
        setCreatingChildOf(null)
        const name = value.slice(0, -1).trim()
        if (!name) return
        const projectId = projectIdForRepo(repoId)
        if (!projectId) return
        void createFolder(projectId, repoId, name, folderParentFor(repoId, parentId)).catch(
          (err) => {
            toast.error(err instanceof Error ? err.message : 'Failed to create folder')
          },
        )
        return
      }

      const branch = value
      const tempId = crypto.randomUUID()
      setCreatingChildOf(null) // hide input immediately

      // The pending row keeps the row the "+" was pressed on, whichever kind it
      // is: that is where the optimistic row is drawn. Where the create LANDS is
      // the placement below, which the two kinds answer differently.
      addPendingCreate(tempId, { repoId, parentId, branch })
      const placement = placementFor(repoId, parentId)

      // Subscribe to sidebar store: when the real workspace arrives, remove
      // pending. The row is matched on the field the create actually set — a
      // folder create leaves parentId empty, so matching on that alone would
      // never resolve and the optimistic row would sit there for good.
      const unsub = useSidebarStore.subscribe((state) => {
        const repo = state.repos.find((r) => r.id === repoId)
        if (!repo) return
        const found = repo.workspaces.find(
          (w) =>
            w.branch === branch &&
            (placement.folderId ? w.folderId === placement.folderId : w.parentId === parentId),
        )
        if (found) {
          clearPendingCreate(tempId)
          unsub()
        }
      })

      // Fire the API
      const projectId = projectIdForRepo(repoId)
      if (!projectId) {
        setPendingCreateError(tempId, 'Unknown project')
        unsub()
        return
      }
      postWorkspace(projectId, repoId, branch, placement).catch((err) => {
        unsub()
        setPendingCreateError(tempId, err instanceof Error ? err.message : 'Create failed')
      })
    },
    [creatingChildOf],
  )

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])

  // Batch import: optimistic spinner rows + the 202 import, orchestrated by the
  // testable helper. The row setters are stable functional-setState wrappers.
  const startImport = useCallback((repoId: string, branches: string[]) => {
    performImportBranches(repoId, branches, {
      addPending: addPendingCreate,
      setError: setPendingCreateError,
      clearPending: clearPendingCreate,
    })
  }, [])

  const startRenaming = useCallback((rowId: string) => setRenamingId(rowId), [])

  const confirmRename = useCallback(
    (name: string) => {
      if (renamingId && name.trim()) {
        // The renamed entity arrives on its own WS stream and updates the tree —
        // no optimistic write here, which is what made the rename appear to work
        // while the branch never actually changed.
        void performRenameRow(renamingId, name.trim())
      }
      setRenamingId(null)
    },
    [renamingId],
  )

  const cancelRename = useCallback(() => setRenamingId(null), [])

  // Hold rows in the tray. The plan is what decides which rows go and what goes
  // WITH them; nothing here calls a delete, and cancelling is a matter of
  // dropping the ids the tray is holding.
  const removeRows = useCallback((subjects: readonly DragSubject[]) => {
    const drafts = planRemoval(
      subjects,
      useSidebarStore.getState().repos,
      dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS,
    )
    if (drafts.length === 0) return
    useRemovalTrayStore.getState().hold(drafts)
  }, [])

  /**
   * Put rows in a new folder.
   *
   * Two steps, in order: the folder has to exist before anything can be filed
   * into it, and each `order` is an index into the folder as it stands once the
   * previous row has landed.
   *
   * The folder is named for you and the inline editor is opened on it, rather
   * than a modal asking for a name before anything happens. The gesture is
   * "these belong together" — the grouping is what the user asked for, and the
   * name is a detail they can type into the row they can now see, over the
   * default, with the rows already inside it.
   */
  /**
   * Lock or unlock rows.
   *
   * Fired sequentially rather than together for the same reason every other
   * multi-row write in this file is: each call is one aggregate write under
   * optimistic-concurrency control, and firing a selection at once is how two of
   * them collide and one silently loses. There is no optimistic write here — the
   * updated workspaces arrive on their repo's WS stream, which is also what
   * re-renders the rows' glyphs.
   */
  const setRowsLocked = useCallback((subjects: readonly DragSubject[], locked: boolean) => {
    const rows = subjects.filter((s) => s.kind === 'workspace' && s.repoId)
    if (rows.length === 0) return
    void (async () => {
      try {
        for (const row of rows) {
          const projectId = projectIdForRepo(row.repoId!)
          if (!projectId) continue
          // react-doctor-disable-next-line async-await-in-loop -- deliberate: one aggregate write each under OCC; firing a selection together is how two of them collide and one is silently dropped.
          await setWorkspaceLock(projectId, row.repoId!, row.id, locked)
        }
      } catch (err) {
        toast.error(
          err instanceof Error ? err.message : `Failed to ${locked ? 'lock' : 'unlock'} workspace`,
        )
      }
    })()
  }, [])

  const groupIntoFolder = useCallback((subjects: readonly DragSubject[]) => {
    const rows = subjects.filter((s) => s.kind === 'workspace' || s.kind === 'folder')
    const repoId = rows[0]?.repoId
    if (!repoId || rows.some((s) => s.repoId !== repoId)) return
    const projectId = projectIdForRepo(repoId)
    if (!projectId) return
    // The folder is a sibling of the rows it collects, so it lands where they
    // already live rather than at the root they would have to be dragged from.
    const parentId = rows[0].parentId ?? ''

    void (async () => {
      try {
        const { id } = await createFolder(projectId, repoId, NEW_FOLDER_NAME, parentId)
        if (!id) return
        // Armed before the row exists: the FolderDTO arrives on the stream a
        // beat later, and a row that mounts already in its editor puts the
        // caret on the default name instead of making the user go and find it.
        setRenamingId(id)
        // The selection has been spent. Leaving it lit would put the collected
        // rows in the same treatment as the open workspace at the moment they
        // move under a folder that is itself asking to be named — and the next
        // plain click, which the user is about to make in the editor, would
        // clear it anyway.
        useSidebarSelectionStore.getState().clearSelected()
        const file = (row: DragSubject, order: number) =>
          row.kind === 'folder'
            ? placeFolder(projectId, repoId, row.id, { parentId: id, order })
            : placeWorkspace(projectId, repoId, row.id, { folderId: id, order })
        for (const [index, row] of rows.entries()) {
          // react-doctor-disable-next-line async-await-in-loop -- deliberate: each `order` is an index into the folder as it stands once the previous row has landed, so firing these together would file the rows in an arbitrary arrangement. Same rule as sendInOrder below.
          await file(row, index)
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to group into a folder')
      }
    })()
  }, [])

  // A callback ref, not an effect: the line mounts one render AFTER the drag
  // began, so the drop resolved at drag start is replayed onto it here. As an
  // effect the first slot a drag lands on would go unmarked until the pointer
  // moved again — and an effect running mid-drag is exactly what this file is
  // built to avoid.
  const attachDropLine = useCallback((el: HTMLDivElement | null) => {
    dropLineRef.current = el
    paintDropLine(el, lastHitRef.current)
  }, [])

  const onPointerDownDrag = useCallback((subject: DragSubject, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return // ignore second pointer mid-drag
    // Block text selection from the PRESS, not from the drag.
    //
    // `selectstart` fires once, as the selection begins — which is on this
    // pointerdown and the first move after it, both BEFORE the 5px threshold
    // promotes the press into a drag. Arming this in beginDrag (where it was)
    // was arming it after the only event it can cancel had already fired, so a
    // drag across the editor went on painting a selection under the shield. The
    // shield cannot help either: it mounts a render later still.
    document.addEventListener('selectstart', preventDefault)
    // Don't capture here — deferring setPointerCapture to the pointermove threshold
    // prevents it from swallowing the dblclick event used for rename.
    pendingRef.current = {
      subject,
      startX: e.clientX,
      startY: e.clientY,
      target: e.currentTarget as HTMLElement,
      pointerId: e.pointerId,
    }
  }, [])

  useEffect(() => {
    // Publish a resolved drop, but only when it would draw or commit something
    // different. This is the whole re-render budget of a drag: a pointermove
    // that stays inside one row's band writes no React state at all.
    function publish(hit: DropHit | null) {
      trackPane(hit?.kind === 'pane')
      const row = hit?.kind === 'row' ? hit.row : null
      if (!sameDrop(row, dropTargetRef.current)) {
        dropTargetRef.current = row
        setDropTarget(row)
      }
      // The line is DOM, not state: it moves on every publish, including the
      // ones that changed nothing for React (an edge scroll re-running the hit
      // test under a held-still pointer resolves the same drop at a new rect).
      lastHitRef.current = hit
      paintDropLine(dropLineRef.current, hit)
    }

    /**
     * Follow the pointer in and out of the editor pane, arming the removal once
     * it has stayed a beat.
     *
     * The dwell is the whole point: a long reorder crosses the pane on its way
     * between two ends of the sidebar, and a pane that removed on release would
     * make that transit a loaded gun. Nothing can be DROPPED until the dwell
     * elapses.
     *
     * What the dwell no longer gates is whether the zone is drawn at all. The
     * veil goes up when the drag starts (see beginDrag) and stays up for its
     * whole length; this only steps it between `available` and `armed`. Hiding
     * it until the pointer had already found the pane and waited made the one
     * gesture that deletes anything discoverable only to someone who already
     * knew it was there.
     */
    function trackPane(over: boolean): void {
      const pane = paneRef.current
      if (!over) {
        if (pane.state === 'off') return
        if (pane.timer !== null) clearTimeout(pane.timer)
        paneRef.current = { state: 'off', timer: null }
        // Back to available, NOT away: the row is still in the air and the pane
        // is still where it would go.
        paintRemovalTarget(false)
        return
      }
      if (pane.state !== 'off') return
      const timer = window.setTimeout(() => {
        if (!draggingRef.current) return
        if (!paintRemovalTarget(true)) {
          paneRef.current = { state: 'off', timer: null }
          return
        }
        paneRef.current = { state: 'armed', timer: null }
      }, PANE_ARM_MS)
      paneRef.current = { state: 'waiting', timer }
    }

    /**
     * Draw the pane's removal zone in one of its two states, and say whether
     * there was anything to draw.
     *
     * Returns false when this drag cannot remove anything — a locked branch, a
     * row whose repo has gone — in which case the veil stays down and the pane
     * never arms. Better to offer no target than one that refuses on release.
     */
    function paintRemovalTarget(armed: boolean): boolean {
      const drag = draggingRef.current
      if (!drag) return false
      // The SAME gate the drop itself uses, asked first. The veil is a promise
      // about what a release will do, so deriving it from a second rule is how
      // it ends up offering a removal the drop then refuses.
      if (!isRemovableByDrag(drag.subjects)) {
        paintRemovalOverlay(null)
        return false
      }
      const drafts = planRemoval(
        drag.subjects,
        useSidebarStore.getState().repos,
        dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS,
      )
      if (drafts.length === 0) {
        paintRemovalOverlay(null)
        return false
      }
      paintRemovalOverlay(describeRemoval(drafts, armed))
      return true
    }

    function beginDrag(e: PointerEvent) {
      const pending = pendingRef.current!
      pendingRef.current = null
      // Capture only now that drag is confirmed — keeps dblclick working.
      if (pending.target.isConnected) pending.target.setPointerCapture(pending.pointerId)

      const subjects = dragSubjectsFor(pending.subject, readSelectedSubjects())
      // Every measurement this drag needs, taken together and before the first
      // style write: the rows to clone, and the scroller's box (which cannot
      // change while a drag is in flight, so it is never measured again).
      const elements = subjects
        .map((s) => rowElementFor(s))
        .filter((el): el is HTMLElement => el !== null)
      const scroller = findScrollParent(pending.target)
      const scrollerBox = scroller?.getBoundingClientRect()
      // The row the hand is actually on — not elements[0], which is whichever
      // row of a multi-row selection happens to be drawn at the front of the
      // stack. Measured here, in the same batch as every other read this drag
      // takes, and never again.
      const grabbed = rowElementFor(pending.subject) ?? pending.target
      grabRef.current = grabOffsetFrom(
        grabbed.getBoundingClientRect(),
        pending.startX,
        pending.startY,
      )

      const rows = cloneGhostRows(
        elements.length > 0 ? elements : [pending.target],
        subjects.length,
      )
      if (scroller && scrollerBox) {
        scrollerXRef.current = { left: scrollerBox.left, right: scrollerBox.right }
        edgeScrollerRef.current = createEdgeScroller(
          scroller,
          { top: scrollerBox.top, height: scrollerBox.height },
          {
            onScrolled: () => {
              // The list moved under the pointer, so the row beneath it did
              // too — and so did the slot the line is marking. Re-resolving
              // here, inside the scroller's own frame and after its write, is
              // what keeps a viewport-positioned line honest without a scroll
              // listener firing through every ordinary drag.
              const at = pointerRef.current
              const drag = draggingRef.current
              if (at && drag) publish(findDrop(at.x, at.y, drag.subjects))
            },
            onRunningChange: (running) => setDropLineTracking(dropLineRef.current, running),
          },
        )
      }

      draggingRef.current = { subjects }
      pointerRef.current = { x: e.clientX, y: e.clientY }
      // Store position before triggering the React re-render so the ghost
      // div mounts at the correct location on its first render.
      lastDragPosRef.current = {
        x: e.clientX - grabRef.current.dx,
        y: e.clientY - grabRef.current.dy,
      }
      // Whatever the pointer is over for the rest of this drag, it is holding a
      // row. The attribute drives the carousel lock in index.css; the cursor and
      // the selection block are the shield's job (drag-ghost.tsx).
      document.documentElement.setAttribute('data-row-dragging', '')
      // Anything the press managed to select before the threshold goes now: the
      // listener above stops a selection STARTING, this drops one that already
      // did (a press that lands on a text node can carry a caret with it).
      window.getSelection()?.removeAllRanges()
      setGhostRows(rows)
      setDraggingWs({ subjects })
      // The removal zone, up front. `publish` below may immediately arm it if
      // the drag started with the pointer already over the pane.
      paintRemovalTarget(false)
      publish(findDrop(e.clientX, e.clientY, subjects))
    }

    function onPointerMove(e: PointerEvent) {
      if (pendingRef.current) {
        const { startX, startY } = pendingRef.current
        if (Math.hypot(e.clientX - startX, e.clientY - startY) > DRAG_THRESHOLD_PX) beginDrag(e)
        return
      }
      const drag = draggingRef.current
      if (!drag) return
      pointerRef.current = { x: e.clientX, y: e.clientY }

      // READS before WRITES. The hit test forces layout; writing the ghost's
      // position first would make every move a read-after-write reflow.
      const hit = findDrop(e.clientX, e.clientY, drag.subjects)

      // Edge scroll only while the pointer is still OVER the sidebar.
      //
      // The band is a function of Y alone, so carrying a row sideways into the
      // editor pane — which is how a row is removed, and which asks the hand to
      // hold still for the arming dwell — kept driving the scroller from a
      // pointer that had left the sidebar entirely. The list ran out from under
      // the drag at 14px a frame while the user was trying to hold still over
      // something else, and the rows they were aiming at were gone by the time
      // it armed.
      const span = scrollerXRef.current
      const overScroller = span !== null && e.clientX >= span.left && e.clientX <= span.right
      if (overScroller) edgeScrollerRef.current?.update(e.clientY)
      else edgeScrollerRef.current?.stop()

      if (ghostRef.current) {
        // One composited transform, not two layout-triggering offsets. See
        // ghostTransform in drag-ghost.tsx for the measurement.
        ghostRef.current.style.transform = ghostTransform(
          e.clientX - grabRef.current.dx,
          e.clientY - grabRef.current.dy,
        )
      }

      publish(hit)
    }

    function endDrag() {
      edgeScrollerRef.current?.stop()
      edgeScrollerRef.current = null
      scrollerXRef.current = null
      grabRef.current = { dx: 0, dy: 0 }
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      trackPane(false)
      // Explicitly, not as a side effect of trackPane: leaving the pane now
      // drops the veil back to `available` rather than taking it away, and
      // trackPane returns early when the pointer had already left. Relying on it
      // left the zone drawn over the editor after every drag that ended
      // anywhere but the pane.
      paintRemovalOverlay(null)
      draggingRef.current = null
      dropTargetRef.current = null
      lastDragPosRef.current = null
      lastHitRef.current = null
      pointerRef.current = null
      setGhostRows(null)
      setDraggingWs(null)
      setDropTarget(null)
    }

    function onPointerUp(e: PointerEvent) {
      pendingRef.current = null
      // Unconditionally: the listener is armed on every press, including the
      // ones that turn out to be plain clicks and never reach endDrag.
      document.removeEventListener('selectstart', preventDefault)
      const drag = draggingRef.current
      if (!drag) return

      // The browser fires a click for this pointerup on the captured row;
      // a drop must never double as a row click (selection/navigation).
      suppressNextClick()

      const hit = findDrop(e.clientX, e.clientY, drag.subjects)
      // Only an ARMED pane removes. A release over one that is still counting
      // out its dwell is a drag that ended in the wrong place, not a delete.
      if (hit?.kind === 'pane') {
        if (paneRef.current.state === 'armed') removeRows(drag.subjects)
      } else if (hit?.kind === 'row') commitDrop(drag.subjects, hit.row)

      endDrag()
    }

    // One request after the other: every `order` is an index into the list as
    // it stands once the previous call has landed, so firing them together
    // would leave a multi-row move in an arbitrary arrangement. The FIRST goes
    // out synchronously — a `Promise.resolve()` seed would hold the common
    // single-row drop back a microtask for nothing.
    function sendInOrder(calls: PlacementCall[]): Promise<void> {
      let chain: Promise<void> | null = null
      for (const call of calls) {
        chain = chain === null ? sendPlacement(call) : chain.then(() => sendPlacement(call))
      }
      return chain ?? Promise.resolve()
    }

    /**
     * Write the move, fire it, and put it back if it is refused.
     *
     * Optimistic as a UNIT: the placement covers every subject AND every
     * sibling the move displaced, so a refusal restores the level exactly as it
     * was rather than un-picking one row at a time.
     */
    function commitDrop(subjects: DragSubject[], target: ResolvedDrop) {
      const repos = useSidebarStore.getState().repos
      const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
      const plan = planDrop(subjects, target, {
        repos,
        projectIds: projects.map((p) => p.id),
      })
      if (!plan || plan.calls.length === 0) return

      setMovingIdsRef.current(new Set(subjects.map((s) => s.id)))
      const done = () => setMovingIdsRef.current(NO_IDS)

      if (plan.projectOrder) {
        void commitProjectOrder(plan.projectOrder, projects, plan.calls).finally(done)
        return
      }

      const undo: SidebarPlacement = capturePlacement(repos, plan.writes)
      useSidebarStore.getState().applyPlacement(plan.writes)
      void sendInOrder(plan.calls)
        .catch((err) => {
          useSidebarStore.getState().applyPlacement(undo)
          // A refusal has to SAY something. The optimistic write means the row
          // visibly moves and then returns to where it started, which on its own
          // reads as the drag having missed — the user retries the same gesture
          // and gets the same nothing. The daemon's reason is the useful part
          // ("a workspace cannot be filed away from its fork parent"), so pass it
          // through rather than inventing a generic failure.
          toast.error(err instanceof Error ? err.message : 'That move was refused')
        })
        .finally(done)
    }

    function commitProjectOrder(
      order: string[],
      projects: readonly Project[],
      calls: PlacementCall[],
    ): Promise<void> {
      const by = new Map(projects.map((p) => [p.id, p]))
      // `order` alongside the new array, not implied by it: the field is what
      // the daemon returns the list sorted by, so a paint that only moved the
      // rows would disagree with itself until the next refetch.
      const next = order
        .map((id) => by.get(id))
        .filter((p): p is Project => p !== undefined)
        .map((p, index) => (p.order === index ? p : { ...p, order: index }))
      // The project list owns its own optimistic write, snap-back included.
      return useProjectDataStore
        .getState()
        .optimisticWrite(next, () => sendInOrder(calls))
        .catch(() => {})
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', endDrag)
    return () => {
      edgeScrollerRef.current?.stop()
      trackPane(false)
      paintRemovalOverlay(null)
      // A teardown mid-drag would otherwise leave the carousel pinned and the
      // selection blocked, with no drag left to end and clear them.
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', endDrag)
    }
  }, [removeRows])

  // Memoize each slice so the Provider hands out stable values: the actions
  // value only changes when the create/rename state does (its callbacks are
  // stable), and the drag value only changes on a real drag update.
  const actionsValue = useMemo<WorkspaceTreeActionsContextValue>(
    () => ({
      creatingChildOf,
      startCreating,
      confirmCreate,
      cancelCreate,
      pendingCreates,
      clearPendingCreate,
      startImport,
      renamingId,
      startRenaming,
      confirmRename,
      cancelRename,
      onPointerDownDrag,
      removeRows,
      groupIntoFolder,
      setRowsLocked,
    }),
    [
      creatingChildOf,
      startCreating,
      confirmCreate,
      cancelCreate,
      pendingCreates,
      startImport,
      renamingId,
      startRenaming,
      confirmRename,
      cancelRename,
      onPointerDownDrag,
      removeRows,
      groupIntoFolder,
      setRowsLocked,
    ],
  )

  const dragValue = useMemo<WorkspaceTreeDragContextValue>(
    () => ({
      draggingWs,
      draggingIds: draggingWs ? new Set(draggingWs.subjects.map((s) => s.id)) : NO_IDS,
      dropTarget,
      movingIds,
    }),
    [draggingWs, dropTarget, movingIds],
  )

  return (
    <>
      <WorkspaceTreeActionsContext.Provider value={actionsValue}>
        <WorkspaceTreeDragContext.Provider value={dragValue}>
          {children}
        </WorkspaceTreeDragContext.Provider>
      </WorkspaceTreeActionsContext.Provider>
      {draggingWs && <DropIndicator ref={attachDropLine} />}
      {draggingWs && ghostRows && (
        <DragGhost ref={ghostRef} origin={lastDragPosRef.current}>
          <DragGhostRows rows={ghostRows} />
        </DragGhost>
      )}
    </>
  )
}

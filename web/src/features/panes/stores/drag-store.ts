import { create } from 'zustand'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { getPaneDropZoneFromRect, type PaneDropZone } from '@/features/panes/utils/pane-drop-zones'

/**
 * The one drag-state owner for pointer-driven drags that cross surfaces (a
 * file dragged out of the explorer, a sidebar row dragged onto a pane). It
 * replaces the old window-global bus (`window.__fileDragData`, the
 * `__crowbarInternalTabDrag*` globals and the `file-tree-drop-on-pane` /
 * `crowbar-internal-tab-drag-hover` window events): a pane subscribes to
 * whether IT is the hovered target with a narrow selector, and a drop is a
 * direct call naming its pane.
 */

export interface DropTarget {
  paneId: string | null
  zone: PaneDropZone
}

export interface FileDrag {
  path: string
  name: string
  isDir: boolean
}

interface DragState {
  /** The explorer file being dragged, while a drag is in flight. */
  file: FileDrag | null
  /** The pane (and zone of it) the pointer is over, while a drag is in flight. */
  hover: DropTarget
}

const NO_TARGET: DropTarget = { paneId: null, zone: null }

export const useDragStore = create<DragState>()(() => ({ file: null, hover: NO_TARGET }))

export function startFileDrag(file: FileDrag): void {
  useDragStore.setState({ file })
}

export function endFileDrag(): void {
  if (useDragStore.getState().file !== null) useDragStore.setState({ file: null })
}

/** A drag ended (dropped or abandoned): no pane is hovered any more. */
export function clearDropHover(): void {
  setDropHover(NO_TARGET)
}

export function setDropHover(next: DropTarget): void {
  const prev = useDragStore.getState().hover
  if (prev.paneId === next.paneId && prev.zone === next.zone) return
  useDragStore.setState({ hover: next.paneId === null ? NO_TARGET : next })
}

export function setDropHoverAt(point: { x: number; y: number }): void {
  setDropHover(resolveDropTarget(point))
}

/** The pane (and zone) under a viewport point, read off the DOM's pane markers. */
export function resolveDropTarget(point: { x: number; y: number }): DropTarget {
  const elements = document.elementsFromPoint(point.x, point.y)
  if (elements.length === 0) return NO_TARGET

  const tabBar = elements
    .map((element) => element.closest<HTMLElement>('[data-tab-bar-pane-id]'))
    .find((element) => Boolean(element?.dataset.tabBarPaneId))
  if (tabBar?.dataset.tabBarPaneId) {
    return { paneId: tabBar.dataset.tabBarPaneId, zone: 'center' }
  }

  const paneContainer = elements
    .map((element) => element.closest<HTMLElement>('[data-pane-id]'))
    .find((element) => Boolean(element?.dataset.paneId))
  if (paneContainer?.dataset.paneId) {
    return {
      paneId: paneContainer.dataset.paneId,
      zone: getPaneDropZoneFromRect(point, paneContainer.getBoundingClientRect()),
    }
  }

  const bottomPaneTarget = elements.find((element) =>
    Boolean(element.closest<HTMLElement>('[data-bottom-pane-drop-target]')),
  )
  if (bottomPaneTarget) return { paneId: BOTTOM_PANE_ID, zone: 'center' }

  return NO_TARGET
}

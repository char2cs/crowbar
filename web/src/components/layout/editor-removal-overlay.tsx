/**
 * What the editor pane says while a removal is armed over it.
 *
 * Rendered once and painted through the DOM, never through React — the same
 * rule the drag ghost and the drop hairline already keep. Arming is a pointer
 * event like any other, and pushing it through state would re-render the whole
 * editor pane in the middle of a drag to change two strings.
 */

const OVERLAY_ATTR = 'data-pane-removal'
const TITLE_ATTR = 'data-pane-removal-title'
const DETAIL_ATTR = 'data-pane-removal-detail'

/** What the veil reads while it is up. */
export interface RemovalOverlayText {
  title: string
  detail: string
}

/**
 * Raise the veil naming what will go, or take it away.
 *
 * A no-op when the pane is not on screen: the sidebar renders in windows that
 * have no editor (the nav screens), and a drag there simply has nowhere to drop.
 */
export function paintRemovalOverlay(text: RemovalOverlayText | null): void {
  const el = document.querySelector<HTMLElement>(`[${OVERLAY_ATTR}]`)
  if (!el) return
  if (!text) {
    el.hidden = true
    return
  }
  const title = el.querySelector<HTMLElement>(`[${TITLE_ATTR}]`)
  const detail = el.querySelector<HTMLElement>(`[${DETAIL_ATTR}]`)
  if (title && title.textContent !== text.title) title.textContent = text.title
  if (detail && detail.textContent !== text.detail) detail.textContent = text.detail
  el.hidden = false
}

/**
 * The dashed destructive veil over the editor pane.
 *
 * Always mounted, `hidden` at rest. Mounting it on arm would cost a React
 * render inside the drag AND give the veil nothing to fade from; hiding it
 * costs an attribute.
 *
 * `pointer-events-none` is load-bearing: the veil covers the pane it belongs
 * to, and the hit test walks the elements under the pointer — a veil that took
 * events would be the topmost of them and would answer for the pane it is
 * drawn on top of.
 */
export function EditorRemovalOverlay() {
  return (
    <div
      {...{ [OVERLAY_ATTR]: '' }}
      hidden
      aria-hidden="true"
      className="pointer-events-none absolute inset-2 z-20 grid place-items-center rounded-lg border-2 border-dashed border-destructive bg-destructive/10 backdrop-blur-[2px]"
    >
      <div className="text-center text-destructive">
        <svg
          aria-hidden="true"
          className="mx-auto size-7"
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.25"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 9a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1l1-9" />
        </svg>
        <div {...{ [TITLE_ATTR]: '' }} className="mt-2 text-sm font-semibold">
          Release to remove
        </div>
        <div {...{ [DETAIL_ATTR]: '' }} className="mt-0.5 text-xs opacity-80">
          You will have 8 seconds to undo
        </div>
      </div>
    </div>
  )
}

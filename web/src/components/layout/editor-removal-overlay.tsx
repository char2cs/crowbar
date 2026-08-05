import { cn } from '@/lib/utils'

/**
 * What the editor pane says while a row is in the air over it.
 *
 * TWO states, because the pane answers two different questions and used to
 * answer only the second:
 *
 *   - `available` — up for the WHOLE drag, from the moment a row leaves its
 *     slot. It says the zone exists. Without it the only way to discover that
 *     the pane deletes anything was to already know, carry a row there, and hold
 *     still for the dwell — an affordance you have to guess at is not one.
 *   - `armed` — after the pointer has rested on the pane long enough that a
 *     release really would remove. It says what will go, by name.
 *
 * The split matters: `available` must never read as "release now", because a
 * release anywhere else is a reorder. That is the whole reason the dwell exists
 * (a long reorder crosses this pane on its way), and showing one state for both
 * would turn the affordance into a lie for most of the drag.
 *
 * Rendered once and painted through the DOM, never through React — the same
 * rule the drag ghost and the drop hairline already keep. Arming is a pointer
 * event like any other, and pushing it through state would re-render the whole
 * editor pane in the middle of a drag to change two strings.
 */

const OVERLAY_ATTR = 'data-pane-removal'
const TITLE_ATTR = 'data-pane-removal-title'
const DETAIL_ATTR = 'data-pane-removal-detail'

/** What the veil reads while it is up, and how loudly. */
export interface RemovalOverlayText {
  title: string
  detail: string
  /** A release right now would remove. Drives the louder treatment. */
  armed: boolean
}

/**
 * Raise the veil, change what it says, or take it away.
 *
 * A no-op when the pane is not on screen: the sidebar renders in windows that
 * have no editor (the nav screens), and a drag there simply has nowhere to drop.
 */
export function paintRemovalOverlay(text: RemovalOverlayText | null): void {
  const el = document.querySelector<HTMLElement>(`[${OVERLAY_ATTR}]`)
  if (!el) return
  if (!text) {
    el.hidden = true
    el.removeAttribute('data-armed')
    return
  }
  const title = el.querySelector<HTMLElement>(`[${TITLE_ATTR}]`)
  const detail = el.querySelector<HTMLElement>(`[${DETAIL_ATTR}]`)
  if (title && title.textContent !== text.title) title.textContent = text.title
  if (detail && detail.textContent !== text.detail) detail.textContent = text.detail
  // Written only on a real change: the pointer sits inside one state for most of
  // a drag, and an attribute rewritten per pointermove is a style recalc per
  // pointermove on a full-pane element.
  const armed = el.hasAttribute('data-armed')
  if (text.armed !== armed) {
    if (text.armed) el.setAttribute('data-armed', '')
    else el.removeAttribute('data-armed')
  }
  el.hidden = false
}

/**
 * The dashed destructive veil over the editor pane.
 *
 * Always mounted, `hidden` at rest. Mounting it on arm would cost a React
 * render inside the drag AND give the veil nothing to fade from; hiding it
 * costs an attribute.
 *
 * ONE treatment at two strengths: available draws the whole veil — border, 45°
 * hatching, blur — and armed is the same veil with its RED turned up.
 *
 * The blur does not take part. It is full strength in both states, because a
 * container-level `opacity` composites the element together with its
 * backdrop-filter and so fades the blur too, which made the available state read
 * as a half-drawn armed one rather than a quieter one. Only the border, the
 * stripes and the text step.
 *
 * The blur is deliberate and it is NOT free. It was removed once as the presumed
 * cause of a drag slowdown; measuring the live app showed the cost was elsewhere
 * entirely (a full-window overlay element, since deleted — see index.css), and a
 * veil that had lost its blur had been degraded for nothing. Measure before
 * trading any of this away.
 *
 * `pointer-events-none` is load-bearing: the veil covers the pane it belongs
 * to, and the hit test walks the elements under the pointer — a veil that took
 * events would be the topmost of them and would answer for the pane it is
 * drawn on top of. It is also why the available state can be up for the whole
 * drag without interfering with a single drop.
 */
export function EditorRemovalOverlay() {
  return (
    <div
      {...{ [OVERLAY_ATTR]: '' }}
      hidden
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute inset-2 z-20 grid place-items-center overflow-hidden rounded-lg border-2 border-dashed',
        // The blur is FULL STRENGTH in both states and never animates. It is
        // what says "this pane is not itself right now", which is as true while
        // the zone is merely available as it is once armed. Fading it — which a
        // container `opacity` does, because opacity composites the element and
        // its backdrop-filter together — made the available state look like a
        // half-rendered version of the armed one instead of a quieter one.
        //
        // 4px, not 2: at 2 the pane read as slightly soft rather than as
        // deliberately veiled, and the hatching had to carry the whole signal.
        'backdrop-blur-[4px]',
        // Only the RED steps between the states, and it steps in ONE place: the
        // `--pattern-fg` token below drives the hatching, and the border and
        // text carry the matching pair. The blur underneath is untouched.
        'border-destructive/40 data-[armed]:border-destructive',
        'transition-[border-color] duration-100 motion-reduce:transition-none',
        // The hatching, as a real CSS pattern: a 1px rule per tile, drawn at
        // 315° and TILED by `background-size`, rather than one enormous gradient
        // stretched across the pane. `transparent 0` is the hard stop (a
        // position below the previous one clamps to it, so the line ends exactly
        // at 1px with no ramp), and the 50% that follows is half of the TILE —
        // which is what makes the period a property of `bg-[length:…]` instead
        // of a number baked into the gradient.
        //
        // The colour is a token so the two states are one variable with two
        // values; nothing here restates the red.
        '[--pattern-fg:--theme(--color-destructive/22%)]',
        'data-[armed]:[--pattern-fg:--theme(--color-destructive/70%)]',
        'before:pointer-events-none before:absolute before:inset-0 before:content-[""]',
        'before:bg-[repeating-linear-gradient(315deg,var(--pattern-fg)_0,var(--pattern-fg)_1px,transparent_0,transparent_50%)]',
        'before:bg-[length:10px_10px]',
      )}
    >
      <div className="relative text-center text-destructive/55 transition-colors duration-100 in-data-[armed]:text-destructive motion-reduce:transition-none">
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

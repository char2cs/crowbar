import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { createFollowScroll, type FollowScroll } from '@/features/agent/hooks/lib/follow-scroll'

/* How close to the bottom still counts as reading the newest end.
   Roughly one message row. Generous on purpose: a turn that lands several
   blocks at once can outrun a single scroll frame, and a reader who is
   obviously at the bottom should not stop being followed because the content
   grew faster than the scroll did. */
const STICK_SLACK = 96

/**
 * How recently a real input must have happened for a scroll to be read as the
 * READER's rather than the browser's.
 *
 * `scrollTop` changing is not evidence that anybody scrolled. The browser
 * moves it on its own — scroll anchoring, keeping the view stable when
 * content around it resizes — and a streaming transcript is content resizing
 * continuously, so this is the common case rather than an edge one. Measured
 * live on a 35-item numbered list: the view jumped 213px backward in one
 * sample while `scrollHeight` moved 3px (far too small to have clamped it),
 * with no gesture anywhere near, and following then stopped dead for 15.6
 * seconds because that jump was indistinguishable from the reader scrolling
 * up.
 *
 * A real gesture always announces itself first — `wheel`, `touchmove`,
 * `keydown`, or a scrollbar drag — a frame or two before the scroll event it
 * causes. Generous on purpose: erring toward "the reader did it" only ever
 * reproduces the old behaviour of pausing, which is safe, while erring the
 * other way would yank a reader back to the bottom mid-sentence.
 */
const READER_INPUT_MS = 1000

/**
 * How long after a turn starts (`pinTurnToTop`) a scroll cannot be read as
 * the reader's, full stop — see `pinGraceUntil`.
 *
 * Sized to the pin's OWN settle sequence, not to any input's recency: the
 * just-sent prompt swaps from a queued row to its ledger row, the working
 * indicator mounts, tail-room finds its real reservation — measured live,
 * that cascade runs for up to ~1.3s. Shorter than that would let the last
 * of it slip back through the generic heuristic; there is no cost to
 * generosity here the way there is with `READER_INPUT_MS`, since this
 * window opens at a moment this file chose, not one it is guessing about.
 */
const PIN_SETTLE_GRACE_MS = 1500

/** Where the reader was, captured on unmount so the NEXT time this exact
 *  chat mounts (a switch back, this session) it can pick up from here
 *  instead of defaulting to the bottom — see UseTranscriptAnchorOptions. */
export interface TranscriptScrollPosition {
  /** Was the reader following the newest message (see STICK_SLACK)? */
  stuck: boolean
  /** `el.scrollHeight - el.scrollTop` at the moment this was captured. A
   *  distance from the BOTTOM, not a raw scrollTop: content can legitimately
   *  grow while the chat is away (new turns landing), and anchoring to the
   *  bottom is what a reader mid-history actually expects to still hold,
   *  the same reasoning `preservePosition`/`restoreFromBottom` already use
   *  for a prepend. */
  distanceFromBottom: number
}

export interface UseTranscriptAnchorOptions {
  /**
   * True while the chat's initial page of history is still being fetched.
   * While true — and for one quiet frame after it goes false, see
   * `easedArmed` below — every resync snaps instantly instead of easing.
   *
   * Without this, a cold or warm chat open reads as the whole transcript
   * sweeping from wherever the empty/loading state left it up to the true
   * bottom: the virtualized list's OWN initial estimated→measured row-height
   * corrections (routinely dozens, as the first page's rows settle in one
   * by one) each retarget the SAME eased glide this hook builds for a
   * single new line streaming in, and a burst of those reads as one long,
   * visible glide rather than the instant landing a reopen should be.
   *
   * Defaults to false: a caller that never mentions loading gets this
   * hook's original always-eased behaviour, unmodified.
   */
  loadingHistory?: boolean
  /** A previously-saved position for this exact chat — read ONCE, at mount.
   *  Omitted or null: land at the bottom, same as before this option
   *  existed. */
  initialPosition?: TranscriptScrollPosition | null
  /** Called once, from unmount, with wherever the reader ended up. Not
   *  called on every scroll: nothing reads the value before the NEXT mount
   *  of this same chat, so there is nothing to keep current in the
   *  meantime. */
  onPositionChange?: (position: TranscriptScrollPosition) => void
}

export interface TranscriptAnchor {
  /** Goes on the scroll container. Its LAST element child is the content
   *  watched for growth — see this file's own effect for why it is the
   *  last child and not (as it used to be, back when it was the only
   *  child) simply the first. */
  scrollRef: React.RefObject<HTMLDivElement | null>
  /** Goes on the same container's onScroll. */
  onScroll: () => void
  /** Call immediately BEFORE prepending older messages. */
  preservePosition: () => void
  /**
   * Call whenever something OUTSIDE this container changed the true
   * scrollable bottom without changing this container's own box, or the
   * content's — the one case the internal ResizeObserver structurally
   * cannot see. The docked composer is exactly that: `.scroll`'s
   * `padding-bottom` reserves room for it via `--agent-dock-h`, but the
   * composer OVERLAYS the transcript rather than sizing it, so growing —
   * a halted-turn banner appearing on one particular close path, say —
   * changes `scrollHeight` alone. Neither the content box nor the
   * container's own box moved, so nothing this hook already watches fires,
   * and the follow target goes stale short of the new true bottom: the
   * last lines sit reachable in principle but behind the (now taller)
   * composer, which is exactly what "closed, but the end is unreachable
   * until more text pushes past it" was reported as live.
   */
  notifyReflow: () => void
  /**
   * Call as a turn STARTS, with the just-sent user message's element: the
   * transcript brings that message's top edge up to the top of the viewport
   * and leaves the reply room to grow downward into, instead of starting the
   * reply wherever bottom-following happened to leave the previous turn.
   *
   * Pass null to give up the pin early (the chat closed, the turn never
   * produced anything). It releases itself as soon as the reply outgrows the
   * space below it — see `tailRoom`.
   */
  pinTurnToTop: (element: HTMLElement | null) => void
}

/**
 * How much empty room the content needs BELOW `pin` for that pin to be able
 * to sit at the top of the viewport — the whole of this behaviour, in one
 * number.
 *
 * A scroll container cannot scroll past its own end, so "put this element at
 * the top" is not a scroll instruction at all when there is nothing below it:
 * it is a request for somewhere to scroll TO. Reserving exactly the shortfall
 * is what makes it reachable, and it is deliberately the ONLY thing this
 * behaviour does — with the room reserved, the ordinary bottom-follow below
 * already lands in the right place in both phases, and the handoff between
 * them needs no mode of its own:
 *
 *   - while the reply is shorter than the viewport, the true bottom IS the
 *     pinned position, so following the bottom holds the prompt at the top
 *     and the reply fills the space underneath;
 *   - once the reply outgrows that space the shortfall reaches zero, this
 *     stops reserving anything, and following the bottom is once again
 *     following the bottom.
 *
 * Returns 0 (and so releases the pin) the moment it is no longer needed.
 */
export function tailRoom(pinTop: number, contentHeight: number, viewportHeight: number): number {
  return Math.max(0, viewportHeight - (contentHeight - pinTop))
}

/**
 * Keeps a transcript pinned to its newest message.
 *
 * The anchoring is measured, not declarative, because the two things a chat does
 * — grow at the bottom as a turn streams, grow at the TOP when older messages
 * page in — need opposite responses, and CSS cannot tell them apart.
 *
 * Growth is observed rather than derived from a revision prop: a turn changes
 * height for reasons no counter sees (a tool row resolving, a subagent shelf
 * appearing, prose reflowing on a pane resize), and every one of them must keep
 * the newest line on screen.
 *
 * Following stops the moment the reader scrolls up. That is the whole contract:
 * a chat that yanks you back to the bottom while you are reading history is
 * worse than one that never follows at all.
 */
export function useTranscriptAnchor(options: UseTranscriptAnchorOptions = {}): TranscriptAnchor {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const stuck = useRef(options.initialPosition?.stuck ?? true)
  // Distance from the bottom to restore after a prepend, or null when the next
  // resize is ordinary growth.
  const restoreFromBottom = useRef<number | null>(null)
  const follow = useRef<FollowScroll | null>(null)
  // The value `follow`'s own writes are producing, right now — how `onScroll`
  // tells "that's just this loop ticking" apart from a real scroll gesture
  // landing mid-flight, since both change the same property. Null whenever
  // nothing is currently following.
  const expectedScrollTop = useRef<number | null>(null)
  // The effect below rebuilds `resync` on every mount — a ref is how
  // `notifyReflow`, a STABLE callback returned once, reaches whichever
  // instance is currently live rather than closing over a stale one.
  const resyncRef = useRef<() => void>(() => {})
  // How the loadingHistory-transition effect below reaches the SAME
  // scheduleArm the ResizeObserver effect's own resync uses, rather than
  // running a second, independent requestAnimationFrame of its own — one
  // pending arm request at a time, always through `armFrame`, however it
  // gets triggered.
  const scheduleArmRef = useRef<() => void>(() => {})
  // Read only inside the two mount-only (`[]` deps) effects below, so they
  // see the LATEST callbacks/values without re-running on every render —
  // this hook's caller remounts wholesale on every chat switch anyway, so
  // identity churn mid-life is not a real concern, but a ref costs nothing
  // and avoids depending on that.
  const optionsRef = useRef(options)
  optionsRef.current = options
  // Armed once the initial history load's own measurement settle is over —
  // see UseTranscriptAnchorOptions.loadingHistory. Starts armed unless a
  // caller says otherwise, reproducing this hook's original (always-eased)
  // behaviour exactly when `loadingHistory` is never mentioned.
  const easedArmed = useRef(!(options.loadingHistory ?? false))
  const armFrame = useRef(0)
  // How far down the content the turn currently held at the top begins, or
  // null when none is — see `pinTurnToTop`. Cleared by `applyTailRoom` itself
  // once the reply has grown past the space below it.
  //
  // An OFFSET, not the element it was measured from, for two reasons. The
  // element does not survive the turn: a just-sent prompt starts life as a
  // queued row and is swapped — in a single commit — for a virtualized
  // message row the moment the ledger confirms it, so anything holding the
  // node would lose the pin mid-reply. And nothing above the pin moves while
  // a turn runs (it is settled history), so the offset stays true without
  // being re-measured, which also keeps this off the layout-reading path of
  // every ResizeObserver callback.
  //
  // Measured relative to `.stream` (the content element `applyTailRoom`
  // reserves padding on), NOT `.scroll` (the scrollable container) — the
  // latter also contains `.scroll-spacer`, a flex-grow sibling ABOVE
  // `.stream` that bottom-anchors a short conversation and collapses toward
  // 0 the instant real content needs the room instead (transcript.css). The
  // very first reservation this pin ever triggers does exactly that: it
  // grows `.stream`, which shrinks the spacer by the same amount, which
  // silently moves anything measured relative to `.scroll` out from under
  // whatever offset was captured here — a turn no longer settled history.
  // Reported live as a nearly blank transcript after a short first prompt:
  // the spacer being large (little content yet) is precisely what made the
  // shift big enough to notice. `.stream`'s own top is never affected by its
  // sibling's height, so an offset measured against it stays true for the
  // same reason the original comment already gives for the rest of this
  // value.
  const pinnedTop = useRef<number | null>(null)
  // When the reader last actually did something — see READER_INPUT_MS.
  const lastInputAt = useRef(Number.NEGATIVE_INFINITY)
  // A scrollbar drag only announces itself once, at `pointerdown`, and can
  // then run for as long as the reader holds the button; the timestamp alone
  // would go stale under them mid-drag.
  const pointerHeld = useRef(false)
  // Until this timestamp, `onScroll` cannot read a scroll as the reader's,
  // no matter what `lastInputAt`/`pointerHeld` say. Set only by
  // `pinTurnToTop`, which already knows — explicitly, synchronously, with
  // no inference involved — that a turn just started and `stuck` needs to
  // survive whatever resizes that turn's own settling produces (a queued
  // row swapping for its ledger row, tail-room finding its footing).
  //
  // `READER_INPUT_MS` above exists to solve a DIFFERENT, harder problem —
  // telling a real gesture apart from the browser's own scroll anchoring,
  // which fires with NO programmatic signal at all, so recency is the only
  // evidence available. Sending a prompt is not that problem: submitting IS
  // a keydown (Enter) or a pointerdown/up (Send), so it always sits inside
  // that same recency window, and the reader-heuristic could only ever be
  // taught to carve THIS keystroke or THAT click out one at a time. Asserting
  // the known window directly, from the one call site that actually knows it
  // exists, closes the whole class at once instead of chasing each new event
  // source into it.
  const pinGraceUntil = useRef(0)

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const initial = optionsRef.current.initialPosition
    if (initial && !initial.stuck) {
      el.scrollTop = Math.max(0, el.scrollHeight - initial.distanceFromBottom)
    } else {
      el.scrollTop = el.scrollHeight
    }
  }, [])

  useEffect(() => {
    const el = scrollRef.current
    // The LAST child, not the first: `.scroll`'s first child is now
    // `.scroll-spacer` (transcript.css), a decorative flex-grow sibling
    // that bottom-anchors a short conversation and is DELIBERATELY pinned
    // at 0 height for the entire time a conversation overflows — which is
    // exactly when growth needs to be seen. Watching it instead of the real
    // content meant every message after the first one to cause overflow
    // grew `.stream` (the actual content, now the LAST child) with nobody
    // watching: the ResizeObserver below never fired again, and follow
    // silently stopped following. `.stream` is always the last child by
    // construction (agent-transcript.tsx renders `.scroll-spacer` first).
    const content = el?.lastElementChild
    if (!el || !content) return
    const buildFollow = () =>
      createFollowScroll(el, (v) => {
        expectedScrollTop.current = v
      })
    follow.current = buildFollow()
    // Re-armed by every unarmed resync below, so eased mode only engages once
    // a FULL frame passes with nothing left to settle — a burst of initial
    // measurement corrections keeps pushing this back, the same way a burst
    // of new lines keeps retargeting the eased loop itself (follow-scroll.ts).
    const scheduleArm = () => {
      if (easedArmed.current) return
      cancelAnimationFrame(armFrame.current)
      armFrame.current = requestAnimationFrame(() => {
        easedArmed.current = true
      })
    }
    scheduleArmRef.current = scheduleArm
    // Reserves (and keeps re-measuring) the room the pinned turn needs below
    // it — see `tailRoom`. Written as padding on the content element rather
    // than as a spacer sibling because `.scroll`'s LAST child is what the
    // observer below treats as the content: a new element there would
    // silently become the thing being watched, and the real content's growth
    // would stop being seen at all.
    const applyTailRoom = () => {
      const pinTop = pinnedTop.current
      const box = content as HTMLElement
      if (pinTop === null) {
        if (box.style.paddingBottom) box.style.paddingBottom = ''
        return
      }
      const reserved = parseFloat(box.style.paddingBottom || '0') || 0
      // `box.scrollHeight`, not `el.scrollHeight` — see `pinnedTop`'s own doc
      // for why measuring against `.scroll` itself (which also contains
      // `.scroll-spacer`) is exactly the bug this replaced.
      const room = tailRoom(pinTop, box.scrollHeight - reserved, el.clientHeight)
      // Released for good once the reply has outgrown the space: re-measuring
      // a pin nobody can see any more would keep this running for the rest of
      // the turn, and re-reserving room mid-reply would yank the reader.
      if (room <= 0) {
        pinnedTop.current = null
        if (box.style.paddingBottom) box.style.paddingBottom = ''
        return
      }
      // Sub-pixel churn here feeds straight back into the ResizeObserver that
      // called this, so only a real change is written.
      if (Math.abs(room - reserved) > 1) box.style.paddingBottom = `${room}px`
    }

    const resync = () => {
      applyTailRoom()
      const keep = restoreFromBottom.current
      if (keep !== null) {
        // Older messages just landed above the fold. Holding the distance from
        // the BOTTOM — not scrollTop — is what leaves the row the reader was
        // looking at exactly where it was. Instant, deliberately: nothing here
        // is "the newest line arriving", so easing it would read as the whole
        // transcript sliding for no visible reason.
        restoreFromBottom.current = null
        el.scrollTop = el.scrollHeight - keep
        return
      }
      if (!stuck.current) return
      // The scrollable ceiling, not the raw content height — el.scrollTop can
      // never legally exceed scrollHeight - clientHeight. Feeding the raw
      // scrollHeight in here doesn't just settle at the wrong place: the
      // exponential-smoothing math below computes its per-frame step from
      // this value BEFORE the browser ever gets a chance to clamp anything,
      // so an error the size of clientHeight (typically far bigger than one
      // chunk's growth) makes the very first frame overshoot the REAL
      // ceiling — which the native scrollTop setter then clamps to
      // instantly. The result is indistinguishable from no easing at all.
      const target = el.scrollHeight - el.clientHeight
      if (!easedArmed.current) {
        // Still settling (see UseTranscriptAnchorOptions.loadingHistory):
        // land on the real target instantly, same as the prepend branch
        // above, and push the arm-check back another frame.
        el.scrollTop = target
        scheduleArm()
        return
      }
      follow.current?.setTarget(target)
    }
    resyncRef.current = resync
    const observer = new ResizeObserver(resync)
    // BOTH, and the second one is the half that was missing. The conversation
    // slides out of view for two different reasons: the content grows (a turn
    // streams) or the VIEWPORT shrinks (the composer grows a line under it).
    // Watching only the content meant typing a second line quietly pushed the
    // newest message behind the box — nothing had resized, so nothing followed.
    observer.observe(content)
    observer.observe(el)
    // The OS window losing focus — switching to another app while a turn is
    // still streaming — freezes requestAnimationFrame in this webview
    // entirely (confirmed live: rAF callbacks stop firing the instant
    // `document.hasFocus()` goes false, even though `document.visibilityState`
    // stays "visible" the whole time, so `visibilitychange` never fires and
    // can't be the signal here). The follow's own glide is built entirely on
    // rAF (follow-scroll.ts), so a reply that keeps growing while the window
    // sits unfocused leaves scrollTop wherever the glide's last real frame
    // wrote it — short of the true bottom — with nothing left to finish the
    // catch-up once rAF stalls.
    //
    // Calling `resync` alone is NOT enough, and was the bug in an earlier
    // version of this fix: requestAnimationFrame is never refused, only its
    // callback's invocation is deferred — a real engine hands back a genuine
    // request id synchronously even while frozen, and that id is never
    // invoked once focus is lost for good (confirmed live). `setTarget`
    // only schedules a FRESH request when its own `raf` bookkeeping reads
    // zero, i.e. only when nothing is already (supposedly) pending — so the
    // instant ANY resize happened while unfocused (the ordinary case for a
    // mid-stream reply, not an edge case), that guard is already pinned on
    // a request that will never fire, and plain `resync` silently no-ops
    // forever. Rebuilding the whole loop guarantees a BRAND NEW request,
    // made after focus has actually returned — which this environment has
    // already been confirmed to honor.
    const onWindowFocus = () => {
      follow.current?.stop()
      follow.current = buildFollow()
      resync()
    }
    window.addEventListener('focus', onWindowFocus)
    // Captured on the window, not the container: a keypress scrolls the
    // transcript while focus sits anywhere in the chat, and capture phase
    // means nothing downstream can swallow the signal before it is recorded.
    const noteInput = () => {
      lastInputAt.current = performance.now()
    }
    // A keydown ONLY: typing, or pressing Enter to send, in the composer is
    // never "the reader scrolling the transcript" — it just happens to be a
    // keydown, the same event type PageDown/Space/arrow keys use to
    // legitimately scroll the transcript when IT has focus. Without this,
    // the send keystroke itself armed `reader` for a full READER_INPUT_MS
    // afterward, and any resize-driven scroll adjustment in that window (the
    // browser's own clamp when content shrinks, say) got misread as the
    // reader grabbing the scrollbar. Observed live: `stuck` latched false
    // right after send, the pin-to-top reservation kept adjusting
    // (`applyTailRoom` runs unconditionally) while `scrollTop` itself never
    // moved again — "the space is there, the auto-scroll didn't work."
    const noteKeydownUnlessEditing = (event: Event) => {
      const target = event.target
      if (target instanceof Element && target.closest('[contenteditable], input, textarea')) return
      noteInput()
    }
    // Scoped to a wheel/touch event that actually targets THIS container —
    // unscoped, this was the wheel/touch twin of the keydown and pointerdown
    // bugs just above/below: scrolling a DIFFERENT pane entirely (split
    // view) or any other on-screen scrollable region set `lastInputAt` for
    // every mounted instance of this hook, and if the browser's own scroll
    // anchoring then adjusted a DIFFERENT, actively-streaming pane within
    // READER_INPUT_MS, `onScroll` misread it as that pane's own reader
    // grabbing the scrollbar and stopped following for the rest of the turn.
    const noteWheelOrTouchWithinContainer = (event: Event) => {
      const target = event.target
      if (!(target instanceof Element) || !el.contains(target)) return
      noteInput()
    }
    // Scoped to a pointerdown that actually STARTS on this container (its
    // scrollbar, its rows) — the scrollbar-drag `pointerHeld` above exists
    // for. Unscoped, this was the pointer-event twin of the keydown bug just
    // above: clicking Send, or literally anything else anywhere in the app,
    // fired a pointerdown/pointerup pair on `window` and got read as the
    // reader grabbing the scrollbar. `pointerup`/`pointercancel` stay
    // UNSCOPED on purpose — a real drag can end with the cursor anywhere
    // once it outruns the scrollbar's bounds — but only ever DO anything
    // when `pointerHeld` says a drag we actually started tracking is the
    // one ending.
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target
      if (!(target instanceof Element) || !el.contains(target)) return
      pointerHeld.current = true
      noteInput()
    }
    const onPointerUp = () => {
      if (!pointerHeld.current) return
      pointerHeld.current = false
      noteInput()
    }
    const INPUT_EVENTS = ['wheel', 'touchstart', 'touchmove'] as const
    for (const type of INPUT_EVENTS) {
      window.addEventListener(type, noteWheelOrTouchWithinContainer, {
        capture: true,
        passive: true,
      })
    }
    window.addEventListener('keydown', noteKeydownUnlessEditing, { capture: true, passive: true })
    window.addEventListener('pointerdown', onPointerDown, { capture: true, passive: true })
    window.addEventListener('pointerup', onPointerUp, { capture: true, passive: true })
    window.addEventListener('pointercancel', onPointerUp, { capture: true, passive: true })
    return () => {
      observer.disconnect()
      window.removeEventListener('focus', onWindowFocus)
      for (const type of INPUT_EVENTS) {
        window.removeEventListener(type, noteWheelOrTouchWithinContainer, { capture: true })
      }
      window.removeEventListener('keydown', noteKeydownUnlessEditing, { capture: true })
      window.removeEventListener('pointerdown', onPointerDown, { capture: true })
      window.removeEventListener('pointerup', onPointerUp, { capture: true })
      window.removeEventListener('pointercancel', onPointerUp, { capture: true })
      follow.current?.stop()
      follow.current = null
      resyncRef.current = () => {}
      scheduleArmRef.current = () => {}
      pinnedTop.current = null
      cancelAnimationFrame(armFrame.current)
      // Wherever the reader ends up, for this exact chat's next mount this
      // session (a switch back) to restore — see
      // UseTranscriptAnchorOptions.onPositionChange.
      optionsRef.current.onPositionChange?.({
        stuck: stuck.current,
        distanceFromBottom: el.scrollHeight - el.scrollTop,
      })
    }
  }, [])

  // Backstops `scheduleArm` above for a chat whose initial page causes NO
  // resize at all (a brand-new, empty chat, say) — nothing would otherwise
  // ever arm eased mode for it. Runs only on the loadingHistory transition
  // (or immediately, for a caller that starts already not-loading), so it
  // does not re-fire on the unrelated renders in between.
  useEffect(() => {
    if (options.loadingHistory) return
    scheduleArmRef.current()
  }, [options.loadingHistory])

  const notifyReflow = useCallback(() => {
    resyncRef.current()
  }, [])

  const pinTurnToTop = useCallback((element: HTMLElement | null) => {
    const el = scrollRef.current
    if (!el || !element) {
      pinnedTop.current = null
      resyncRef.current()
      return
    }
    // Measured once, here, against `.stream` (`el`'s last child — see the
    // mount effect above) rather than `el` itself — see `pinnedTop`'s own
    // doc for why. Both elements are fixed for the element's lifetime; no
    // scrollTop term is needed the way `el`-relative measurement required,
    // since a descendant's rect and its ancestor CONTENT element's rect move
    // together by the same amount as `el` scrolls.
    const base = el.lastElementChild as HTMLElement | null
    pinnedTop.current =
      element.getBoundingClientRect().top - (base ?? el).getBoundingClientRect().top
    // A turn starting is also the reader rejoining the live end — it is their
    // own prompt that just landed. Without this, a prompt sent after reading
    // back through history would reserve the room and then not move.
    stuck.current = true
    // Protects the assertion just above for as long as this pin's own
    // settling can plausibly still be resizing things — see
    // `pinGraceUntil`/`PIN_SETTLE_GRACE_MS`.
    pinGraceUntil.current = performance.now() + PIN_SETTLE_GRACE_MS
    resyncRef.current()
  }, [])

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    // A scroll event the follow loop's OWN writes produced echoes back a
    // value it just wrote — matching within a pixel is what tells that apart
    // from the reader's own gesture, and it must not count as "scrolled up":
    // the loop runs precisely BECAUSE the reader is still at the bottom.
    if (
      expectedScrollTop.current !== null &&
      Math.abs(el.scrollTop - expectedScrollTop.current) <= 1
    ) {
      return
    }
    // Not one of our own writes. The loop's own drift check would catch this
    // too on its next frame, but clearing the expected value here means the
    // NEXT tick doesn't have to.
    expectedScrollTop.current = null
    // ...but "not ours" is still not "the reader's". The browser moves
    // scrollTop by itself to keep the view stable when content around it
    // resizes, which a streaming transcript does constantly — see
    // READER_INPUT_MS. Treating that as a gesture is what left a reply
    // stranded mid-generation with the transcript refusing to follow it any
    // further. Nobody having touched anything means the view is still where
    // the reader left it: keep following, from wherever it now sits.
    const reader =
      performance.now() >= pinGraceUntil.current &&
      (pointerHeld.current || performance.now() - lastInputAt.current < READER_INPUT_MS)
    if (!reader) {
      resyncRef.current()
      return
    }
    // Re-armed as soon as the reader comes back to the bottom, so following
    // resumes without them having to do anything but scroll down.
    stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_SLACK
  }, [])

  const preservePosition = useCallback(() => {
    const el = scrollRef.current
    if (el) restoreFromBottom.current = el.scrollHeight - el.scrollTop
  }, [])

  return { scrollRef, onScroll, preservePosition, notifyReflow, pinTurnToTop }
}

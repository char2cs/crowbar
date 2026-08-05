import { useEffect, useRef, useState } from 'react'
import type React from 'react'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'
import { cn } from '@/lib/utils'

// Discover every flip-dot spinner SVG as raw markup at build time. Adding a
// spinner = drop an .svg in ./spinners — no index, no codegen.
//
// The assets animate with SMIL, but we do NOT mount them that way: their 25
// discrete `<animate>` timers run on the main thread and make every step
// rebuild the whole layer tree, so one 16px spinner costs in proportion to the
// size of the entire app. buildFlickerStrip flattens the timeline into a static
// sprite strip that plays as a single stepped `transform` — see flicker-strip.ts
// for why that is exact and why it is cheap. Inlining the markup (rather than
// an <img src>) stays load-bearing either way: it lets the dots'
// fill="currentColor" inherit the Crowbar theme token from an ancestor.
const SPINNERS = Object.values(
  import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw', import: 'default' }),
) as string[]

// Baked lazily and memoised: an asset costs one parse the first time it is
// picked, and never again. Building all of them up front would hold ~30 strips
// in memory for the one or two a session actually shows.
const strips = new Map<number, ReturnType<typeof buildFlickerStrip>>()
function stripFor(index: number): ReturnType<typeof buildFlickerStrip> {
  if (!strips.has(index)) strips.set(index, buildFlickerStrip(SPINNERS[index] ?? ''))
  return strips.get(index) ?? null
}

export function FlickerSpinner({
  className,
  ...props
}: React.ComponentProps<'span'>): React.ReactElement {
  // Random-pick one spinner per instance, stable for the component's lifetime
  // (mirrors the retired WorkspaceAgentSpinner's spinnerNames pick).
  const [index] = useState(() => Math.floor(Math.random() * SPINNERS.length))
  const strip = stripFor(index)
  const boxRef = useRef<HTMLSpanElement>(null)
  const stripRef = useRef<HTMLSpanElement>(null)

  // A spinner nobody can see still drives the compositor. The sidebar's panels
  // are an off-viewport scroll-snap carousel and rows scroll out of view, so
  // park the animation while the box is off-screen.
  //
  // Deliberately fail-open: the play state starts `running` and only a
  // definitive not-intersecting observation pauses it. A spinner frozen by a
  // wrong guess reads as "hung", which is far worse than the frames it saves.
  useEffect(() => {
    const box = boxRef.current
    if (!box || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        const el = stripRef.current
        if (el) el.style.animationPlayState = entry.isIntersecting ? 'running' : 'paused'
      }
    })
    observer.observe(box)
    return () => observer.disconnect()
  }, [])

  return (
    <span
      ref={boxRef}
      role="status"
      aria-label="Loading"
      // The stable hook for "this is the real flicker spinner" — call-site
      // tests assert on it. They used to look for an <animate> element, which
      // is no longer how the animation is expressed.
      data-flicker-spinner=""
      // Exempts this subtree from the prefers-reduced-motion kill-switch in
      // index.css. SMIL was never subject to it; a CSS animation is, and
      // freezing an indeterminate status spinner turns "still working" into
      // "looks hung". Guarded by __tests__/reduced-motion.test.ts.
      data-essential-motion=""
      // Size via className (default size-4). Color is NOT baked in here: the
      // SVG dots use fill="currentColor", so callers color this by wrapping it
      // in (or applying) a text-* theme token span — never a hardcoded color.
      // overflow-hidden is the frame window the strip slides behind.
      className={cn(
        'relative inline-flex size-4 items-center justify-center overflow-hidden',
        className,
      )}
      {...props}
    >
      {strip ? (
        <span
          ref={stripRef}
          aria-hidden="true"
          className="absolute inset-y-0 left-0 [&>svg]:block [&>svg]:size-full"
          style={{
            // N frames wide, stepped one frame-width per tick. translateX(-100%)
            // is the strip's own width, so steps(N) lands exactly on frame
            // boundaries; `steps()` defaults to jump-end, which holds frame 0
            // from t=0 — the same phase SMIL starts on.
            width: `${strip.frames * 100}%`,
            animation: `flicker-strip ${strip.duration} steps(${strip.frames}) infinite`,
            // Promotes the strip so playback is a compositor transform on an
            // already-painted layer: no style recalc, no layer-tree rebuild,
            // and it keeps running while the window is unfocused.
            //
            // react-doctor-disable-next-line no-permanent-will-change -- the rule's fix (add on animationstart, remove on animationend) does not apply: this animation is `infinite` and runs for the element's entire mounted lifetime, and the element is mounted only while something is loading. There is no "end" to remove the hint at, and dropping it costs the promotion this spinner was rebuilt from SMIL to get (−59% main thread, measured).
            willChange: 'transform',
          }}
          // react-doctor-disable-next-line dangerous-html-sink -- `markup` is derived from a static build-time SVG asset from ./spinners/*.svg (import.meta.glob '?raw'), never attacker-influenceable. DOMPurify would strip the load-bearing currentColor markup.
          dangerouslySetInnerHTML={{ __html: strip.markup }}
        />
      ) : (
        // An asset the baker cannot reproduce exactly (see flicker-strip.ts)
        // falls back to inlining it as-is and letting its own SMIL run. Slower,
        // never wrong.
        <span
          aria-hidden="true"
          className="size-full [&>svg]:size-full"
          // react-doctor-disable-next-line dangerous-html-sink -- static build-time SVG asset, as above.
          dangerouslySetInnerHTML={{ __html: SPINNERS[index] ?? '' }}
        />
      )}
    </span>
  )
}
